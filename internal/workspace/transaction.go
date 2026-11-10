package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/state"
	"gopkg.in/yaml.v3"
)

// TransactionOutcome describes whether a failed operation rolled back or
// crossed its durable commit point.
type TransactionOutcome string

const transactionDocuments = "documents"
const transactionCommittedPhase = "committed"

const (
	// OutcomeRolledBack means no durable ownership change remains.
	OutcomeRolledBack TransactionOutcome = "rolled-back"
	// OutcomePending means recovery is required before mutation.
	OutcomePending TransactionOutcome = "pending-recovery"
	// OutcomeCommittedCleanupPending means the owner change is committed and a journal remains to finish cleanup.
	OutcomeCommittedCleanupPending TransactionOutcome = "committed-cleanup-pending"
	// OutcomeCommitIndeterminate means the commit-marker publication could not be reconciled; recovery must inspect the journal.
	OutcomeCommitIndeterminate TransactionOutcome = "commit-indeterminate"
)

// TransactionError carries the recovery ID and durable outcome without
// exposing private journal snapshots in its message.
type TransactionError struct {
	ID      string
	Outcome TransactionOutcome
	Cause   error
}

func (e *TransactionError) Error() string {
	return fmt.Sprintf("workspace transaction %s (%s): %v", e.ID, e.Outcome, e.Cause)
}
func (e *TransactionError) Unwrap() error { return e.Cause }

// TransactionInfo is a safe public summary of a pending journal.
type TransactionInfo struct {
	ID        string
	Roots     []string
	Committed bool
	Phase     string
}

type moveJournal struct {
	Kind                    string               `json:"kind,omitempty"`
	Version                 int                  `json:"version"`
	ID                      string               `json:"id"`
	Phase                   string               `json:"phase"`
	Intent                  string               `json:"intent,omitempty"`
	Committed               bool                 `json:"committed"`
	PublicRoot              string               `json:"public_root"`
	PrivateRoot             string               `json:"private_root"`
	SourceRoot              string               `json:"source_root"`
	DestinationRoot         string               `json:"destination_root"`
	SourceRef               EntryRef             `json:"source_ref,omitempty"`
	DestinationRef          EntryRef             `json:"destination_ref,omitempty"`
	PublicConfig            string               `json:"public_config"`
	PrivateConfig           string               `json:"private_config"`
	PublicBefore            []byte               `json:"public_before"`
	PrivateBefore           []byte               `json:"private_before"`
	PublicAfter             []byte               `json:"public_after"`
	PrivateAfter            []byte               `json:"private_after"`
	SourceRel               string               `json:"source_rel"`
	SourceMethod            string               `json:"source_method,omitempty"`
	DestinationRel          string               `json:"destination_rel"`
	Files                   []fileSnapshot       `json:"files"`
	Links                   []linkSnapshot       `json:"links"`
	Created                 string               `json:"created"`
	StateKeys               []string             `json:"state_keys,omitempty"`
	StateRecords            []state.RenderRecord `json:"state_records,omitempty"`
	DestinationStateExisted bool                 `json:"destination_state_existed,omitempty"`
	StateCleaned            bool                 `json:"state_cleaned,omitempty"`
	WholeFolder             bool                 `json:"whole_folder,omitempty"`
	SelectedDirs            []string             `json:"selected_dirs,omitempty"`
	CleanedPaths            []string             `json:"cleaned_paths,omitempty"`
	RolledBackPaths         []string             `json:"rolled_back_paths,omitempty"`
}

var lockState = struct {
	sync.Mutex
	held map[string]bool
}{held: map[string]bool{}}
var faultHook func(string) error
var errSimulatedCrash = errors.New("simulated process interruption")
var mutationGuardLockedHook func()
var recoveryLockedHook func()

// AcquireMutationLocks serializes mutations for canonical roots in stable
// order. The returned release function is safe to call once or repeatedly.
func AcquireMutationLocks(ctx context.Context, roots ...string) (func(), error) {
	canonical := make([]string, 0, len(roots))
	for _, r := range roots {
		p, e := canonicalRoot(r)
		if e != nil {
			return nil, e
		}
		canonical = append(canonical, p)
	}
	sort.Strings(canonical)
	lockState.Lock()
	for _, p := range canonical {
		if lockState.held[p] {
			lockState.Unlock()
			return nil, fmt.Errorf("workspace root is being modified: %s", p)
		}
	}
	for _, p := range canonical {
		lockState.held[p] = true
	}
	lockState.Unlock()
	fileReleases := make([]func(), 0, len(canonical))
	for _, root := range canonical {
		releaseFile, lockErr := acquireRootLock(ctx, root)
		if lockErr != nil {
			for i := len(fileReleases) - 1; i >= 0; i-- {
				fileReleases[i]()
			}
			lockState.Lock()
			for _, p := range canonical {
				delete(lockState.held, p)
			}
			lockState.Unlock()
			return nil, lockErr
		}
		fileReleases = append(fileReleases, releaseFile)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			lockState.Lock()
			for i := len(fileReleases) - 1; i >= 0; i-- {
				fileReleases[i]()
			}
			for _, p := range canonical {
				delete(lockState.held, p)
			}
			lockState.Unlock()
		})
	}
	if err := ctx.Err(); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

// PendingTransactions finds journals whose root set intersects any requested
// root. It is intentionally callable before merged configuration validation.
//
//nolint:gocyclo // journal loading rejects malformed entries while matching any owner root.
func PendingTransactions(ctx context.Context, journalDir string, roots ...string) ([]TransactionInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := journalDirectory(journalDir)
	if err != nil {
		return nil, err
	}
	if err := validateJournalLocation(dir, roots...); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	requested := map[string]bool{}
	for _, root := range roots {
		p, e := canonicalRoot(root)
		if e == nil {
			requested[p] = true
		}
	}
	var pending []TransactionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("workspace journal may not be a symlink: %s", entry.Name())
		}
		data, e := os.ReadFile(filepath.Join(dir, entry.Name()))
		if e != nil {
			return nil, e
		}
		var j moveJournal
		if e := json.Unmarshal(data, &j); e != nil {
			return nil, fmt.Errorf("invalid workspace journal %s", entry.Name())
		}
		matches := len(requested) == 0
		for _, r := range []string{j.PublicRoot, j.PrivateRoot} {
			if requested[r] {
				matches = true
			}
		}
		if matches {
			pending = append(pending, TransactionInfo{ID: j.ID, Roots: []string{j.PublicRoot, j.PrivateRoot}, Committed: j.Committed, Phase: j.Phase})
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })
	return pending, nil
}

// MutationGuard blocks a normal mutation if any pending transaction touches a
// requested root, then acquires the same root locks used by moves.
func MutationGuard(ctx context.Context, journalDir string, roots ...string) (func(), error) {
	release, err := AcquireMutationLocks(ctx, roots...)
	if err != nil {
		return nil, err
	}
	if mutationGuardLockedHook != nil {
		mutationGuardLockedHook()
	}
	pending, err := PendingTransactions(ctx, journalDir, roots...)
	if err != nil {
		release()
		return nil, err
	}
	if len(pending) > 0 {
		release()
		return nil, fmt.Errorf("workspace has pending transaction %s; recover it before mutating", pending[0].ID)
	}
	return release, nil
}

// ApplyMove executes a previously validated move; dry-run leaves disk and memory unchanged.
//
//nolint:gocyclo // durable transaction step order is explicit for crash recovery.
func (w *Workspace) ApplyMove(ctx context.Context, plan *MovePlan, dryRun bool) error {
	if plan == nil || plan.data == nil {
		return fmt.Errorf("move plan is missing its private snapshot")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	d := plan.data
	roots := []string{d.source.Root, d.destination.Root}
	release, err := MutationGuard(ctx, w.JournalDir, roots...)
	if err != nil {
		return err
	}
	defer release()
	if err := validatePlanFingerprints(d); err != nil {
		return err
	}
	if !d.entry.IsSetup() {
		sourcePath := filepath.Join(d.source.Root, d.backupRel)
		if err := verifyPathParents(d.source.Root, sourcePath); err != nil {
			return err
		}
		selectedDirs := selectedDirectoryRoots(d.entry.Files, d.sourceFiles)
		if err := verifyMoveSourceSnapshot(sourcePath, d.sourceFiles, d.entry.IsFolder(), selectedDirs); err != nil {
			return fmt.Errorf("move source changed after planning: %w", err)
		}
		if err := verifyDestinationFree(d.destination.Root, d.backupRel, d.entry.Files); err != nil {
			return err
		}
	}
	if err := w.validateTargetsFromRoleMove(plan); err != nil {
		return err
	}
	if err := checkGitIgnore(ctx, d.destination.Root, ".tidydots-txn/"); err != nil {
		return err
	}
	if err := checkGitIgnore(ctx, d.source.Root, ".tidydots-txn-config-*"); err != nil {
		return err
	}
	if err := checkGitIgnore(ctx, d.destination.Root, ".tidydots-txn-config-*"); err != nil {
		return err
	}
	if !d.entry.IsSetup() {
		if err := revalidatePlannedArtifacts(d.source.Root, d.backupRel, d.entry.EffectiveMethod(), d.sourceFiles, d.stateRecords, d.stateKeys, nil); err != nil {
			return fmt.Errorf("move source artifacts changed after planning: %w", err)
		}
		if err := checkGeneratedIgnoresAt(ctx, d.source.Root, filepath.ToSlash(d.backupRel), d.sourceFiles); err != nil {
			return err
		}
		if err := checkGeneratedIgnoresAt(ctx, d.destination.Root, filepath.ToSlash(d.backupRel), d.sourceFiles); err != nil {
			return err
		}
	}
	journalDir, err := journalDirectory(w.JournalDir)
	if err != nil {
		return err
	}
	if err := validateJournalLocation(journalDir, d.source.Root, d.destination.Root); err != nil {
		return err
	}
	for _, link := range d.targets {
		target, err := filepath.Abs(link.path)
		if err != nil {
			return err
		}
		if pathsOverlap(target, journalDir) {
			return fmt.Errorf("transaction journal overlaps live deployment target %q", link.path)
		}
	}
	if err := ensureDurablePrivateDir(journalDir); err != nil {
		return err
	}
	for _, root := range roots {
		if err := syncDirectory(root); err != nil {
			return fmt.Errorf("filesystem does not support required durable directory updates for %s: %w", root, err)
		}
	}
	for _, link := range d.targets {
		if err := syncDirectory(filepath.Dir(link.path)); err != nil {
			return fmt.Errorf("filesystem does not support required durable link updates for %s: %w", link.path, err)
		}
	}
	id, err := transactionID()
	if err != nil {
		return err
	}
	for i := range d.targets {
		dir := filepath.Dir(d.targets[i].path)
		d.targets[i].forwardTemp = filepath.Join(dir, fmt.Sprintf(".tidydots-txn-link-%s-%d-forward", id, i))
		d.targets[i].rollbackTemp = filepath.Join(dir, fmt.Sprintf(".tidydots-txn-link-%s-%d-rollback", id, i))
	}
	j := moveJournal{Version: 1, ID: id, Phase: "prepared", PublicRoot: repositoryByRole(w, Public).Root, PrivateRoot: repositoryByRole(w, Private).Root, SourceRoot: d.source.Root, DestinationRoot: d.destination.Root, SourceRef: plan.SourceRef, DestinationRef: plan.DestinationRef, PublicConfig: repositoryByRole(w, Public).ConfigPath, PrivateConfig: repositoryByRole(w, Private).ConfigPath, PublicBefore: d.publicConfig, PrivateBefore: d.privateConfig, SourceRel: d.backupRel, DestinationRel: d.backupRel, SourceMethod: d.entry.EffectiveMethod(), Files: d.sourceFiles, Links: d.targets, Created: time.Now().UTC().Format(time.RFC3339Nano)}
	j.StateKeys = append([]string(nil), d.stateKeys...)
	j.StateRecords = cloneRenderRecords(d.stateRecords)
	_, statErr := os.Stat(filepath.Join(d.destination.Root, ".tidydots.db"))
	j.DestinationStateExisted = statErr == nil
	j.WholeFolder = d.entry.IsFolder() && !d.entry.IsSetup()
	j.SelectedDirs = selectedDirectoryRoots(d.entry.Files, d.sourceFiles)
	j.PublicAfter, j.PrivateAfter, err = moveConfigBytes(plan)
	if err != nil {
		return err
	}
	if err := writeJournal(journalDir, j); err != nil {
		return err
	}
	fail := func(cause error) error {
		if errors.Is(cause, errSimulatedCrash) {
			outcome := OutcomePending
			if j.Committed {
				outcome = OutcomeCommittedCleanupPending
			}
			return &TransactionError{ID: id, Outcome: outcome, Cause: cause}
		}
		rollbackErr := rollbackMove(ctx, journalDir, &j)
		if rollbackErr != nil {
			return &TransactionError{ID: id, Outcome: OutcomePending, Cause: errors.Join(cause, rollbackErr)}
		}
		return &TransactionError{ID: id, Outcome: OutcomeRolledBack, Cause: cause}
	}
	stage := filepath.Join(d.destination.Root, ".tidydots-txn", id, "source")
	type transactionStep struct {
		name string
		run  func() error
	}
	steps := make([]transactionStep, 0, 5)
	if !d.entry.IsSetup() {
		steps = append(steps, transactionStep{"stage-source", func() error {
			j.Intent = "stage-source"
			if err := writeJournal(journalDir, j); err != nil {
				return err
			}
			if err := injectFault("stage-source-before"); err != nil {
				return err
			}
			if err := verifyMoveSourceSnapshot(filepath.Join(d.source.Root, d.backupRel), d.sourceFiles, j.WholeFolder, j.SelectedDirs); err != nil {
				return fmt.Errorf("move source changed before staging: %w", err)
			}
			if err := verifyPathParents(d.destination.Root, stage); err != nil {
				return err
			}
			if err := ensureDurableDirectory(filepath.Dir(stage), d.destination.Root); err != nil {
				return err
			}
			if err := copySnapshot(stage, d.sourceFiles); err != nil {
				return err
			}
			if !j.WholeFolder {
				if err := verifySelectedSubtrees(stage, d.sourceFiles, j.SelectedDirs); err != nil {
					return fmt.Errorf("staged selected-directory membership changed: %w", err)
				}
			}
			return syncSnapshotTree(stage)
		}}, transactionStep{"publish-source", func() error {
			j.Intent = "publish-source"
			if err := writeJournal(journalDir, j); err != nil {
				return err
			}
			if err := injectFault("publish-source-before"); err != nil {
				return err
			}
			if err := verifyMoveSourceSnapshot(filepath.Join(d.source.Root, d.backupRel), d.sourceFiles, j.WholeFolder, j.SelectedDirs); err != nil {
				return fmt.Errorf("move source changed before publication: %w", err)
			}
			dest := filepath.Join(d.destination.Root, d.backupRel)
			if err := verifyMoveDestinationParents(d.destination.Root, d.backupRel, d.entry.Files); err != nil {
				return err
			}
			if !j.WholeFolder {
				if err := verifySelectedSubtrees(stage, d.sourceFiles, j.SelectedDirs); err != nil {
					return fmt.Errorf("staged selected-directory membership changed before publication: %w", err)
				}
			}
			if err := ensureDurableDirectory(filepath.Dir(dest), d.destination.Root); err != nil {
				return err
			}
			if err := publishSnapshot(stage, dest, d.sourceFiles, j.WholeFolder, d.destination.Root); err != nil {
				return err
			}
			return nil
		}}, transactionStep{"transfer-state", func() error {
			j.Intent = "transfer-state"
			if err := writeJournal(journalDir, j); err != nil {
				return err
			}
			if err := injectFault("transfer-state-before"); err != nil {
				return err
			}
			return importMoveState(ctx, id, &j)
		}}, transactionStep{"retarget-links", func() error {
			j.Intent = "retarget-links"
			if err := writeJournal(journalDir, j); err != nil {
				return err
			}
			if err := injectFault("retarget-links-before"); err != nil {
				return err
			}
			for _, link := range d.targets {
				if err := rejectSourceSymlinkParents(link.path, d.source.Root); err != nil {
					return err
				}
				if err := writeLink(link); err != nil {
					return err
				}
			}
			return nil
		}})
	}
	for _, role := range []Role{Public, Private} {
		steps = append(steps, transactionStep{"publish-" + string(role), func() error {
			return publishMoveManifest(ctx, journalDir, &j, role)
		}})
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			return fail(err)
		}
		if err := injectFault(step.name + "-after"); err != nil {
			return fail(err)
		}
		j.Phase = step.name
		j.Intent = ""
		if err := writeJournal(journalDir, j); err != nil {
			return fail(err)
		}
		if err := injectFault(step.name + "-recorded"); err != nil {
			return fail(err)
		}
	}
	if !d.entry.IsSetup() {
		if err := verifyMoveSourceSnapshot(filepath.Join(d.source.Root, d.backupRel), d.sourceFiles, j.WholeFolder, j.SelectedDirs); err != nil {
			return fail(fmt.Errorf("move source changed before commit: %w", err))
		}
		if err := revalidateMoveSourceState(ctx, &j); err != nil {
			return fail(err)
		}
	}
	if err := verifyCommittedOwnership(ctx, &j); err != nil {
		return fail(err)
	}
	if err := publishCommitMarker(journalDir, &j); err != nil {
		var transactionErr *TransactionError
		if errors.As(err, &transactionErr) {
			return transactionErr
		}
		return fail(err)
	}
	if err := injectFault("commit-marker-after"); err != nil {
		return fail(err)
	}
	if err := cleanupCommitted(journalDir, &j); err != nil {
		return &TransactionError{ID: id, Outcome: OutcomeCommittedCleanupPending, Cause: err}
	}
	if err := w.reload(); err != nil {
		return &TransactionError{ID: id, Outcome: OutcomeCommittedCleanupPending, Cause: err}
	}
	return nil
}

func publishMoveManifest(ctx context.Context, journalDir string, journal *moveJournal, role Role) error {
	journal.Intent = "publish-" + string(role)
	if err := writeJournal(journalDir, *journal); err != nil {
		return err
	}
	if err := injectFault(journal.Intent + "-before"); err != nil {
		return err
	}
	if err := revalidateMoveInputs(ctx, journal); err != nil {
		return err
	}
	path, root, before, after := journal.PublicConfig, journal.PublicRoot, journal.PublicBefore, journal.PublicAfter
	if role == Private {
		path, root, before, after = journal.PrivateConfig, journal.PrivateRoot, journal.PrivateBefore, journal.PrivateAfter
	}
	otherPath, otherExpected := journal.PrivateConfig, journal.PrivateBefore
	if role == Private {
		otherPath, otherExpected = journal.PublicConfig, journal.PublicAfter
	}
	if err := verifyConfigIsExpected(path, before); err != nil {
		return err
	}
	if err := verifyConfigIsExpected(otherPath, otherExpected); err != nil {
		return err
	}
	if err := verifyPathParents(root, path); err != nil {
		return err
	}
	return atomicWrite(path, after)
}

func (w *Workspace) validateTargetsFromRoleMove(plan *MovePlan) error {
	if plan.data.entry.IsSetup() {
		return nil
	}
	pub, priv, err := moveConfigBytes(plan)
	if err != nil {
		return err
	}
	candidate := &Workspace{}
	for _, repo := range w.Repositories {
		cfg := new(config.Config)
		data := pub
		if repo.Role == Private {
			data = priv
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return err
		}
		copyRepo := *repo
		copyRepo.Config = cfg
		candidate.Repositories = append(candidate.Repositories, &copyRepo)
	}
	if _, err := candidate.Applications(); err != nil {
		return err
	}
	return candidate.ValidateTargets(plan.data.platform)
}
func injectFault(phase string) error {
	if faultHook != nil {
		return faultHook(phase)
	}
	return nil
}

func journalDirectory(override string) (string, error) {
	if override != "" {
		p, e := filepath.Abs(override)
		return p, e
	}
	base, e := os.UserConfigDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(base, "tidydots", "transactions"), nil
}
func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700) //nolint:gosec // journal storage must be owner-only
}
func transactionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func writeJournal(dir string, j moveJournal) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, j.ID+".json")
	if err := validateJournalStorage(dir, path, j); err != nil {
		return err
	}
	name, err := writeJournalTemp(dir, data)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(name) }()
	return publishJournalFile(dir, name, path, j)
}

func validateJournalStorage(dir, path string, j moveJournal) error {
	if _, err := verifyProtectedDirectory(dir, j.PublicRoot, j.PrivateRoot); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("transaction journal directory is not a protected regular directory")
	}
	existing, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !existing.Mode().IsRegular() || existing.Mode()&os.ModeSymlink != 0 || existing.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("transaction journal is not a protected regular file")
	}
	return nil
}

func writeJournalTemp(dir string, data []byte) (string, error) {
	tmp, err := os.CreateTemp(dir, ".journal-")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func publishJournalFile(dir, name, path string, j moveJournal) error {
	if err := os.Rename(name, path); err != nil {
		return err
	}
	if j.Committed && j.Phase == transactionCommittedPhase {
		if err := injectFault("commit-marker-directory-sync-before"); err != nil {
			return err
		}
	}
	return syncDirectory(dir)
}

func publishCommitMarker(dir string, j *moveJournal) error {
	previous := *j
	j.Committed = true
	j.Phase = transactionCommittedPhase
	if err := writeJournal(dir, *j); err == nil {
		return nil
	} else {
		journalPath := filepath.Join(dir, j.ID+".json")
		published, readErr := readRecoveryJournal(journalPath, j.ID)
		if readErr != nil {
			return &TransactionError{ID: j.ID, Outcome: OutcomeCommitIndeterminate, Cause: errors.Join(err, readErr)}
		}
		if published.Committed {
			*j = published
			return &TransactionError{ID: j.ID, Outcome: OutcomeCommittedCleanupPending, Cause: err}
		}
		*j = previous
		return err
	}
}

var _ = yaml.Marshal
