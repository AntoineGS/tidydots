package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// SaveEntry replaces one entry in its owning document. The workspace is
// reloaded only after durable publication; dry-run is completely read-only.
func (w *Workspace) SaveEntry(ctx context.Context, ref EntryRef, updated config.SubEntry, dryRun bool) error {
	repo, app, old, err := w.Lookup(ref)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkRepositoryFresh(repo); err != nil {
		return err
	}
	if updated.Name == "" {
		return fmt.Errorf("entry name must not be empty")
	}
	cfg := cloneConfig(repo.Config)
	for i := range cfg.Applications {
		if cfg.Applications[i].Name == app.Name {
			for j := range cfg.Applications[i].Entries {
				if cfg.Applications[i].Entries[j].Name == old.Name {
					cfg.Applications[i].Entries[j] = updated
					break
				}
			}
		}
	}
	if err := w.validateCandidate(repo, cfg); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	if err := w.saveRepositoryConfig(ctx, repo, cfg); err != nil {
		return err
	}
	return w.reload()
}

// DeleteEntry removes a definition from its owner; it does not delete files.
func (w *Workspace) DeleteEntry(ctx context.Context, ref EntryRef, dryRun bool) error {
	repo, app, _, err := w.Lookup(ref)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkRepositoryFresh(repo); err != nil {
		return err
	}
	cfg := cloneConfig(repo.Config)
	for i := range cfg.Applications {
		if cfg.Applications[i].Name != app.Name {
			continue
		}
		entries := cfg.Applications[i].Entries[:0]
		for _, e := range cfg.Applications[i].Entries {
			if e.Name != ref.Name {
				entries = append(entries, e)
			}
		}
		cfg.Applications[i].Entries = entries
		if len(entries) == 0 && cfg.Applications[i].Package == nil {
			cfg.Applications = append(cfg.Applications[:i], cfg.Applications[i+1:]...)
		}
		break
	}
	if err := w.validateCandidate(repo, cfg); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	if err := w.saveRepositoryConfig(ctx, repo, cfg); err != nil {
		return err
	}
	return w.reload()
}

// EditApplication edits only fields from one repository scope. A nil package
// pointer leaves package ownership unchanged; package updates remain scoped.
func (w *Workspace) EditApplication(ctx context.Context, role Role, name string, description, when *string, pkg **config.EntryPackage, dryRun bool) error {
	repo, err := w.Repository(role)
	if err != nil {
		return err
	}
	if err := checkRepositoryFresh(repo); err != nil {
		return err
	}
	cfg := cloneConfig(repo.Config)
	found := false
	for i := range cfg.Applications {
		app := &cfg.Applications[i]
		if app.Name != name {
			continue
		}
		found = true
		if description != nil {
			app.Description = *description
		}
		if when != nil {
			app.When = *when
		}
		if pkg != nil {
			app.Package = *pkg
		}
		break
	}
	if !found {
		return fmt.Errorf("application %q not found in %s repository", name, role)
	}
	if err := w.validateCandidate(repo, cfg); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	if err := w.saveRepositoryConfig(ctx, repo, cfg); err != nil {
		return err
	}
	return w.reload()
}

// SaveApplication saves one form's metadata in a selected repository scope.
// A name change renames every existing same-name contribution atomically.
// Updated.Entries is ignored: entry edits are separate owner-scoped operations.
// No missing repository contribution is created implicitly.
func (w *Workspace) SaveApplication(ctx context.Context, role Role, originalName string, updated config.Application, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	selected, err := w.Repository(role)
	if err != nil {
		return err
	}
	for _, repo := range w.Repositories {
		if err := checkRepositoryFresh(repo); err != nil {
			return err
		}
	}
	candidate, docs, err := buildApplicationSaveCandidate(w, selected, originalName, updated)
	if err != nil {
		return err
	}
	if err := validateApplicationWorkspace(candidate); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	return w.saveDocuments(ctx, docs)
}

func buildApplicationSaveCandidate(w *Workspace, selected *Repository, originalName string, updated config.Application) (*Workspace, []documentChange, error) {
	if originalName == "" || updated.Name == "" {
		return nil, nil, fmt.Errorf("application names must not be empty")
	}
	if !repositoryHasApplication(selected, originalName) {
		return nil, nil, fmt.Errorf("application %q not found in %s repository", originalName, selected.Role)
	}
	if updated.Name != originalName && workspaceHasApplication(w, updated.Name) {
		return nil, nil, fmt.Errorf("application %q already exists in another repository contribution", updated.Name)
	}
	candidate := &Workspace{}
	docs := make([]documentChange, 0, len(w.Repositories))
	for _, repo := range w.Repositories {
		cfg := cloneConfig(repo.Config)
		changed := renameApplicationContribution(cfg, originalName, updated.Name, repo == selected, updated)
		copyRepo := *repo
		copyRepo.Config = cfg
		candidate.Repositories = append(candidate.Repositories, &copyRepo)
		if changed {
			docs = append(docs, documentChange{repo: repo, cfg: cfg})
		}
	}
	return candidate, docs, nil
}

func repositoryHasApplication(repo *Repository, name string) bool {
	for _, app := range repo.Config.Applications {
		if app.Name == name {
			return true
		}
	}
	return false
}

func workspaceHasApplication(w *Workspace, name string) bool {
	for _, repo := range w.Repositories {
		if repositoryHasApplication(repo, name) {
			return true
		}
	}
	return false
}

func renameApplicationContribution(cfg *config.Config, from, to string, updateMetadata bool, updated config.Application) bool {
	changed := false
	for i := range cfg.Applications {
		app := &cfg.Applications[i]
		if app.Name != from {
			continue
		}
		app.Name = to
		changed = true
		if updateMetadata {
			app.Description = updated.Description
			app.When = updated.When
			app.Package = updated.Package
		}
	}
	return changed
}

func validateApplicationWorkspace(candidate *Workspace) error {
	if _, err := candidate.Applications(); err != nil {
		return err
	}
	if err := validateRepositoryConfigs(candidate.Repositories); err != nil {
		return err
	}
	return candidate.ValidateCrossRepositoryTargets(configuredPlatform())
}

// AddEntry adds an entry only to the requested owner document. If the app
// exists solely in the other scope, the new scope inherits its trimmed when
// condition and no package/description metadata.
//
//nolint:gocyclo // app creation, owner checks, validation and persistence are one scoped operation.
func (w *Workspace) AddEntry(ctx context.Context, role Role, appName string, entry config.SubEntry, dryRun bool) error {
	repo, err := w.Repository(role)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkRepositoryFresh(repo); err != nil {
		return err
	}
	if appName == "" || entry.Name == "" {
		return fmt.Errorf("application and entry names must not be empty")
	}
	cfg := cloneConfig(repo.Config)
	index := -1
	for i := range cfg.Applications {
		if cfg.Applications[i].Name == appName {
			index = i
			break
		}
	}
	if index < 0 {
		app := config.Application{Name: appName}
		for _, other := range w.Repositories {
			if other.Role == role {
				continue
			}
			for _, existing := range other.Config.Applications {
				if existing.Name == appName {
					app.When = strings.TrimSpace(existing.When)
					break
				}
			}
		}
		cfg.Applications = append(cfg.Applications, app)
		index = len(cfg.Applications) - 1
	}
	for _, existing := range cfg.Applications[index].Entries {
		if existing.Name == entry.Name {
			return fmt.Errorf("entry %q already exists in %s/%s", entry.Name, appName, role)
		}
	}
	cfg.Applications[index].Entries = append(cfg.Applications[index].Entries, entry)
	if err := w.validateCandidate(repo, cfg); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	return w.saveRepositoryConfig(ctx, repo, cfg)
}

// AddApplication adds a source-owned application block. App conditions remain
// independent across repository scopes; package uniqueness is workspace-wide.
func (w *Workspace) AddApplication(ctx context.Context, role Role, app config.Application, dryRun bool) error {
	repo, err := w.Repository(role)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkRepositoryFresh(repo); err != nil {
		return err
	}
	if app.Name == "" {
		return fmt.Errorf("application name must not be empty")
	}
	for _, existing := range repo.Config.Applications {
		if existing.Name == app.Name {
			return fmt.Errorf("application %q already exists in %s repository", app.Name, role)
		}
	}
	cfg := cloneConfig(repo.Config)
	cfg.Applications = append(cfg.Applications, app)
	if err := w.validateCandidate(repo, cfg); err != nil {
		return err
	}
	if dryRun {
		return nil
	}
	return w.saveRepositoryConfig(ctx, repo, cfg)
}

func (w *Workspace) validateCandidate(owner *Repository, cfg *config.Config) error {
	candidate := &Workspace{}
	for _, r := range w.Repositories {
		copyRepo := *r
		if r == owner {
			copyRepo.Config = cfg
		}
		candidate.Repositories = append(candidate.Repositories, &copyRepo)
	}
	if _, err := candidate.Applications(); err != nil {
		return err
	}
	if err := validateRepositoryConfigs(candidate.Repositories); err != nil {
		return err
	}
	return candidate.ValidateTargets(configuredPlatform())
}

func validateRepositoryConfigs(repositories []*Repository) error {
	for _, repo := range repositories {
		if repo == nil || repo.Config == nil {
			continue
		}
		if errs := config.ValidateConfig(repo.Config); len(errs) > 0 {
			return fmt.Errorf("invalid %s repository configuration: %w", repo.Role, errs[0])
		}
	}
	return nil
}

func configuredPlatform() *platform.Platform { return platform.Detect() }

func checkRepositoryFresh(repo *Repository) error {
	if repo.ConfigDigest == "" {
		return fmt.Errorf("repository %s has no loaded fingerprint; reload before editing", repo.Role)
	}
	current, err := os.ReadFile(repo.ConfigPath)
	if err != nil {
		return err
	}
	if digest(current) != repo.ConfigDigest {
		return fmt.Errorf("%s configuration changed outside tidydots; reload before editing", repo.Role)
	}
	return nil
}

// RenameApplication renames every enabled contribution in one journaled edit.
func (w *Workspace) RenameApplication(ctx context.Context, oldName, newName string, dryRun bool) error {
	if oldName == newName {
		return nil
	}
	return w.editMergedApplication(ctx, oldName, newName, false, dryRun)
}

// DeleteApplication deletes all enabled definitions without deleting source files.
func (w *Workspace) DeleteApplication(ctx context.Context, name string, dryRun bool) error {
	return w.editMergedApplication(ctx, name, "", true, dryRun)
}

type documentChange struct {
	repo *Repository
	cfg  *config.Config
}

//nolint:gocyclo // paired documents share one validated journal transaction.
func (w *Workspace) editMergedApplication(ctx context.Context, oldName, newName string, remove, dryRun bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var docs []documentChange
	for _, repo := range w.Repositories {
		if err := checkRepositoryFresh(repo); err != nil {
			return err
		}
		cfg := cloneConfig(repo.Config)
		changedDoc := false
		for i := range cfg.Applications {
			if cfg.Applications[i].Name != oldName {
				continue
			}
			if remove {
				cfg.Applications = append(cfg.Applications[:i], cfg.Applications[i+1:]...)
			} else {
				cfg.Applications[i].Name = newName
			}
			changedDoc = true
			break
		}
		if changedDoc {
			docs = append(docs, documentChange{repo, cfg})
		}
	}
	if len(docs) == 0 {
		return fmt.Errorf("application %q not found", oldName)
	}
	candidate := &Workspace{}
	for _, repo := range w.Repositories {
		copyRepo := *repo
		for _, doc := range docs {
			if doc.repo == repo {
				copyRepo.Config = doc.cfg
			}
		}
		candidate.Repositories = append(candidate.Repositories, &copyRepo)
	}
	if _, err := candidate.Applications(); err != nil {
		return err
	}
	if err := validateRepositoryConfigs(candidate.Repositories); err != nil {
		return err
	}
	if err := candidate.ValidateTargets(configuredPlatform()); err != nil {
		return err
	}
	if !remove {
		for _, repo := range w.Repositories {
			for _, app := range repo.Config.Applications {
				if app.Name == newName {
					return fmt.Errorf("application %q already exists in %s repository", newName, repo.Role)
				}
			}
		}
	}
	if dryRun {
		return nil
	}
	if len(docs) == 1 {
		if err := w.saveRepositoryConfig(ctx, docs[0].repo, docs[0].cfg); err != nil {
			return err
		}
		return w.reload()
	}
	return w.saveDocuments(ctx, docs)
}

func cloneConfig(c *config.Config) *config.Config {
	data, _ := yaml.Marshal(c)
	var result config.Config
	_ = yaml.Unmarshal(data, &result)
	result.BackupRoot = c.BackupRoot
	return &result
}

func (w *Workspace) saveRepositoryConfig(ctx context.Context, repo *Repository, cfg *config.Config) error {
	return w.saveDocuments(ctx, []documentChange{{repo, cfg}})
}

//nolint:gocyclo // all checks and rollback branches enforce durable document writes.
func (w *Workspace) saveDocuments(ctx context.Context, docs []documentChange) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, doc := range docs {
		if errs := config.ValidateConfig(doc.cfg); len(errs) > 0 {
			return fmt.Errorf("invalid %s repository configuration: %w", doc.repo.Role, errs[0])
		}
	}
	roots := make([]string, 0, len(w.Repositories))
	for _, r := range w.Repositories {
		roots = append(roots, r.Root)
	}
	release, err := MutationGuard(ctx, w.JournalDir, roots...)
	if err != nil {
		return err
	}
	defer release()
	for _, repo := range w.Repositories {
		if err := checkRepositoryFresh(repo); err != nil {
			return err
		}
	}
	candidate := &Workspace{}
	for _, repo := range w.Repositories {
		copyRepo := *repo
		for _, doc := range docs {
			if doc.repo == repo {
				copyRepo.Config = doc.cfg
				break
			}
		}
		candidate.Repositories = append(candidate.Repositories, &copyRepo)
	}
	if _, err := candidate.Applications(); err != nil {
		return err
	}
	if err := validateRepositoryConfigs(candidate.Repositories); err != nil {
		return err
	}
	if err := candidate.ValidateCrossRepositoryTargets(configuredPlatform()); err != nil {
		return err
	}
	journalDir, err := journalDirectory(w.JournalDir)
	if err != nil {
		return err
	}
	for _, root := range roots {
		if err := validateJournalLocation(journalDir, root); err != nil {
			return err
		}
	}
	if err := ensureDurablePrivateDir(journalDir); err != nil {
		return err
	}
	if err := syncDirectory(journalDir); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(journalDir)); err != nil {
		return err
	}
	for _, doc := range docs {
		if err := syncDirectory(filepath.Dir(doc.repo.ConfigPath)); err != nil {
			return fmt.Errorf("filesystem does not support durable config publication for %s: %w", doc.repo.Role, err)
		}
	}
	id, err := transactionID()
	if err != nil {
		return err
	}
	pub, priv := repositoryByRole(w, Public), repositoryByRole(w, Private)
	j := moveJournal{Version: 1, Kind: transactionDocuments, ID: id, Phase: "prepared"}
	if pub != nil {
		j.PublicRoot = pub.Root
		j.PublicConfig = pub.ConfigPath
		j.PublicBefore, err = os.ReadFile(pub.ConfigPath)
		if err != nil {
			return err
		}
		j.PublicAfter = append([]byte(nil), j.PublicBefore...)
	}
	if priv != nil {
		j.PrivateRoot = priv.Root
		j.PrivateConfig = priv.ConfigPath
		j.PrivateBefore, err = os.ReadFile(priv.ConfigPath)
		if err != nil {
			return err
		}
		j.PrivateAfter = append([]byte(nil), j.PrivateBefore...)
	}
	for _, doc := range docs {
		if err := checkGitIgnore(ctx, doc.repo.Root, ".tidydots-txn-config-*"); err != nil {
			return err
		}
		if err := checkRepositoryFresh(doc.repo); err != nil {
			return err
		}
		data, e := yaml.Marshal(doc.cfg)
		if e != nil {
			return e
		}
		if doc.repo.Role == Public {
			j.PublicAfter = data
		} else {
			j.PrivateAfter = data
		}
	}
	if err := writeJournal(journalDir, j); err != nil {
		return err
	}
	for _, doc := range docs {
		after := j.PublicAfter
		before := j.PublicBefore
		path := j.PublicConfig
		if doc.repo.Role == Private {
			after = j.PrivateAfter
			before = j.PrivateBefore
			path = j.PrivateConfig
		}
		_ = before
		j.Intent = "publish-" + string(doc.repo.Role)
		if err := writeJournal(journalDir, j); err != nil {
			return &TransactionError{ID: id, Outcome: OutcomePending, Cause: err}
		}
		if err := injectFault(j.Intent + "-before"); err != nil {
			return &TransactionError{ID: id, Outcome: OutcomePending, Cause: err}
		}
		if err := checkRepositoryFresh(doc.repo); err != nil {
			if rollbackErr := rollbackMove(ctx, journalDir, &j); rollbackErr != nil {
				return &TransactionError{ID: id, Outcome: OutcomePending, Cause: errors.Join(err, rollbackErr)}
			}
			return err
		}
		if err := atomicWrite(path, after); err != nil {
			if rollbackErr := rollbackMove(ctx, journalDir, &j); rollbackErr != nil {
				return &TransactionError{ID: id, Outcome: OutcomePending, Cause: errors.Join(err, rollbackErr)}
			}
			return &TransactionError{ID: id, Outcome: OutcomeRolledBack, Cause: err}
		}
		if err := injectFault(j.Intent + "-after"); err != nil {
			return &TransactionError{ID: id, Outcome: OutcomePending, Cause: err}
		}
		j.Phase = j.Intent
		j.Intent = ""
		if err := writeJournal(journalDir, j); err != nil {
			return &TransactionError{ID: id, Outcome: OutcomePending, Cause: err}
		}
	}
	if j.PublicConfig != "" {
		if err := verifyConfigIsExpected(j.PublicConfig, j.PublicAfter); err != nil {
			return &TransactionError{ID: id, Outcome: OutcomePending, Cause: err}
		}
	}
	if j.PrivateConfig != "" {
		if err := verifyConfigIsExpected(j.PrivateConfig, j.PrivateAfter); err != nil {
			return &TransactionError{ID: id, Outcome: OutcomePending, Cause: err}
		}
	}
	if err := publishCommitMarker(journalDir, &j); err != nil {
		var transactionErr *TransactionError
		if errors.As(err, &transactionErr) {
			return transactionErr
		}
		rollbackErr := rollbackMove(ctx, journalDir, &j)
		if rollbackErr != nil {
			return &TransactionError{ID: id, Outcome: OutcomePending, Cause: errors.Join(err, rollbackErr)}
		}
		return &TransactionError{ID: id, Outcome: OutcomeRolledBack, Cause: err}
	}
	if err := injectFault("documents-commit-marker-after"); err != nil {
		return &TransactionError{ID: id, Outcome: OutcomeCommittedCleanupPending, Cause: err}
	}
	if err := cleanupCommitted(journalDir, &j); err != nil {
		return &TransactionError{ID: id, Outcome: OutcomeCommittedCleanupPending, Cause: err}
	}
	return w.reload()
}

func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tidydots-txn-config-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func (w *Workspace) reload() error {
	loaded, err := Load(w.Repositories[0].Root, privateRoot(w))
	if err != nil {
		return err
	}
	w.Repositories = loaded.Repositories
	return nil
}
func privateRoot(w *Workspace) string {
	for _, r := range w.Repositories {
		if r.Role == Private {
			return r.Root
		}
	}
	return ""
}
