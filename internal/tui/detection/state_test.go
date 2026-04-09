package detection

import (
	"os"
	"path/filepath"
	"testing"

	tuitable "github.com/AntoineGS/tidydots/internal/tui/table"
)

// mkSymlink creates a symlink at dst pointing to src.
func mkSymlink(t *testing.T, src, dst string) {
	t.Helper()
	if err := os.Symlink(src, dst); err != nil {
		t.Fatalf("os.Symlink(%q, %q): %v", src, dst, err)
	}
}

// mkDir creates a directory (including parents).
func mkDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%q): %v", path, err)
	}
}

// mkFile creates a file with empty content.
func mkFile(t *testing.T, path string) {
	t.Helper()
	mkDir(t, filepath.Dir(path))
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create(%q): %v", path, err)
	}
	f.Close()
}

// ── Folder-based tests ──────────────────────────────────────────────────────

func TestDetectConfigState_Folder_Linked(t *testing.T) {
	// targetPath is a symlink → StateLinked
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup")
	targetPath := filepath.Join(tmp, "target_link")

	mkDir(t, backupPath)
	mkSymlink(t, backupPath, targetPath)

	got := DetectConfigState(backupPath, targetPath, true, nil)
	if got != tuitable.StateLinked {
		t.Errorf("folder symlink → want StateLinked, got %v", got)
	}
}

func TestDetectConfigState_Folder_Ready(t *testing.T) {
	// backup exists, target is NOT a symlink → StateReady
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup")
	targetPath := filepath.Join(tmp, "target")

	mkDir(t, backupPath)
	// target does not exist — backup exists → Ready
	got := DetectConfigState(backupPath, targetPath, true, nil)
	if got != tuitable.StateReady {
		t.Errorf("folder backup-only → want StateReady, got %v", got)
	}
}

func TestDetectConfigState_Folder_ReadyWithExistingTarget(t *testing.T) {
	// backup exists AND target exists (real dir, not symlink) → StateReady (backup wins)
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup")
	targetPath := filepath.Join(tmp, "target")

	mkDir(t, backupPath)
	mkDir(t, targetPath)

	got := DetectConfigState(backupPath, targetPath, true, nil)
	if got != tuitable.StateReady {
		t.Errorf("folder backup+target → want StateReady, got %v", got)
	}
}

func TestDetectConfigState_Folder_Adopt(t *testing.T) {
	// no backup, target exists as real directory → StateAdopt
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup_missing")
	targetPath := filepath.Join(tmp, "target")

	mkDir(t, targetPath)

	got := DetectConfigState(backupPath, targetPath, true, nil)
	if got != tuitable.StateAdopt {
		t.Errorf("folder target-only → want StateAdopt, got %v", got)
	}
}

func TestDetectConfigState_Folder_Missing(t *testing.T) {
	// neither backup nor target exists → StateMissing
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup_missing")
	targetPath := filepath.Join(tmp, "target_missing")

	got := DetectConfigState(backupPath, targetPath, true, nil)
	if got != tuitable.StateMissing {
		t.Errorf("folder nothing → want StateMissing, got %v", got)
	}
}

// ── File-based tests ────────────────────────────────────────────────────────

func TestDetectConfigState_Files_Linked(t *testing.T) {
	// All files in backupPath are symlinked from targetPath → StateLinked
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup")
	targetPath := filepath.Join(tmp, "target")

	mkDir(t, backupPath)
	mkDir(t, targetPath)

	files := []string{".bashrc", ".zshrc"}
	for _, f := range files {
		src := filepath.Join(backupPath, f)
		dst := filepath.Join(targetPath, f)
		mkFile(t, src)
		mkSymlink(t, src, dst)
	}

	got := DetectConfigState(backupPath, targetPath, false, files)
	if got != tuitable.StateLinked {
		t.Errorf("files all-symlinked → want StateLinked, got %v", got)
	}
}

func TestDetectConfigState_Files_Ready(t *testing.T) {
	// backup files exist, target files do not exist → StateReady
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup")
	targetPath := filepath.Join(tmp, "target")

	mkDir(t, backupPath)
	mkDir(t, targetPath)

	files := []string{".bashrc"}
	mkFile(t, filepath.Join(backupPath, files[0]))
	// targetPath/.bashrc intentionally absent

	got := DetectConfigState(backupPath, targetPath, false, files)
	if got != tuitable.StateReady {
		t.Errorf("files backup-only → want StateReady, got %v", got)
	}
}

func TestDetectConfigState_Files_NoBackupTargetOnly(t *testing.T) {
	// File-based: backup files do NOT exist, only target files exist.
	// The loop skips files with no backup, so anyBackup=false and anyTarget=false.
	// Result is StateMissing (StateAdopt is unreachable for file-based without a backup).
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup")
	targetPath := filepath.Join(tmp, "target")

	mkDir(t, backupPath)
	mkDir(t, targetPath)

	files := []string{".bashrc"}
	// backup file is absent; only target file exists
	mkFile(t, filepath.Join(targetPath, files[0]))

	got := DetectConfigState(backupPath, targetPath, false, files)
	if got != tuitable.StateMissing {
		t.Errorf("files target-only (no backup) → want StateMissing, got %v", got)
	}
}

func TestDetectConfigState_Files_Missing(t *testing.T) {
	// neither backup nor target files exist → StateMissing
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup")
	targetPath := filepath.Join(tmp, "target")

	mkDir(t, backupPath)
	mkDir(t, targetPath)

	files := []string{".bashrc"}
	// no files created at all

	got := DetectConfigState(backupPath, targetPath, false, files)
	if got != tuitable.StateMissing {
		t.Errorf("files nothing → want StateMissing, got %v", got)
	}
}

func TestDetectConfigState_Files_EmptyFileList(t *testing.T) {
	// empty files list with isFolder=false → StateMissing (no files checked)
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup")
	targetPath := filepath.Join(tmp, "target")

	mkDir(t, backupPath)

	got := DetectConfigState(backupPath, targetPath, false, []string{})
	if got != tuitable.StateMissing {
		t.Errorf("files empty list → want StateMissing, got %v", got)
	}
}

func TestDetectConfigState_Files_PartialSymlinks(t *testing.T) {
	// Some files symlinked, some only in backup → StateReady (not all linked)
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup")
	targetPath := filepath.Join(tmp, "target")

	mkDir(t, backupPath)
	mkDir(t, targetPath)

	files := []string{".bashrc", ".zshrc"}

	// .bashrc: backup only
	mkFile(t, filepath.Join(backupPath, files[0]))

	// .zshrc: backup + symlinked
	src := filepath.Join(backupPath, files[1])
	dst := filepath.Join(targetPath, files[1])
	mkFile(t, src)
	mkSymlink(t, src, dst)

	got := DetectConfigState(backupPath, targetPath, false, files)
	if got != tuitable.StateReady {
		t.Errorf("files partial-symlinks → want StateReady, got %v", got)
	}
}

func TestDetectConfigState_Files_NonSymlinkTarget(t *testing.T) {
	// backup file exists, target file exists but is a real file (not a symlink) → StateReady
	// (because backup exists and takes priority over anyTarget check)
	tmp := t.TempDir()
	backupPath := filepath.Join(tmp, "backup")
	targetPath := filepath.Join(tmp, "target")

	mkDir(t, backupPath)
	mkDir(t, targetPath)

	files := []string{".vimrc"}
	mkFile(t, filepath.Join(backupPath, files[0]))
	mkFile(t, filepath.Join(targetPath, files[0])) // real file, not symlink

	got := DetectConfigState(backupPath, targetPath, false, files)
	if got != tuitable.StateReady {
		t.Errorf("files real-file-target with backup → want StateReady, got %v", got)
	}
}
