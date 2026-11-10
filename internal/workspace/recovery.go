package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AntoineGS/tidydots/internal/config"
	"gopkg.in/yaml.v3"
)

// Recover rolls back an uncommitted transaction or finishes cleanup after its
// durable commit point. Replaying recovery is safe.
//
//nolint:gocyclo // recovery validates identity, roots and commit state before replay.
func Recover(ctx context.Context, journalDir, id string, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id == "" || strings.ContainsAny(id, "/\\") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid transaction ID")
	}
	dir, err := journalDirectory(journalDir)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, id+".json")
	j, err := readRecoveryJournal(path, id)
	if err != nil {
		return err
	}
	if err := validateJournalLocation(dir, j.PublicRoot, j.PrivateRoot); err != nil {
		return err
	}
	if err := validateJournalRoots(&j); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	roots := make([]string, 0, 2)
	if j.PublicRoot != "" {
		roots = append(roots, j.PublicRoot)
	}
	if j.PrivateRoot != "" {
		roots = append(roots, j.PrivateRoot)
	}
	release, err := AcquireMutationLocks(ctx, roots...)
	if err != nil {
		return err
	}
	defer release()
	if recoveryLockedHook != nil {
		recoveryLockedHook()
	}
	latest, err := readRecoveryJournal(path, id)
	if err != nil {
		return err
	}
	if latest.PublicRoot != j.PublicRoot || latest.PrivateRoot != j.PrivateRoot {
		return fmt.Errorf("transaction journal roots changed while acquiring recovery locks")
	}
	j = latest
	if err := validateJournalLocation(dir, j.PublicRoot, j.PrivateRoot); err != nil {
		return err
	}
	if err := validateJournalRoots(&j); err != nil {
		return err
	}
	if j.Committed {
		if err := cleanupCommitted(dir, &j); err != nil {
			return &TransactionError{ID: id, Outcome: OutcomeCommittedCleanupPending, Cause: err}
		}
		return nil
	}
	if err := rollbackMove(ctx, dir, &j); err != nil {
		return &TransactionError{ID: id, Outcome: OutcomePending, Cause: err}
	}
	return nil
}

func readRecoveryJournal(path, id string) (moveJournal, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return moveJournal{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return moveJournal{}, fmt.Errorf("transaction journal is not a protected regular file")
	}
	if !info.Mode().IsRegular() {
		return moveJournal{}, fmt.Errorf("transaction journal is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return moveJournal{}, err
	}
	var j moveJournal
	if err := json.Unmarshal(data, &j); err != nil {
		return moveJournal{}, fmt.Errorf("read transaction journal: %w", err)
	}
	if j.ID != id || j.Version != 1 {
		return moveJournal{}, fmt.Errorf("transaction journal identity/version mismatch")
	}
	return j, nil
}

func validateJournalLocation(dir string, roots ...string) error {
	_, err := verifyProtectedDirectory(dir, roots...)
	return err
}

//nolint:gocyclo // every persisted path is constrained to its registered root.
func validateJournalRoots(j *moveJournal) error {
	if j.Kind != transactionDocuments && (j.PublicRoot == "" || j.PrivateRoot == "" || rootsOverlap(j.PublicRoot, j.PrivateRoot)) {
		return fmt.Errorf("transaction journal has invalid repository roots")
	}
	if j.Kind == transactionDocuments && j.PublicRoot == "" && j.PrivateRoot == "" {
		return fmt.Errorf("document journal has no roots")
	}
	for _, root := range []string{j.PublicRoot, j.PrivateRoot} {
		if root == "" {
			continue
		}
		canonical, err := canonicalRoot(root)
		if err != nil {
			return err
		}
		if canonical != root {
			return fmt.Errorf("transaction repository roots changed since journaling")
		}
	}
	if j.PublicRoot != "" && j.PublicConfig != filepath.Join(j.PublicRoot, "tidydots.yaml") || j.PrivateRoot != "" && j.PrivateConfig != filepath.Join(j.PrivateRoot, "tidydots.yaml") {
		return fmt.Errorf("transaction journal configuration paths are outside their roots")
	}
	if j.SourceRoot != "" && (j.SourceRoot != j.PublicRoot && j.SourceRoot != j.PrivateRoot) {
		return fmt.Errorf("transaction source root is not registered")
	}
	if j.DestinationRoot != "" && (j.DestinationRoot != j.PublicRoot && j.DestinationRoot != j.PrivateRoot) {
		return fmt.Errorf("transaction destination root is not registered")
	}
	for _, item := range []struct{ root, path string }{
		{j.PublicRoot, j.PublicConfig}, {j.PrivateRoot, j.PrivateConfig},
	} {
		if item.path == "" {
			continue
		}
		if err := verifyPathParents(item.root, item.path); err != nil {
			return fmt.Errorf("unsafe journal configuration path: %w", err)
		}
		info, err := os.Lstat(item.path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("journal configuration path is not a regular file")
		}
	}
	if j.SourceRoot != "" {
		source := filepath.Join(j.SourceRoot, j.SourceRel)
		if err := verifyPathParents(j.SourceRoot, source); err != nil {
			return fmt.Errorf("unsafe journal source path: %w", err)
		}
	}
	if j.DestinationRoot != "" && j.DestinationRel != "" {
		destination := filepath.Join(j.DestinationRoot, j.DestinationRel)
		if err := verifyPathParents(j.DestinationRoot, destination); err != nil {
			return fmt.Errorf("unsafe journal destination path: %w", err)
		}
	}
	if j.DestinationRoot != "" {
		stage := filepath.Join(j.DestinationRoot, ".tidydots-txn", j.ID, "source")
		if err := verifyPathParents(j.DestinationRoot, stage); err != nil {
			return fmt.Errorf("unsafe journal stage path: %w", err)
		}
	}
	return nil
}

//nolint:gocyclo // constructs and validates the two independent owner documents.
func moveConfigBytes(plan *MovePlan) ([]byte, []byte, error) {
	d := plan.data
	srcCfg := cloneConfig(d.source.Config)
	dstCfg := cloneConfig(d.destination.Config)
	sourceIndex := -1
	entryIndex := -1
	for i := range srcCfg.Applications {
		if srcCfg.Applications[i].Name == d.app.Name {
			sourceIndex = i
			for j := range srcCfg.Applications[i].Entries {
				if srcCfg.Applications[i].Entries[j].Name == d.entry.Name {
					entryIndex = j
					break
				}
			}
			break
		}
	}
	if sourceIndex < 0 || entryIndex < 0 {
		return nil, nil, fmt.Errorf("source definition changed after planning")
	}
	srcCfg.Applications[sourceIndex].Entries = append(srcCfg.Applications[sourceIndex].Entries[:entryIndex], srcCfg.Applications[sourceIndex].Entries[entryIndex+1:]...)
	if len(srcCfg.Applications[sourceIndex].Entries) == 0 && srcCfg.Applications[sourceIndex].Package == nil {
		srcCfg.Applications = append(srcCfg.Applications[:sourceIndex], srcCfg.Applications[sourceIndex+1:]...)
	}
	destApp := -1
	for i := range dstCfg.Applications {
		if dstCfg.Applications[i].Name == d.app.Name {
			destApp = i
			break
		}
	}
	if destApp < 0 {
		dstCfg.Applications = append(dstCfg.Applications, config.Application{Name: d.app.Name, When: d.app.When})
		destApp = len(dstCfg.Applications) - 1
	}
	dstCfg.Applications[destApp].Entries = append(dstCfg.Applications[destApp].Entries, d.newEntry)
	if errs := config.ValidateConfig(srcCfg); len(errs) > 0 {
		return nil, nil, fmt.Errorf("source config after move is invalid: %w", errs[0])
	}
	if errs := config.ValidateConfig(dstCfg); len(errs) > 0 {
		return nil, nil, fmt.Errorf("destination config after move is invalid: %w", errs[0])
	}
	sourceBytes, err := yaml.Marshal(srcCfg)
	if err != nil {
		return nil, nil, err
	}
	destinationBytes, err := yaml.Marshal(dstCfg)
	if err != nil {
		return nil, nil, err
	}
	if d.source.Role == Public {
		return sourceBytes, destinationBytes, nil
	}
	return destinationBytes, sourceBytes, nil
}

func validatePlanFingerprints(d *moveData) error {
	for _, repo := range []*Repository{d.source, d.destination} {
		data, err := os.ReadFile(repo.ConfigPath)
		if err != nil {
			return err
		}
		want := digest(data)
		if repo == d.source {
			expected := digestConfigSnapshot(d, repo)
			if want != expected {
				return fmt.Errorf("%s configuration changed after move planning", repo.Role)
			}
		} else if want != digestConfigSnapshot(d, repo) {
			return fmt.Errorf("%s configuration changed after move planning", repo.Role)
		}
	}
	return nil
}
func digestConfigSnapshot(d *moveData, r *Repository) string {
	if r.Role == Public {
		return d.publicHash
	}
	return d.privateHash
}

//nolint:gocyclo // replayable rollback preserves before-images on external conflicts.
func rollbackMove(ctx context.Context, dir string, j *moveJournal) error {
	if err := injectFault("rollback-before"); err != nil {
		return err
	}
	j.Intent = "rollback-state"
	if err := writeJournal(dir, *j); err != nil {
		return err
	}
	if err := rollbackMoveState(ctx, j); err != nil {
		return err
	}
	j.Phase = j.Intent
	j.Intent = ""
	if err := writeJournal(dir, *j); err != nil {
		return err
	}
	if j.Kind == transactionDocuments {
		j.Intent = "rollback-public-config"
		if err := writeJournal(dir, *j); err != nil {
			return err
		}
		if err := restoreConfigIfChanged(j.PublicConfig, j.PublicBefore, j.PublicAfter); err != nil {
			return err
		}
		if j.PrivateConfig != "" {
			j.Intent = "rollback-private-config"
			if err := writeJournal(dir, *j); err != nil {
				return err
			}
			if err := restoreConfigIfChanged(j.PrivateConfig, j.PrivateBefore, j.PrivateAfter); err != nil {
				return err
			}
		}
		if err := os.Remove(filepath.Join(dir, j.ID+".json")); err != nil && !os.IsNotExist(err) {
			return err
		}
		return syncDirectory(dir)
	}
	for i := len(j.Links) - 1; i >= 0; i-- {
		if err := rejectSourceSymlinkParents(j.Links[i].path, j.SourceRoot); err != nil {
			return err
		}
		j.Intent = fmt.Sprintf("rollback-link-%d", i)
		if err := writeJournal(dir, *j); err != nil {
			return err
		}
		if _, err := os.Lstat(j.Links[i].path); err == nil {
			if err := restoreLink(j.Links[i]); err != nil {
				return err
			}
		}
	}
	j.Intent = "rollback-public-config"
	if err := writeJournal(dir, *j); err != nil {
		return err
	}
	if err := restoreConfigIfChanged(j.PublicConfig, j.PublicBefore, j.PublicAfter); err != nil {
		return err
	}
	if err := injectFault("rollback-public-config-after"); err != nil {
		return err
	}
	j.Intent = "rollback-private-config"
	if err := writeJournal(dir, *j); err != nil {
		return err
	}
	if err := restoreConfigIfChanged(j.PrivateConfig, j.PrivateBefore, j.PrivateAfter); err != nil {
		return err
	}
	if err := injectFault("rollback-private-config-after"); err != nil {
		return err
	}
	dest := filepath.Join(j.DestinationRoot, j.DestinationRel)
	if err := rollbackDestinationSnapshot(dir, dest, j); err != nil {
		return fmt.Errorf("destination changed during rollback: %w", err)
	}
	if err := removeStageDirectory(j.DestinationRoot, j.ID); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, j.ID+".json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDirectory(dir)
}

func restoreConfigIfChanged(path string, before, after []byte) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if digest(current) == digest(before) {
		return nil
	}
	if digest(current) != digest(after) {
		return fmt.Errorf("configuration changed outside transaction; refusing rollback: %s", path)
	}
	return atomicWrite(path, before)
}

func verifyConfigIsExpected(path string, expected []byte) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if digest(current) != digest(expected) {
		return fmt.Errorf("configuration changed outside transaction; refusing to overwrite: %s", path)
	}
	return nil
}

func rollbackDestinationSnapshot(dir, root string, j *moveJournal) error {
	rolled := map[string]bool{}
	for _, p := range j.RolledBackPaths {
		rolled[p] = true
	}
	for i := len(j.Files) - 1; i >= 0; i-- {
		item := j.Files[i]
		if rolled[item.rel] {
			continue
		}
		path := root
		if item.rel != "." {
			path = filepath.Join(root, item.rel)
		}
		if err := verifyPathParents(j.DestinationRoot, path); err != nil {
			return err
		}
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := verifyOneSnapshot(path, item); err != nil {
			return err
		}
		j.Intent = "rollback-destination:" + item.rel
		if err := writeJournal(dir, *j); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return err
		}
		j.RolledBackPaths = append(j.RolledBackPaths, item.rel)
		j.Phase = "rollback-destination"
		j.Intent = ""
		if err := writeJournal(dir, *j); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocyclo // cleanup remains resumable after any individual phase fails.
func cleanupCommitted(dir string, j *moveJournal) error {
	if err := verifyCommittedOwnership(context.Background(), j); err != nil {
		return fmt.Errorf("committed ownership no longer matches; retaining source and journal: %w", err)
	}
	if j.Kind == transactionDocuments {
		if err := os.Remove(filepath.Join(dir, j.ID+".json")); err != nil && !os.IsNotExist(err) {
			return err
		}
		return syncDirectory(dir)
	}
	if err := writeJournal(dir, *j); err != nil {
		return err
	}
	if err := injectFault("cleanup-before"); err != nil {
		return err
	}
	// A replay may resume a source-removal intent. Preserve that per-path
	// intent: cleanupSourceSnapshot needs it to distinguish a removal that
	// completed just before a crash from an external change.
	if !strings.HasPrefix(j.Intent, "cleanup-source:") {
		j.Intent = "cleanup-state"
		if err := writeJournal(dir, *j); err != nil {
			return err
		}
		if err := cleanupMoveState(context.Background(), j); err != nil {
			return err
		}
		j.StateCleaned = true
		if err := writeJournal(dir, *j); err != nil {
			return err
		}
		if err := injectFault("cleanup-state-after"); err != nil {
			return err
		}
		j.Phase = j.Intent
		j.Intent = ""
		if err := writeJournal(dir, *j); err != nil {
			return err
		}
	}
	if err := cleanupSourceSnapshot(dir, j); err != nil {
		return err
	}
	if err := removeStageDirectory(j.DestinationRoot, j.ID); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, j.ID+".json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncDirectory(dir)
}

func removeStageDirectory(root, id string) error {
	path := filepath.Join(root, ".tidydots-txn", id)
	if err := verifyPathParents(root, path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("transaction stage path is not a regular directory")
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func verifyCommittedOwnership(ctx context.Context, j *moveJournal) error {
	for _, item := range []struct {
		path string
		data []byte
	}{{j.PublicConfig, j.PublicAfter}, {j.PrivateConfig, j.PrivateAfter}} {
		if item.path == "" {
			continue
		}
		if err := verifyConfigIsExpected(item.path, item.data); err != nil {
			return err
		}
	}
	if j.Kind == transactionDocuments || len(j.Files) == 0 {
		return nil
	}
	destination := filepath.Join(j.DestinationRoot, j.DestinationRel)
	if err := verifySnapshotTreeAncestry(j.DestinationRoot, destination, j.Files); err != nil {
		return err
	}
	verify := verifySnapshotAt
	if j.WholeFolder {
		verify = verifyExactTree
	}
	if err := verify(destination, j.Files); err != nil {
		return fmt.Errorf("destination data changed or is missing: %w", err)
	}
	for _, link := range j.Links {
		current, err := os.Readlink(link.path)
		if err != nil {
			return fmt.Errorf("verify committed live link %q: %w", link.path, err)
		}
		if current != link.next {
			return fmt.Errorf("committed live link changed: %s", link.path)
		}
	}
	if err := verifyRemainingSourceAncestry(j.SourceRoot, filepath.Join(j.SourceRoot, j.SourceRel), j); err != nil {
		return fmt.Errorf("source snapshot ancestry changed: %w", err)
	}
	if err := revalidateCommittedSourceSet(ctx, j); err != nil {
		return err
	}
	if err := revalidateMoveIgnores(ctx, j); err != nil {
		return err
	}
	return verifyCommittedMoveState(ctx, j)
}

//nolint:gocyclo // per-path cleanup intent and replay reconciliation are explicit.
func cleanupSourceSnapshot(dir string, j *moveJournal) error {
	if err := verifyCommittedOwnership(context.Background(), j); err != nil {
		return fmt.Errorf("committed destination changed before source cleanup: %w", err)
	}
	root := filepath.Join(j.SourceRoot, j.SourceRel)
	if len(j.CleanedPaths) == 0 {
		if _, err := os.Lstat(root); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
		if err := verifySnapshotForCleanup(root, j); err != nil {
			return fmt.Errorf("source changed after commit; refusing cleanup: %w", err)
		}
	}
	cleaned := map[string]bool{}
	for _, p := range j.CleanedPaths {
		cleaned[p] = true
	}
	for i := len(j.Files) - 1; i >= 0; i-- {
		item := j.Files[i]
		path := root
		if item.rel != "." {
			path = filepath.Join(root, item.rel)
		}
		if err := verifyPathParents(j.SourceRoot, path); err != nil {
			return err
		}
		if cleaned[item.rel] {
			if _, err := os.Lstat(path); err == nil {
				return fmt.Errorf("completed source removal was recreated: %s", path)
			} else if !os.IsNotExist(err) {
				return err
			}
			continue
		}
		_, err := os.Lstat(path)
		if os.IsNotExist(err) {
			if j.Intent != "cleanup-source:"+item.rel {
				return fmt.Errorf("source path disappeared outside transaction: %s", path)
			}
			j.CleanedPaths = append(j.CleanedPaths, item.rel)
			j.Phase = "cleanup-source"
			j.Intent = ""
			if err := writeJournal(dir, *j); err != nil {
				return err
			}
			if err := injectFault("cleanup-source-recorded:" + item.rel); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if err := verifyOneSnapshot(path, item); err != nil {
			return fmt.Errorf("source changed after commit; refusing cleanup: %w", err)
		}
		if err := verifyCommittedOwnership(context.Background(), j); err != nil {
			return fmt.Errorf("committed destination changed before source cleanup: %w", err)
		}
		j.Intent = "cleanup-source:" + item.rel
		if err := writeJournal(dir, *j); err != nil {
			return err
		}
		if err := injectFault("cleanup-source-before:" + item.rel); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return err
		}
		if err := injectFault("cleanup-source-after:" + item.rel); err != nil {
			return err
		}
		j.CleanedPaths = append(j.CleanedPaths, item.rel)
		j.Phase = "cleanup-source"
		j.Intent = ""
		if err := writeJournal(dir, *j); err != nil {
			return err
		}
		if err := injectFault("cleanup-source-recorded:" + item.rel); err != nil {
			return err
		}
	}
	return nil
}

func verifySnapshotForCleanup(root string, j *moveJournal) error {
	cleaned := map[string]bool{}
	for _, p := range j.CleanedPaths {
		cleaned[p] = true
	}
	if strings.HasPrefix(j.Intent, "cleanup-source:") {
		cleaned[strings.TrimPrefix(j.Intent, "cleanup-source:")] = true
	}
	for _, f := range j.Files {
		if cleaned[f.rel] {
			continue
		}
		path := root
		if f.rel != "." {
			path = filepath.Join(root, f.rel)
		}
		if err := verifyOneSnapshot(path, f); err != nil {
			return err
		}
	}
	if !j.WholeFolder {
		return nil
	}
	expected := map[string]bool{}
	for _, f := range j.Files {
		if !cleaned[f.rel] {
			expected[filepath.Clean(f.rel)] = true
		}
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if !expected[filepath.Clean(rel)] {
			return fmt.Errorf("unexpected path appeared: %s", rel)
		}
		return nil
	})
}

func verifySnapshotAt(root string, files []fileSnapshot) error {
	for _, f := range files {
		path := root
		if f.rel != "." {
			path = filepath.Join(root, f.rel)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if f.dir {
			if !info.IsDir() || info.Mode().Perm() != f.mode {
				return fmt.Errorf("expected directory %s", path)
			}
			continue
		}
		if f.link != "" {
			link, e := os.Readlink(path)
			if e != nil || link != f.link {
				return fmt.Errorf("symlink changed: %s", path)
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("file type changed: %s", path)
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if digest(data) != digest(f.data) || info.Mode().Perm() != f.mode {
			return fmt.Errorf("file changed: %s", path)
		}
	}
	return nil
}

//nolint:gocyclo // explicit and whole-folder snapshots have distinct durability paths.
func publishSnapshot(stage, destination string, files []fileSnapshot, wholeFolder bool, root string) error {
	if err := syncSnapshotTree(stage); err != nil {
		return err
	}
	if wholeFolder {
		if err := ensureDurableDirectory(filepath.Dir(destination), root); err != nil {
			return err
		}
		if err := os.Rename(stage, destination); err != nil {
			return err
		}
		return syncDirectoryParents(stage, destination)
	}
	if err := ensureDurableDirectory(destination, root); err != nil {
		return err
	}
	for _, f := range files {
		if f.rel == "." {
			continue
		}
		from := filepath.Join(stage, f.rel)
		to := filepath.Join(destination, f.rel)
		if f.dir {
			if err := ensureDurableDirectory(to, root); err != nil {
				return err
			}
			if err := os.Chmod(to, f.mode); err != nil {
				return err
			}
			if err := syncDirectory(to); err != nil {
				return err
			}
			continue
		}
		if err := ensureDurableDirectory(filepath.Dir(to), root); err != nil {
			return err
		}
		if _, err := os.Lstat(to); err == nil {
			return fmt.Errorf("destination file appeared during move: %s", to)
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(from, to); err != nil {
			return err
		}
		if err := syncDirectoryParents(from, to); err != nil {
			return err
		}
	}
	return syncSnapshotTree(destination)
}

func verifyExactTree(root string, files []fileSnapshot) error {
	if err := verifySnapshotAt(root, files); err != nil {
		return err
	}
	expected := map[string]bool{".": true}
	for _, f := range files {
		expected[filepath.Clean(f.rel)] = true
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if !expected[filepath.Clean(rel)] {
			return fmt.Errorf("unexpected path appeared: %s", rel)
		}
		return nil
	})
}

func verifyOneSnapshot(path string, f fileSnapshot) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if f.dir {
		if !info.IsDir() || info.Mode().Perm() != f.mode {
			return fmt.Errorf("expected directory %s", path)
		}
		return nil
	}
	if f.link != "" {
		link, e := os.Readlink(path)
		if e != nil || link != f.link {
			return fmt.Errorf("symlink changed: %s", path)
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("file type changed: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if digest(data) != digest(f.data) || info.Mode().Perm() != f.mode {
		return fmt.Errorf("file changed: %s", path)
	}
	return nil
}
