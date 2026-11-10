package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/state"
	tmpl "github.com/AntoineGS/tidydots/internal/template"
)

// verifyPathParents proves that path is lexically below root and that no
// existing component between root and path's leaf redirects through a symlink.
// The leaf is intentionally left to the caller to inspect with Lstat.
//
//nolint:gocyclo // every existing path component must be checked independently.
func verifyPathParents(root, path string) error {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("resolve repository root %q: %w", root, err)
	}
	canonicalRoot, err = filepath.Abs(canonicalRoot)
	if err != nil {
		return err
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(canonicalRoot, filepath.Clean(absolutePath))
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes repository root %q", path, canonicalRoot)
	}
	if rel == "." {
		return nil
	}
	parent := filepath.Dir(rel)
	if parent == "." {
		return nil
	}
	current := canonicalRoot
	for _, component := range strings.Split(parent, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return fmt.Errorf("inspect path ancestor %q: %w", current, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path traverses symlink ancestor %q", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("path ancestor %q is not a directory", current)
		}
	}
	return nil
}

// verifyProtectedDirectory returns an absolute path suitable for private
// journal/state storage. It is read-only: missing directories are not created.
// Existing symlinks anywhere from the filesystem root to path are forbidden.
//
//nolint:gocyclo // verifies each filesystem component and each protected root.
func verifyProtectedDirectory(path string, mutationRoots ...string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("protected directory path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	abs := filepath.Clean(absolute)
	volume := filepath.VolumeName(abs)
	root := string(filepath.Separator)
	if volume != "" {
		root = volume + string(filepath.Separator)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	current := root
	if rel != "." {
		for _, component := range strings.Split(rel, string(filepath.Separator)) {
			if component == "" || component == "." {
				continue
			}
			current = filepath.Join(current, component)
			info, statErr := os.Lstat(current)
			if os.IsNotExist(statErr) {
				continue
			}
			if statErr != nil {
				return "", fmt.Errorf("inspect protected path ancestor %q: %w", current, statErr)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("protected directory has symlink ancestry at %q", current)
			}
			if current != abs && !info.IsDir() {
				return "", fmt.Errorf("protected path ancestor %q is not a directory", current)
			}
		}
	}
	for _, mutationRoot := range mutationRoots {
		canonical, canonicalErr := filepath.EvalSymlinks(mutationRoot)
		if canonicalErr != nil {
			return "", fmt.Errorf("resolve mutation root %q: %w", mutationRoot, canonicalErr)
		}
		canonical, canonicalErr = filepath.Abs(canonical)
		if canonicalErr != nil {
			return "", canonicalErr
		}
		if pathsOverlap(abs, filepath.Clean(canonical)) {
			return "", fmt.Errorf("protected directory %q overlaps mutation root %q", abs, canonical)
		}
	}
	return abs, nil
}

// checkExpandedSourceOwnership checks the actual snapshot expansion, including
// recognized template artifacts and aliases, against every configured source
// claim. Inactive definitions participate because configuration ownership is
// independent of today's conditions.
//
//nolint:gocyclo // checks complete, inactive ownership definitions and expanded files.
func checkExpandedSourceOwnership(w *Workspace, ref EntryRef, backup string, files []fileSnapshot) error {
	for _, repo := range w.Repositories {
		for _, app := range repo.Config.Applications {
			for _, entry := range app.Entries {
				if repo.Role == ref.Repository && app.Name == ref.App && entry.Name == ref.Name {
					continue
				}
				if !entry.IsConfig() {
					continue
				}
				otherBackup, err := staticBackup(repo, entry.Backup)
				if err != nil {
					return fmt.Errorf("cannot prove expanded source ownership because %s/%s has a dynamic backup path", app.Name, entry.Name)
				}
				claims := []string{}
				if entry.IsFolder() {
					claims = append(claims, otherBackup)
				} else {
					for _, name := range entry.Files {
						if err := safeRelative(name); err != nil {
							return fmt.Errorf("cannot prove source ownership for %s/%s: %w", app.Name, entry.Name, err)
						}
						claims = append(claims, filepath.Join(otherBackup, name))
					}
				}
				for _, file := range files {
					moved := backup
					if file.rel != "." {
						moved = filepath.Join(backup, file.rel)
					}
					for _, claim := range claims {
						if pathsOverlap(moved, claim) {
							return fmt.Errorf("expanded source artifact %q is also owned or referenced by %s/%s (%s)", file.rel, app.Name, entry.Name, repo.Role)
						}
					}
				}
			}
		}
	}
	return nil
}

// expandAttributedTemplateArtifacts adds generated files only when the
// selected template's known symlink alias or captured render history proves
// their provenance. The base snapshot remains the source-ownership set; this
// function never recursively expands a directory or adds structural parents.
//
//nolint:gocyclo // per-method provenance and alias checks are separate safety gates.
func expandAttributedTemplateArtifacts(root string, files []fileSnapshot, backupRel string, method string, records []state.RenderRecord, stateKeys []string) ([]fileSnapshot, error) {
	if method != config.MethodSymlink && method != config.MethodCopy {
		return nil, fmt.Errorf("unsupported template deployment method %q", method)
	}
	backupRel, err := cleanRepositoryRelative(backupRel)
	if err != nil {
		return nil, fmt.Errorf("invalid template source prefix: %w", err)
	}
	remaps := make(map[string]string)
	for _, key := range stateKeys {
		from, to, ok := decodeStateRemap(key)
		if ok {
			remaps[from] = to
		}
	}
	result := append([]fileSnapshot(nil), files...)
	seen := make(map[string]bool, len(files))
	for _, file := range files {
		seen[filepath.Clean(file.rel)] = true
	}
	for _, file := range files {
		if !strings.HasSuffix(file.rel, ".tmpl") || file.link != "" || file.dir {
			continue
		}
		templateRel := filepath.Clean(file.rel)
		sourceKey := "./" + filepath.ToSlash(filepath.Join(backupRel, templateRel))
		historyOwned := renderHistoryOwnsSource(sourceKey, records, remaps)
		aliasOwned := method == config.MethodSymlink && hasCapturedTemplateAlias(root, templateRel, files)
		suffixes := []string{templateRenderedSuffix, templateConflictSuffix, templateRenderedSuffix + ".bak"}
		if !historyOwned && !aliasOwned {
			if err := rejectUnattributedGeneratedArtifacts(root, templateRel, suffixes, seen); err != nil {
				return nil, err
			}
			continue
		}
		for _, suffix := range suffixes {
			rel := templateRel + suffix
			if seen[rel] {
				continue
			}
			artifact, exists, err := captureGeneratedArtifact(root, rel)
			if err != nil {
				return nil, err
			}
			if exists {
				result = append(result, artifact)
				seen[rel] = true
			}
		}
	}
	if err := validateMovedSymlinks(result); err != nil {
		return nil, err
	}
	return result, nil
}

func rejectUnattributedGeneratedArtifacts(root, templateRel string, suffixes []string, selected map[string]bool) error {
	for _, suffix := range suffixes {
		rel := filepath.Clean(templateRel + suffix)
		if selected[rel] {
			continue
		}
		path := filepath.Join(root, rel)
		if err := verifyPathParents(root, path); err != nil {
			return err
		}
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("cannot prove ownership of existing template artifact %q; refusing to leave potentially private recovery data behind", rel)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func renderHistoryOwnsSource(sourceKey string, records []state.RenderRecord, remaps map[string]string) bool {
	for _, record := range records {
		candidates := []string{record.TemplatePath}
		if remapped := remaps[record.TemplatePath]; remapped != "" {
			candidates = append(candidates, remapped)
		}
		for _, candidate := range candidates {
			if candidate == sourceKey || remaps[candidate] == sourceKey {
				return true
			}
			if copySource, _, isCopy := parseCopyRenderKey(candidate); isCopy && (copySource == sourceKey || remaps[copySource] == sourceKey) {
				return true
			}
			if copySource, _, isCopy := parseCopyRenderKey(candidate); isCopy {
				if remapped := remaps[copySource]; remapped != "" {
					if remappedSource, _, remappedCopy := parseCopyRenderKey(remapped); remappedCopy && remappedSource == sourceKey {
						return true
					}
				}
			}
		}
	}
	return false
}

func hasCapturedTemplateAlias(root, templateRel string, files []fileSnapshot) bool {
	aliasRel := filepath.Join(filepath.Dir(templateRel), tmpl.TargetName(filepath.Base(templateRel)))
	for _, file := range files {
		if filepath.Clean(file.rel) != filepath.Clean(aliasRel) || file.link == "" || filepath.IsAbs(file.link) {
			continue
		}
		resolved := filepath.Clean(filepath.Join(filepath.Dir(aliasRel), file.link))
		return resolved == filepath.Clean(templateRel+templateRenderedSuffix)
	}
	path := filepath.Join(root, aliasRel)
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	link, err := os.Readlink(path)
	if err != nil || filepath.IsAbs(link) {
		return false
	}
	return filepath.Clean(filepath.Join(filepath.Dir(aliasRel), link)) == filepath.Clean(templateRel+templateRenderedSuffix)
}

func captureGeneratedArtifact(root, rel string) (fileSnapshot, bool, error) {
	if err := safeRelative(rel); err != nil {
		return fileSnapshot{}, false, err
	}
	path := filepath.Join(root, rel)
	if err := verifyPathParents(root, path); err != nil {
		return fileSnapshot{}, false, err
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return fileSnapshot{}, false, nil
	}
	if err != nil {
		return fileSnapshot{}, false, err
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, false, fmt.Errorf("attributed generated artifact %q is not a regular file", rel)
	}
	if linkedRegularFile(info) {
		return fileSnapshot{}, false, fmt.Errorf("attributed generated artifact %q is multiply-linked", rel)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{}, false, err
	}
	return fileSnapshot{rel: filepath.Clean(rel), mode: info.Mode().Perm(), data: data}, true, nil
}
