package fsys_test

import (
	"errors"
	"io/fs"
	"os"
	"testing"

	"github.com/AntoineGS/tidydots/internal/fsys"
)

// newFS is a helper that returns a MemFS with a pre-created base directory.
func newFS(t *testing.T) *fsys.MemFS {
	t.Helper()
	m := fsys.NewMemFS()
	if err := m.MkdirAll("/base", 0o755); err != nil {
		t.Fatalf("MkdirAll /base: %v", err)
	}
	return m
}

func TestMemFS_WriteAndReadFile(t *testing.T) {
	m := newFS(t)

	data := []byte("hello, world")
	if err := m.WriteFile("/base/file.txt", data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := m.ReadFile("/base/file.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("ReadFile = %q, want %q", got, data)
	}
}

func TestMemFS_ReadFile_Nonexistent(t *testing.T) {
	m := newFS(t)

	_, err := m.ReadFile("/base/missing.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ReadFile nonexistent: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Stat_CorrectSizeAndMode(t *testing.T) {
	m := newFS(t)

	data := []byte("content")
	if err := m.WriteFile("/base/stat.txt", data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	info, err := m.Stat("/base/stat.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() != int64(len(data)) {
		t.Errorf("Size = %d, want %d", info.Size(), len(data))
	}
	if info.Mode() != fs.FileMode(0o600) {
		t.Errorf("Mode = %v, want 0600", info.Mode())
	}
	if info.IsDir() {
		t.Error("IsDir should be false for regular file")
	}
}

func TestMemFS_Stat_Nonexistent(t *testing.T) {
	m := newFS(t)

	_, err := m.Stat("/base/nope.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat nonexistent: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Lstat_OnSymlink(t *testing.T) {
	m := newFS(t)

	// Write a target file.
	if err := m.WriteFile("/base/target.txt", []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Create a symlink.
	if err := m.Symlink("/base/target.txt", "/base/link.txt"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	info, err := m.Lstat("/base/link.txt")
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("Lstat mode %v should include ModeSymlink", info.Mode())
	}
}

func TestMemFS_Symlink_Readlink_Roundtrip(t *testing.T) {
	m := newFS(t)

	if err := m.WriteFile("/base/original.txt", []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.Symlink("/base/original.txt", "/base/alias.txt"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	target, err := m.Readlink("/base/alias.txt")
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != "/base/original.txt" {
		t.Errorf("Readlink = %q, want %q", target, "/base/original.txt")
	}
}

func TestMemFS_Readlink_Nonexistent(t *testing.T) {
	m := newFS(t)

	_, err := m.Readlink("/base/nosuchlink")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Readlink nonexistent: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Stat_FollowsSymlink(t *testing.T) {
	m := newFS(t)

	data := []byte("followed")
	if err := m.WriteFile("/base/real.txt", data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.Symlink("/base/real.txt", "/base/sym.txt"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	info, err := m.Stat("/base/sym.txt")
	if err != nil {
		t.Fatalf("Stat through symlink: %v", err)
	}
	if info.Size() != int64(len(data)) {
		t.Errorf("Stat through symlink: Size = %d, want %d", info.Size(), len(data))
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		t.Error("Stat should follow symlink and not return ModeSymlink")
	}
}

func TestMemFS_MkdirAll_And_ReadDir(t *testing.T) {
	m := fsys.NewMemFS()

	if err := m.MkdirAll("/a/b/c", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := m.WriteFile("/a/b/file.txt", []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entries, err := m.ReadDir("/a/b")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}

	if len(entries) != 2 {
		t.Fatalf("ReadDir /a/b: got %v, want [c, file.txt]", names)
	}
	if names[0] != "c" || names[1] != "file.txt" {
		t.Errorf("ReadDir /a/b entries = %v, want [c file.txt]", names)
	}
	if !entries[0].IsDir() {
		t.Error("entry 'c' should be a directory")
	}
	if entries[1].IsDir() {
		t.Error("entry 'file.txt' should not be a directory")
	}
}

func TestMemFS_ReadDir_Nonexistent(t *testing.T) {
	m := fsys.NewMemFS()

	_, err := m.ReadDir("/nonexistent")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ReadDir nonexistent: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Remove_File(t *testing.T) {
	m := newFS(t)

	if err := m.WriteFile("/base/del.txt", []byte("bye"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.Remove("/base/del.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err := m.ReadFile("/base/del.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("after Remove, ReadFile got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Remove_Nonexistent(t *testing.T) {
	m := newFS(t)

	err := m.Remove("/base/ghost.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Remove nonexistent: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_RemoveAll(t *testing.T) {
	m := fsys.NewMemFS()

	if err := m.MkdirAll("/tree/a/b", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := m.WriteFile("/tree/a/b/file.txt", []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.WriteFile("/tree/a/other.txt", []byte("other"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := m.RemoveAll("/tree/a"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	// All children gone.
	_, err := m.ReadFile("/tree/a/b/file.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("after RemoveAll, file still exists: %v", err)
	}
	// Parent directory still exists.
	if _, err := m.Stat("/tree"); err != nil {
		t.Errorf("parent /tree should still exist after RemoveAll /tree/a: %v", err)
	}
}

func TestMemFS_WalkDir_VisitsFilesInOrder(t *testing.T) {
	m := fsys.NewMemFS()

	if err := m.MkdirAll("/walk/sub", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := m.WriteFile("/walk/a.txt", []byte("a"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}
	if err := m.WriteFile("/walk/b.txt", []byte("b"), 0o644); err != nil {
		t.Fatalf("WriteFile b.txt: %v", err)
	}
	if err := m.WriteFile("/walk/sub/c.txt", []byte("c"), 0o644); err != nil {
		t.Fatalf("WriteFile sub/c.txt: %v", err)
	}

	var visited []string
	err := m.WalkDir("/walk", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		visited = append(visited, path)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}

	// Paths should be sorted: /walk, /walk/a.txt, /walk/b.txt, /walk/sub, /walk/sub/c.txt
	expected := []string{"/walk", "/walk/a.txt", "/walk/b.txt", "/walk/sub", "/walk/sub/c.txt"}
	if len(visited) != len(expected) {
		t.Fatalf("WalkDir visited %v, want %v", visited, expected)
	}
	for i, p := range expected {
		if visited[i] != p {
			t.Errorf("WalkDir[%d] = %q, want %q", i, visited[i], p)
		}
	}
}

func TestMemFS_Rename_File(t *testing.T) {
	m := newFS(t)

	if err := m.WriteFile("/base/old.txt", []byte("content"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.Rename("/base/old.txt", "/base/new.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	data, err := m.ReadFile("/base/new.txt")
	if err != nil {
		t.Fatalf("ReadFile after rename: %v", err)
	}
	if string(data) != "content" {
		t.Errorf("renamed file content = %q, want %q", data, "content")
	}

	_, err = m.ReadFile("/base/old.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old path after rename: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Symlink_DuplicateFails(t *testing.T) {
	m := newFS(t)

	if err := m.WriteFile("/base/file.txt", []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.Symlink("/base/file.txt", "/base/link"); err != nil {
		t.Fatalf("first Symlink: %v", err)
	}
	if err := m.Symlink("/base/file.txt", "/base/link"); !errors.Is(err, os.ErrExist) {
		t.Errorf("duplicate Symlink: got %v, want os.ErrExist", err)
	}
}

func TestMemFS_OsFS_ImplementsInterface(t *testing.T) {
	// Compile-time check that both OsFS and MemFS satisfy the FS interface.
	var _ fsys.FS = fsys.OsFS{}
	var _ fsys.FS = &fsys.MemFS{}
}

func TestMemFS_Lstat_File(t *testing.T) {
	m := newFS(t)

	if err := m.WriteFile("/base/plain.txt", []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	info, err := m.Lstat("/base/plain.txt")
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.IsDir() {
		t.Error("Lstat on file: IsDir should be false")
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		t.Error("Lstat on file: should not report ModeSymlink")
	}
	if info.Mode() != fs.FileMode(0o600) {
		t.Errorf("Lstat mode = %v, want 0600", info.Mode())
	}
}

func TestMemFS_Lstat_Directory(t *testing.T) {
	m := fsys.NewMemFS()

	if err := m.MkdirAll("/mydir", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	info, err := m.Lstat("/mydir")
	if err != nil {
		t.Fatalf("Lstat dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("Lstat on dir: IsDir should be true")
	}
}

func TestMemFS_Lstat_Nonexistent(t *testing.T) {
	m := newFS(t)

	_, err := m.Lstat("/base/nobody")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Lstat nonexistent: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Remove_Symlink(t *testing.T) {
	m := newFS(t)

	if err := m.WriteFile("/base/tgt.txt", []byte("y"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.Symlink("/base/tgt.txt", "/base/lnk"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if err := m.Remove("/base/lnk"); err != nil {
		t.Fatalf("Remove symlink: %v", err)
	}

	_, err := m.Readlink("/base/lnk")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("after Remove symlink, Readlink got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Remove_EmptyDirectory(t *testing.T) {
	m := fsys.NewMemFS()

	if err := m.MkdirAll("/emptydir", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := m.Remove("/emptydir"); err != nil {
		t.Fatalf("Remove empty dir: %v", err)
	}

	_, err := m.Stat("/emptydir")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("after Remove dir, Stat got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Remove_NonEmptyDirectoryFails(t *testing.T) {
	m := fsys.NewMemFS()

	if err := m.MkdirAll("/nonempty", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := m.WriteFile("/nonempty/file.txt", []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	err := m.Remove("/nonempty")
	if err == nil {
		t.Error("Remove non-empty dir: expected error, got nil")
	}
}

func TestMemFS_Rename_Symlink(t *testing.T) {
	m := newFS(t)

	if err := m.WriteFile("/base/src.txt", []byte("z"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.Symlink("/base/src.txt", "/base/oldlink"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if err := m.Rename("/base/oldlink", "/base/newlink"); err != nil {
		t.Fatalf("Rename symlink: %v", err)
	}

	target, err := m.Readlink("/base/newlink")
	if err != nil {
		t.Fatalf("Readlink after rename: %v", err)
	}
	if target != "/base/src.txt" {
		t.Errorf("Readlink = %q, want %q", target, "/base/src.txt")
	}
	_, err = m.Readlink("/base/oldlink")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old symlink after rename: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Rename_Directory(t *testing.T) {
	m := fsys.NewMemFS()

	if err := m.MkdirAll("/olddir/sub", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := m.WriteFile("/olddir/file.txt", []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := m.Rename("/olddir", "/newdir"); err != nil {
		t.Fatalf("Rename dir: %v", err)
	}

	data, err := m.ReadFile("/newdir/file.txt")
	if err != nil {
		t.Fatalf("ReadFile after dir rename: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("file content after dir rename = %q, want %q", data, "hello")
	}

	_, err = m.Stat("/olddir")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old dir after rename: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Rename_Nonexistent(t *testing.T) {
	m := newFS(t)

	err := m.Rename("/base/ghost.txt", "/base/other.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Rename nonexistent: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_ReadDir_IncludesSymlinks(t *testing.T) {
	m := newFS(t)

	if err := m.WriteFile("/base/real.txt", []byte("r"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := m.Symlink("/base/real.txt", "/base/link.txt"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	entries, err := m.ReadDir("/base")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}

	found := false
	for _, e := range entries {
		if e.Name() == "link.txt" {
			found = true
			if e.Type()&fs.ModeSymlink == 0 {
				t.Errorf("link.txt entry type = %v, want ModeSymlink set", e.Type())
			}
		}
	}
	if !found {
		t.Errorf("ReadDir entries %v: missing link.txt", names)
	}
}

func TestMemFS_WriteFile_MissingParent(t *testing.T) {
	m := fsys.NewMemFS()

	err := m.WriteFile("/nonexistent/file.txt", []byte("x"), 0o644)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("WriteFile missing parent: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_Symlink_MissingParent(t *testing.T) {
	m := fsys.NewMemFS()

	err := m.Symlink("/target", "/nonexistent/link")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Symlink missing parent: got %v, want os.ErrNotExist", err)
	}
}

func TestMemFS_DirEntry_Info(t *testing.T) {
	m := newFS(t)

	if err := m.WriteFile("/base/info.txt", []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entries, err := m.ReadDir("/base")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("ReadDir: no entries returned")
	}

	info, err := entries[0].Info()
	if err != nil {
		t.Fatalf("DirEntry.Info: %v", err)
	}
	if info.Name() != "info.txt" {
		t.Errorf("DirEntry.Info Name = %q, want %q", info.Name(), "info.txt")
	}
}

func TestMemFS_FileInfo_Methods(t *testing.T) {
	m := newFS(t)

	if err := m.WriteFile("/base/meta.txt", []byte("12345"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	info, err := m.Stat("/base/meta.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	if info.Name() != "meta.txt" {
		t.Errorf("Name = %q, want %q", info.Name(), "meta.txt")
	}
	if !info.ModTime().IsZero() {
		t.Errorf("ModTime should be zero, got %v", info.ModTime())
	}
	if info.Sys() != nil {
		t.Errorf("Sys should be nil, got %v", info.Sys())
	}
}

func TestMemFS_WalkDir_StopOnError(t *testing.T) {
	m := fsys.NewMemFS()

	if err := m.MkdirAll("/stop", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := m.WriteFile("/stop/a.txt", []byte("a"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	walkErr := errors.New("stop walking")
	var count int
	err := m.WalkDir("/stop", func(_ string, _ fs.DirEntry, _ error) error {
		count++
		return walkErr
	})
	if !errors.Is(err, walkErr) {
		t.Errorf("WalkDir stop: got %v, want %v", err, walkErr)
	}
	if count != 1 {
		t.Errorf("WalkDir stop: visited %d entries, want 1", count)
	}
}
