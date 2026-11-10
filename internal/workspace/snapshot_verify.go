package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

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
	return filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
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
