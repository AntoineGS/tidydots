// Package workspace composes independently owned public and private repository
// configurations without changing either source document.
package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AntoineGS/tidydots/internal/config"
)

// Role identifies where a repository-owned definition is stored.
type Role string

const (
	// Public is the primary, normally version-controlled repository.
	Public Role = "public"
	// Private is the optional secondary repository.
	Private Role = "private"
)

// EntryRef is a stable identifier for an entry, independent of view ordering.
type EntryRef struct {
	Repository Role
	App        string
	Name       string
}

// Repository is one source configuration and its canonical local paths.
type Repository struct {
	Role         Role
	Root         string
	ConfigPath   string
	ConfigDigest string
	Config       *config.Config
}

// Entry is a derived workspace entry with its owning source identity and
// app-level condition. Value retains the source document's portable paths.
type Entry struct {
	Ref     EntryRef
	Value   config.SubEntry
	AppWhen string
}

// Application is a merged, read-oriented view of same-named app contributions.
// It must not be saved as a combined config document.
type Application struct {
	Name         string
	Description  string
	Entries      []Entry
	Package      *config.EntryPackage
	PackageOwner Role
}

// Workspace owns the repositories participating in a merged view.
type Workspace struct {
	Repositories []*Repository
	// JournalDir overrides the private local transaction directory. It is
	// primarily useful to applications embedding Workspace and to tests.
	JournalDir string
}

// ValidateFresh verifies that every loaded repository still matches its
// manifest fingerprint. Call it after acquiring MutationGuard and before
// selecting, preflighting, or writing. If selection was prepared earlier, a
// mismatch requires callers to reload and discard that selection before any
// write. A missing fingerprint fails closed.
func (w *Workspace) ValidateFresh() error {
	if w == nil || len(w.Repositories) == 0 {
		return fmt.Errorf("workspace has no loaded repositories; reload before mutation")
	}
	for _, repo := range w.Repositories {
		if repo == nil {
			return fmt.Errorf("workspace contains an unloaded repository; reload before mutation")
		}
		if err := checkRepositoryFresh(repo); err != nil {
			return fmt.Errorf("validate %s repository freshness: %w", repo.Role, err)
		}
	}
	return nil
}

// Load opens the public repository and, when provided, an optional private
// repository. It neither creates repositories nor rewrites source paths.
func Load(publicDir, privateDir string) (*Workspace, error) {
	return loadWorkspace(publicDir, privateDir, true)
}

// LoadForInspection validates and loads the paired manifests without denying
// read-only inspection when a transaction journal is pending. Mutating methods
// on the returned workspace still enforce MutationGuard before writing.
func LoadForInspection(publicDir, privateDir string) (*Workspace, error) {
	return loadWorkspace(publicDir, privateDir, false)
}

func loadWorkspace(publicDir, privateDir string, denyPending bool) (*Workspace, error) {
	publicRoot, err := canonicalRepositoryRoot(publicDir)
	if err != nil {
		return nil, fmt.Errorf("resolve public repository: %w", err)
	}
	roots := []string{publicRoot}
	if privateDir != "" {
		privateRoot, err := canonicalRepositoryRoot(privateDir)
		if err != nil {
			return nil, fmt.Errorf("resolve private repository: %w", err)
		}
		roots = append(roots, privateRoot)
	}
	if denyPending {
		pending, err := PendingTransactions(context.Background(), "", roots...)
		if err != nil {
			return nil, err
		}
		if len(pending) > 0 {
			return nil, fmt.Errorf("workspace has pending transaction %s; recover it before loading for mutation", pending[0].ID)
		}
	}
	public, err := loadRepository(Public, publicDir)
	if err != nil {
		return nil, err
	}
	workspace := &Workspace{Repositories: []*Repository{public}}
	if privateDir == "" {
		if _, err := workspace.Applications(); err != nil {
			return nil, err
		}
		return workspace, nil
	}
	private, err := loadRepository(Private, privateDir)
	if err != nil {
		return nil, err
	}
	if rootsOverlap(public.Root, private.Root) {
		return nil, fmt.Errorf("public and private repository roots must be distinct and non-nested: %s and %s", public.Root, private.Root)
	}
	shared, err := shareGitObjectStore(public.Root, private.Root)
	if err != nil {
		return nil, fmt.Errorf("checking independent Git ownership: %w", err)
	}
	if shared {
		return nil, fmt.Errorf("public and private repositories share a Git object store; use independently initialized repositories, not linked worktrees")
	}
	workspace.Repositories = append(workspace.Repositories, private)
	if _, err := workspace.Applications(); err != nil {
		return nil, err
	}
	return workspace, nil
}

func canonicalRepositoryRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("repository path is empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository is not a directory: %s", canonical)
	}
	return filepath.Clean(canonical), nil
}

func canonicalRoot(root string) (string, error) { return canonicalRepositoryRoot(root) }

// shareGitObjectStore compares Git's canonical common directory without
// consulting remotes or performing any network operation. Non-Git roots remain
// supported; only two Git roots can share ownership history.
func shareGitObjectStore(left, right string) (bool, error) {
	git, err := exec.LookPath("git")
	if err != nil {
		return false, nil
	}
	common := func(root string) (string, bool, error) {
		cmd := exec.CommandContext(context.Background(), git, "rev-parse", "--git-common-dir")
		cmd.Dir = root
		output, err := cmd.Output()
		if err != nil {
			return "", false, nil
		}
		path := strings.TrimSpace(string(output))
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			return "", false, fmt.Errorf("canonicalize Git common directory for %s: %w", root, err)
		}
		return filepath.Clean(canonical), true, nil
	}
	a, aGit, err := common(left)
	if err != nil {
		return false, err
	}
	b, bGit, err := common(right)
	if err != nil {
		return false, err
	}
	return aGit && bGit && a == b, nil
}

func loadRepository(role Role, root string) (*Repository, error) {
	if root == "" {
		return nil, fmt.Errorf("%s repository path is empty", role)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve %s repository path %q: %w", role, root, err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve %s repository %q: %w", role, root, err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return nil, fmt.Errorf("inspect %s repository %q: %w", role, canonical, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s repository is not a directory: %s", role, canonical)
	}
	configPath := filepath.Join(canonical, "tidydots.yaml")
	bytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read %s repository config %s: %w", role, configPath, err)
	}
	loaded, err := config.Parse(bytes)
	if err != nil {
		return nil, fmt.Errorf("load %s repository config %s: %w", role, configPath, err)
	}
	return &Repository{Role: role, Root: filepath.Clean(canonical), ConfigPath: configPath, ConfigDigest: digest(bytes), Config: loaded}, nil
}

func rootsOverlap(left, right string) bool {
	return sameOrChild(left, right) || sameOrChild(right, left)
}

func sameOrChild(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && (rel == "." || (rel != ".." && !filepath.IsAbs(rel) && !hasParentPrefix(rel)))
}

func hasParentPrefix(path string) bool {
	separator := string(filepath.Separator)
	return len(path) >= len(separator)+2 && path[:len(separator)+2] == ".."+separator
}

// Repository returns the context registered for role.
func (w *Workspace) Repository(role Role) (*Repository, error) {
	for _, repository := range w.Repositories {
		if repository.Role == role {
			return repository, nil
		}
	}
	return nil, fmt.Errorf("%s repository is not enabled", role)
}

// Lookup resolves a stable entry reference to the source repository and source
// application/entry values. The returned config values are copies.
func (w *Workspace) Lookup(ref EntryRef) (*Repository, config.Application, config.SubEntry, error) {
	repository, err := w.Repository(ref.Repository)
	if err != nil {
		return nil, config.Application{}, config.SubEntry{}, err
	}
	for _, app := range repository.Config.Applications {
		if app.Name != ref.App {
			continue
		}
		for _, entry := range app.Entries {
			if entry.Name == ref.Name {
				return repository, app, entry, nil
			}
		}
	}
	return nil, config.Application{}, config.SubEntry{}, fmt.Errorf("entry %s/%s not found in %s repository", ref.App, ref.Name, ref.Repository)
}
