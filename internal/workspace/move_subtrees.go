package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func selectedDirectoryRoots(selected []string, files []fileSnapshot) []string {
	dirs := make(map[string]bool, len(files))
	for _, file := range files {
		if file.dir {
			dirs[filepath.Clean(file.rel)] = true
		}
	}
	var roots []string
	for _, path := range selected {
		rel := filepath.Clean(filepath.FromSlash(path))
		if dirs[rel] {
			roots = append(roots, rel)
		}
	}
	return roots
}

func verifyMoveSourceSnapshot(root string, files []fileSnapshot, wholeFolder bool, selectedDirs []string) error {
	verify := verifySnapshotAt
	if wholeFolder {
		verify = verifyExactTree
	}
	if err := verify(root, files); err != nil {
		return err
	}
	if !wholeFolder {
		return verifySelectedSubtrees(root, files, selectedDirs)
	}
	return nil
}

// verifySelectedSubtrees enforces exact membership only below explicitly
// selected directories. Unselected siblings of the backup root remain private
// to their other owners and are intentionally outside this check.
func verifySelectedSubtrees(root string, files []fileSnapshot, roots []string) error {
	for _, selected := range roots {
		selectedRoot := filepath.Join(root, selected)
		expected := make(map[string]fileSnapshot)
		for _, file := range files {
			rel := filepath.Clean(file.rel)
			if rel == selected || sameOrChild(selected, rel) {
				expected[rel] = file
			}
		}
		if err := filepath.WalkDir(selectedRoot, func(path string, _ os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			want, ok := expected[filepath.Clean(rel)]
			if !ok {
				return fmt.Errorf("unexpected selected-directory member appeared: %s", rel)
			}
			return verifyOneSnapshot(path, want)
		}); err != nil {
			return err
		}
		if len(expected) == 0 {
			return fmt.Errorf("selected directory has no captured members: %s", selected)
		}
		for rel := range expected {
			path := filepath.Join(root, rel)
			if _, err := os.Lstat(path); err != nil {
				return fmt.Errorf("selected-directory member is missing: %s: %w", rel, err)
			}
		}
	}
	return nil
}

func verifyRemainingSelectedSubtrees(backup string, j *moveJournal, pending []fileSnapshot) error {
	cleaned := make(map[string]bool, len(j.CleanedPaths))
	for _, rel := range j.CleanedPaths {
		cleaned[filepath.Clean(rel)] = true
	}
	intent := ""
	if strings.HasPrefix(j.Intent, "cleanup-source:") {
		intent = filepath.Clean(strings.TrimPrefix(j.Intent, "cleanup-source:"))
	}
	for _, selected := range j.SelectedDirs {
		if err := verifyOneRemainingSelectedSubtree(backup, selected, pending, cleaned, intent); err != nil {
			return err
		}
	}
	return nil
}

func verifyOneRemainingSelectedSubtree(backup, selected string, pending []fileSnapshot, cleaned map[string]bool, intent string) error {
	if cleaned[selected] || intent == selected {
		return nil
	}
	expected := make(map[string]fileSnapshot)
	for _, file := range pending {
		rel := filepath.Clean(file.rel)
		if rel == selected || sameOrChild(selected, rel) {
			expected[rel] = file
		}
	}
	selectedRoot := filepath.Join(backup, selected)
	if len(expected) == 0 {
		if _, err := os.Lstat(selectedRoot); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
		return fmt.Errorf("selected directory remains after all recorded members were removed: %s", selected)
	}
	for rel, file := range expected {
		if err := verifyOneSnapshot(filepath.Join(backup, rel), file); err != nil {
			return fmt.Errorf("remaining selected-directory member changed: %w", err)
		}
	}
	return filepath.WalkDir(selectedRoot, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(backup, path)
		if err != nil {
			return err
		}
		want, ok := expected[filepath.Clean(rel)]
		if !ok {
			return fmt.Errorf("unexpected source path appeared during selected-directory cleanup: %s", rel)
		}
		return verifyOneSnapshot(path, want)
	})
}
