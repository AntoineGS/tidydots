package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/AntoineGS/tidydots/internal/config"
	tmpl "github.com/AntoineGS/tidydots/internal/template"
)

func (f fileSnapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Rel  string      `json:"rel"`
		Mode os.FileMode `json:"mode"`
		Data []byte      `json:"data,omitempty"`
		Link string      `json:"link,omitempty"`
		Dir  bool        `json:"dir,omitempty"`
	}{f.rel, f.mode, f.data, f.link, f.dir})
}
func (f *fileSnapshot) UnmarshalJSON(data []byte) error {
	var v struct {
		Rel  string      `json:"rel"`
		Mode os.FileMode `json:"mode"`
		Data []byte      `json:"data"`
		Link string      `json:"link"`
		Dir  bool        `json:"dir"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	f.rel, f.mode, f.data, f.link, f.dir = v.Rel, v.Mode, v.Data, v.Link, v.Dir
	return nil
}
func (l linkSnapshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Path string `json:"path"`
		Old  string `json:"old"`
		Next string `json:"next"`
	}{l.path, l.old, l.next})
}
func (l *linkSnapshot) UnmarshalJSON(data []byte) error {
	var v struct {
		Path string `json:"path"`
		Old  string `json:"old"`
		Next string `json:"next"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	l.path, l.old, l.next = v.Path, v.Old, v.Next
	return nil
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// RegisterGitExcludes installs the reserved transaction ignore rule in this
// repository's local exclude file. It never inspects remotes or uses network.
const (
	stateDatabaseName       = ".tidydots.db"
	transactionIgnoreDir    = ".tidydots-txn/"
	transactionConfigIgnore = ".tidydots-txn-config-*"
	templateRenderedSuffix  = ".rendered"
	templateConflictSuffix  = ".conflict"
)

// RegisterGitExcludes adds transaction and generated-file rules to local Git excludes.
func RegisterGitExcludes(root string) error {
	return registerGitExcludes(root)
}

func checkGeneratedIgnoresAt(ctx context.Context, root, backup string, files []fileSnapshot) error {
	git, hasGit, err := gitExecutableOrNonGit(root, "check generated-file ignores")
	if err != nil {
		return err
	}
	if !hasGit {
		return nil
	}
	cmd := exec.CommandContext(ctx, git, "rev-parse", "--show-toplevel")
	cmd.Dir = root
	if _, err := cmd.Output(); err != nil {
		if isNotGitRepository(err) {
			return nil
		}
		return fmt.Errorf("resolve Git root for %s: %w", root, err)
	}
	backup, err = cleanRepositoryRelative(backup)
	if err != nil {
		return fmt.Errorf("invalid backup path for ignore check: %w", err)
	}
	checks := []string{stateDatabaseName, transactionIgnoreDir, transactionConfigIgnore}
	for _, f := range files {
		if strings.HasSuffix(f.rel, ".tmpl.rendered") || strings.HasSuffix(f.rel, ".tmpl.conflict") || strings.HasSuffix(f.rel, ".tmpl.rendered.bak") {
			actual := filepath.Join(backup, f.rel)
			checks = append(checks, filepath.ToSlash(actual))
		}
	}
	for _, path := range uniqueStrings(checks) {
		cmd := exec.CommandContext(ctx, git, "check-ignore", "-q", "--no-index", "--", path)
		cmd.Dir = root
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git does not effectively ignore generated/transaction path %q in %s; add it to .gitignore or run RegisterGitExcludes before moving", path, root)
		}
	}
	return nil
}

func cleanRepositoryRelative(path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be repository-relative")
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository")
	}
	return clean, nil
}

func isNotGitRepository(err error) bool {
	if strings.Contains(err.Error(), "not a git repository") {
		return true
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && strings.Contains(string(exitErr.Stderr), "not a git repository")
}

func checkGitIgnore(ctx context.Context, root, path string) error {
	git, hasGit, err := gitExecutableOrNonGit(root, "check Git exclusion")
	if err != nil {
		return err
	}
	if !hasGit {
		return nil
	}
	probe := exec.CommandContext(ctx, git, "rev-parse", "--show-toplevel")
	probe.Dir = root
	if _, err := probe.Output(); err != nil {
		if isNotGitRepository(err) {
			return nil
		}
		return fmt.Errorf("resolve Git root for %s: %w", root, err)
	}
	cmd := exec.CommandContext(ctx, git, "check-ignore", "-q", "--no-index", path)
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git does not effectively ignore %q in %s; add it to .gitignore or run RegisterGitExcludes: %w", path, root, err)
	}
	return nil
}

//nolint:gocyclo // ownership proof must compare every complete source definition.
func checkExclusiveSources(w *Workspace, ref EntryRef, backup string, selected config.SubEntry) error {
	owned := func(path string) bool {
		if selected.IsFolder() {
			return pathIsWithin(backup, path) || backup == path
		}
		for _, name := range selected.Files {
			p := filepath.Clean(filepath.Join(backup, name))
			if p == path || pathIsWithin(p, path) || pathIsWithin(path, p) {
				return true
			}
		}
		return false
	}
	for _, repo := range w.Repositories {
		for _, app := range repo.Config.Applications {
			for _, entry := range app.Entries {
				if repo.Role == ref.Repository && app.Name == ref.App && entry.Name == ref.Name {
					continue
				}
				if !entry.IsConfig() {
					continue
				}
				other, err := staticBackup(repo, entry.Backup)
				if err != nil {
					return fmt.Errorf("cannot prove source ownership because %s/%s has a dynamic backup path", app.Name, entry.Name)
				}
				if entry.IsFolder() {
					if owned(other) || (selected.IsFolder() && pathIsWithin(other, backup)) {
						return fmt.Errorf("source is shared with %s/%s (%s)", app.Name, entry.Name, repo.Role)
					}
					continue
				}
				for _, name := range entry.Files {
					if owned(filepath.Clean(filepath.Join(other, name))) {
						return fmt.Errorf("source is shared with %s/%s (%s)", app.Name, entry.Name, repo.Role)
					}
				}
			}
		}
	}
	return nil
}

//nolint:gocyclo // safely classifies regular files, directories and source links.
func snapshotOwned(root string, files []string) ([]fileSnapshot, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect move source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("source backup may not be a symlink")
	}
	var paths []string
	if len(files) == 0 {
		paths = append(paths, ".")
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path == root {
				return nil
			}
			rel, e := filepath.Rel(root, path)
			if e != nil {
				return e
			}
			if forbiddenName(rel) {
				return fmt.Errorf("source contains protected control path %q", rel)
			}
			paths = append(paths, rel)
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		for _, name := range files {
			if err := safeRelative(name); err != nil {
				return nil, err
			}
			full := filepath.Join(root, name)
			if err := ensureNoSymlinkParents(root, full); err != nil {
				return nil, err
			}
			fi, err := os.Lstat(full)
			if err != nil {
				return nil, fmt.Errorf("selected source %q: %w", name, err)
			}
			if fi.IsDir() {
				err = filepath.WalkDir(full, func(path string, d fs.DirEntry, walkErr error) error {
					if walkErr != nil {
						return walkErr
					}
					rel, e := filepath.Rel(root, path)
					if e != nil {
						return e
					}
					if forbiddenName(rel) {
						return fmt.Errorf("source contains protected control path %q", rel)
					}
					paths = append(paths, rel)
					return nil
				})
				if err != nil {
					return nil, err
				}
			} else {
				paths = append(paths, name)
			}
			if strings.HasSuffix(name, ".tmpl") {
				alias := filepath.Join(filepath.Dir(name), tmpl.TargetName(name))
				aliasPath := filepath.Join(root, alias)
				if aliasInfo, e := os.Lstat(aliasPath); e == nil {
					if aliasInfo.Mode()&os.ModeSymlink != 0 {
						link, readErr := os.Readlink(aliasPath)
						if readErr != nil {
							return nil, readErr
						}
						rendered := name + templateRenderedSuffix
						resolved := filepath.Clean(filepath.Join(filepath.Dir(alias), link))
						if !filepath.IsAbs(link) && resolved == filepath.Clean(rendered) {
							paths = append(paths, alias)
							for _, suffix := range []string{templateRenderedSuffix, templateConflictSuffix, templateRenderedSuffix + ".bak"} {
								artifact := name + suffix
								artifactPath := filepath.Join(root, artifact)
								if _, artifactErr := os.Lstat(artifactPath); artifactErr == nil {
									paths = append(paths, artifact)
								} else if !os.IsNotExist(artifactErr) {
									return nil, artifactErr
								}
							}
						}
					}
				} else if !os.IsNotExist(e) {
					return nil, e
				}
			}
		}
	}
	seen := map[string]bool{}
	result := make([]fileSnapshot, 0, len(paths))
	for _, rel := range paths {
		rel = filepath.Clean(rel)
		if seen[rel] {
			continue
		}
		seen[rel] = true
		full := filepath.Join(root, rel)
		fi, e := os.Lstat(full)
		if e != nil {
			return nil, e
		}
		if forbiddenName(rel) {
			return nil, fmt.Errorf("source contains protected control path %q", rel)
		}
		s := fileSnapshot{rel: rel, mode: fi.Mode().Perm()}
		switch {
		case fi.IsDir():
			s.dir = true
		case fi.Mode()&os.ModeSymlink != 0:
			s.link, e = os.Readlink(full)
			if e != nil {
				return nil, e
			}
			if filepath.IsAbs(s.link) {
				return nil, fmt.Errorf("absolute source symlink %q is not movable", rel)
			}
		case fi.Mode().IsRegular():
			if linkedRegularFile(fi) {
				return nil, fmt.Errorf("multiply-linked source file %q is not movable", rel)
			}
			s.data, e = os.ReadFile(full)
			if e != nil {
				return nil, e
			}
		default:
			return nil, fmt.Errorf("special source file %q is not movable", rel)
		}
		result = append(result, s)
	}
	if len(files) == 0 && len(result) == 0 {
		result = append(result, fileSnapshot{rel: ".", mode: info.Mode().Perm(), dir: true})
	}
	if err := validateMovedSymlinks(result); err != nil {
		return nil, err
	}
	return result, nil
}

func linkedRegularFile(info os.FileInfo) bool {
	v := reflect.ValueOf(info.Sys())
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return false
	}
	field := v.FieldByName("Nlink")
	return field.IsValid() && field.CanUint() && field.Uint() > 1
}

func validateMovedSymlinks(files []fileSnapshot) error {
	types := make(map[string]fileSnapshot, len(files))
	for _, f := range files {
		types[filepath.Clean(f.rel)] = f
	}
	// Explicitly selected nested files have structural parent directories in
	// the published tree even when those directories are not owned snapshots.
	// Add them only to the resolver graph, never to the transferred/owned set.
	for _, f := range files {
		parent := filepath.Dir(filepath.Clean(f.rel))
		for parent != "." && parent != "" {
			if existing, ok := types[parent]; ok {
				if !existing.dir && existing.link == "" {
					return fmt.Errorf("source path parent %q is not a directory", parent)
				}
			} else {
				types[parent] = fileSnapshot{rel: parent, dir: true}
			}
			next := filepath.Dir(parent)
			if next == parent {
				break
			}
			parent = next
		}
	}
	for _, f := range files {
		if f.link == "" {
			continue
		}
		if err := resolveSnapshotLink(filepath.Clean(f.rel), types, map[string]bool{}); err != nil {
			return fmt.Errorf("source symlink %q: %w", f.rel, err)
		}
	}
	return nil
}

func resolveSnapshotLink(linkPath string, files map[string]fileSnapshot, active map[string]bool) error {
	if active[linkPath] {
		return fmt.Errorf("symlink cycle detected")
	}
	active[linkPath] = true
	defer delete(active, linkPath)
	link, ok := files[linkPath]
	if !ok || link.link == "" {
		return fmt.Errorf("link target %q is not part of the moved source set", linkPath)
	}
	if filepath.IsAbs(link.link) {
		return fmt.Errorf("absolute target is not movable")
	}
	resolved, err := resolveSnapshotComponents(filepath.Dir(linkPath), link.link, files, active)
	if err != nil {
		return err
	}
	if _, exists := files[resolved]; !exists {
		return fmt.Errorf("target %q is not part of the moved source set", resolved)
	}
	return nil
}

//nolint:gocyclo // pathname resolution must model each symlink expansion in order.
func resolveSnapshotComponents(base, target string, files map[string]fileSnapshot, active map[string]bool) (string, error) {
	if filepath.IsAbs(target) {
		return "", fmt.Errorf("absolute target is not movable")
	}
	base = filepath.Clean(base)
	stack := []string{}
	if base != "." && base != "" {
		stack = strings.Split(base, string(filepath.Separator))
	}
	// Do not filepath.Clean target text: cleaning a/b/../x before expanding
	// symlinks changes kernel semantics when b is itself a symlink.
	parts := strings.Split(target, string(filepath.Separator))
	for index, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(stack) == 0 {
				return "", fmt.Errorf("target escapes the moved source set")
			}
			stack = stack[:len(stack)-1]
			continue
		}
		candidate := filepath.Join(append(append([]string{}, stack...), part)...)
		if node, exists := files[candidate]; exists && node.link != "" {
			if active[candidate] {
				return "", fmt.Errorf("symlink cycle detected")
			}
			active[candidate] = true
			resolved, err := resolveSnapshotComponents(filepath.Dir(candidate), node.link, files, active)
			delete(active, candidate)
			if err != nil {
				return "", err
			}
			stack = nil
			if resolved != "." {
				stack = strings.Split(resolved, string(filepath.Separator))
			}
		} else {
			stack = append(stack, part)
		}
		if index < len(parts)-1 {
			resolved := filepath.Join(stack...)
			node, exists := files[resolved]
			if !exists || !node.dir {
				return "", fmt.Errorf("non-directory or missing component %q in target", resolved)
			}
		}
	}
	if len(stack) == 0 {
		return ".", nil
	}
	return filepath.Join(stack...), nil
}

func safeRelative(name string) error {
	clean := filepath.Clean(name)
	if filepath.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("selected source path %q escapes its backup", name)
	}
	if forbiddenName(clean) {
		return fmt.Errorf("selected source path %q is protected", name)
	}
	return nil
}
func forbiddenName(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == ".git" || part == ".tidydots.db" || strings.HasPrefix(part, ".tidydots-txn") {
			return true
		}
	}
	return false
}
func ensureNoSymlinkParents(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	cur := root
	parts := strings.Split(rel, string(filepath.Separator))
	for _, p := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, p)
		fi, e := os.Lstat(cur)
		if e != nil {
			return e
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("source path traverses symlink parent %q", cur)
		}
	}
	return nil
}

func verifyDestinationFree(root, rel string, files []string) error {
	destination := filepath.Join(root, rel)
	if len(files) == 0 {
		if _, err := os.Lstat(destination); err == nil {
			return fmt.Errorf("destination source already exists: %s", destination)
		} else if !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	for _, name := range files {
		p := filepath.Join(destination, name)
		if _, err := os.Lstat(p); err == nil {
			return fmt.Errorf("destination source already exists: %s", p)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

//nolint:gocyclo // stages directories, links and synced regular files distinctly.
func copySnapshot(root string, files []fileSnapshot) error {
	for _, f := range files {
		dest := root
		if f.rel != "." {
			dest = filepath.Join(root, f.rel)
		}
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		destAbs, err := filepath.Abs(dest)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootAbs, destAbs)
		if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("snapshot destination %q escapes stage root %q", dest, root)
		}
		if _, err := os.Lstat(root); err == nil {
			if err := verifyPathParents(root, dest); err != nil {
				return fmt.Errorf("unsafe snapshot destination %q: %w", dest, err)
			}
		} else if os.IsNotExist(err) {
			if err := verifyNoSymlinkPath(filepath.Dir(dest)); err != nil {
				return fmt.Errorf("unsafe snapshot destination parent for %q: %w", dest, err)
			}
		} else {
			return err
		}
		if f.dir {
			if existing, err := os.Lstat(dest); err == nil && existing.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("snapshot destination directory %q is a symlink", dest)
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.MkdirAll(dest, f.mode); err != nil {
				return err
			}
			if f.rel != "." {
				if err := os.Chmod(dest, f.mode); err != nil {
					return err
				}
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
			return err
		}
		if f.link != "" {
			if err := os.Symlink(f.link, dest); err != nil {
				return err
			}
			continue
		}
		file, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, f.mode)
		if err != nil {
			return err
		}
		if _, err := file.Write(f.data); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		if err := os.Chmod(dest, f.mode); err != nil {
			return err
		}
	}
	for _, f := range files {
		if f.rel == "." && f.dir {
			return os.Chmod(root, f.mode)
		}
	}
	return nil
}
