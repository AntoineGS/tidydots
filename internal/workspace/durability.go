package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// syncDirectoryObserver is an optional test observer; production leaves it nil.
var syncDirectoryObserver func(string)

func syncRegularFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func syncSnapshotTree(root string) error {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			directories = append(directories, path)
			return nil
		}
		if info.Mode().IsRegular() {
			return syncRegularFile(path)
		}
		return fmt.Errorf("cannot durably publish special file %q", path)
	})
	if err != nil {
		return err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		if err := syncDirectory(directories[i]); err != nil {
			return err
		}
	}
	return syncDirectory(filepath.Dir(root))
}

func syncDirectoryParents(paths ...string) error {
	seen := make(map[string]bool)
	for _, path := range paths {
		parent := filepath.Dir(path)
		if seen[parent] {
			continue
		}
		seen[parent] = true
		if err := syncDirectory(parent); err != nil {
			return err
		}
	}
	return nil
}

// ensureDurableDirectory creates path and persists every new directory entry
// from the deepest directory back through a known existing ancestor.
func ensureDurableDirectory(path, boundary string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	boundary, err = filepath.Abs(boundary)
	if err != nil {
		return err
	}
	boundary = filepath.Clean(boundary)
	var missing []string
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, statErr := os.Lstat(current)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("directory path is not a real directory: %s", current)
			}
			if !sameOrChild(boundary, current) && current != boundary {
				return fmt.Errorf("directory path %s is outside durability boundary %s", path, boundary)
			}
			break
		}
		if !os.IsNotExist(statErr) {
			return statErr
		}
		missing = append(missing, current)
		if current == filepath.Dir(current) {
			return fmt.Errorf("no existing ancestor for directory %s", path)
		}
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], 0700); err != nil && !os.IsExist(err) {
			return err
		}
		if err := syncDirectory(filepath.Dir(missing[i])); err != nil {
			return err
		}
	}
	return nil
}

func ensureDurablePrivateDir(path string) error {
	if err := ensureDurableDirectory(path, filepath.VolumeName(path)+string(filepath.Separator)); err != nil {
		return err
	}
	if err := os.Chmod(path, 0700); err != nil { //nolint:gosec // protected transaction state is owner-only.
		return err
	}
	return syncDirectory(path)
}
