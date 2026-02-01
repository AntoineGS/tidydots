package manager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AntoineGS/dot-manager/internal/config"
	"github.com/AntoineGS/dot-manager/internal/platform"
)

func TestRestoreFolder(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source directory with content
	srcDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "config.txt"), []byte("config"), 0644)

	// Target directory
	targetDir := filepath.Join(tmpDir, "target", "config")

	cfg := &config.Config{BackupRoot: tmpDir}
	plat := &platform.Platform{OS: platform.OSLinux}
	mgr := New(cfg, plat)
	mgr.Verbose = true

	err := mgr.restoreFolder("test", srcDir, targetDir)
	if err != nil {
		t.Fatalf("restoreFolder() error = %v", err)
	}

	// Check symlink was created
	if !isSymlink(targetDir) {
		t.Error("Target is not a symlink")
	}

	// Check symlink points to source
	link, err := os.Readlink(targetDir)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}

	if link != srcDir {
		t.Errorf("Symlink target = %q, want %q", link, srcDir)
	}
}

func TestRestoreFolderSkipsExistingSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	srcDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(srcDir, 0755)

	targetDir := filepath.Join(tmpDir, "target")
	os.Symlink(srcDir, targetDir)

	cfg := &config.Config{BackupRoot: tmpDir}
	plat := &platform.Platform{OS: platform.OSLinux}
	mgr := New(cfg, plat)
	mgr.Verbose = true

	err := mgr.restoreFolder("test", srcDir, targetDir)
	if err != nil {
		t.Fatalf("restoreFolder() error = %v", err)
	}

	// Symlink should still exist and point to same target
	link, _ := os.Readlink(targetDir)
	if link != srcDir {
		t.Errorf("Symlink target changed to %q, want %q", link, srcDir)
	}
}

func TestRestoreFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source files
	srcDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(srcDir, "file2.txt"), []byte("content2"), 0644)

	targetDir := filepath.Join(tmpDir, "target")

	cfg := &config.Config{BackupRoot: tmpDir}
	plat := &platform.Platform{OS: platform.OSLinux}
	mgr := New(cfg, plat)

	files := []string{"file1.txt", "file2.txt"}
	err := mgr.restoreFiles("test", files, srcDir, targetDir)
	if err != nil {
		t.Fatalf("restoreFiles() error = %v", err)
	}

	// Check symlinks were created
	for _, file := range files {
		targetFile := filepath.Join(targetDir, file)
		if !isSymlink(targetFile) {
			t.Errorf("%s is not a symlink", file)
		}

		link, _ := os.Readlink(targetFile)
		expectedLink := filepath.Join(srcDir, file)
		if link != expectedLink {
			t.Errorf("Symlink for %s = %q, want %q", file, link, expectedLink)
		}
	}
}

func TestRestoreFilesRemovesExisting(t *testing.T) {
	tmpDir := t.TempDir()

	srcDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "config.txt"), []byte("new content"), 0644)

	targetDir := filepath.Join(tmpDir, "target")
	os.MkdirAll(targetDir, 0755)
	os.WriteFile(filepath.Join(targetDir, "config.txt"), []byte("old content"), 0644)

	cfg := &config.Config{BackupRoot: tmpDir}
	plat := &platform.Platform{OS: platform.OSLinux}
	mgr := New(cfg, plat)

	err := mgr.restoreFiles("test", []string{"config.txt"}, srcDir, targetDir)
	if err != nil {
		t.Fatalf("restoreFiles() error = %v", err)
	}

	targetFile := filepath.Join(targetDir, "config.txt")
	if !isSymlink(targetFile) {
		t.Error("Target file is not a symlink after restore")
	}
}

func TestRestoreDryRun(t *testing.T) {
	tmpDir := t.TempDir()

	srcDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "config.txt"), []byte("content"), 0644)

	targetDir := filepath.Join(tmpDir, "target")

	cfg := &config.Config{BackupRoot: tmpDir}
	plat := &platform.Platform{OS: platform.OSLinux}
	mgr := New(cfg, plat)
	mgr.DryRun = true

	err := mgr.restoreFolder("test", srcDir, targetDir)
	if err != nil {
		t.Fatalf("restoreFolder() error = %v", err)
	}

	// Target should NOT be created in dry run mode
	if pathExists(targetDir) {
		t.Error("Target was created despite dry run mode")
	}
}

func TestRestoreIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// Create backup structure
	backupRoot := filepath.Join(tmpDir, "backup")
	nvimBackup := filepath.Join(backupRoot, "nvim")
	os.MkdirAll(nvimBackup, 0755)
	os.WriteFile(filepath.Join(nvimBackup, "init.lua"), []byte("vim config"), 0644)

	bashBackup := filepath.Join(backupRoot, "bash")
	os.MkdirAll(bashBackup, 0755)
	os.WriteFile(filepath.Join(bashBackup, ".bashrc"), []byte("bash config"), 0644)

	// Create config
	cfg := &config.Config{
		BackupRoot: backupRoot,
		Paths: []config.PathSpec{
			{
				Name:   "nvim",
				Files:  []string{},
				Backup: "./nvim",
				Targets: map[string]string{
					"linux": filepath.Join(tmpDir, "home", ".config", "nvim"),
				},
			},
			{
				Name:   "bash",
				Files:  []string{".bashrc"},
				Backup: "./bash",
				Targets: map[string]string{
					"linux": filepath.Join(tmpDir, "home"),
				},
			},
		},
	}

	plat := &platform.Platform{OS: platform.OSLinux}
	mgr := New(cfg, plat)

	err := mgr.Restore()
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Check nvim folder symlink
	nvimTarget := filepath.Join(tmpDir, "home", ".config", "nvim")
	if !isSymlink(nvimTarget) {
		t.Error("nvim target is not a symlink")
	}

	// Check bashrc file symlink
	bashrcTarget := filepath.Join(tmpDir, "home", ".bashrc")
	if !isSymlink(bashrcTarget) {
		t.Error(".bashrc target is not a symlink")
	}
}
