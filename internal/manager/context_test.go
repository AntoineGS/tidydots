package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AntoineGS/dot-manager/internal/config"
	"github.com/AntoineGS/dot-manager/internal/platform"
)

func setupTestManager(t *testing.T) *Manager {
	t.Helper()
	tmpDir := t.TempDir()

	// Create a simple config with some entries
	cfg := &config.Config{
		Version:    2,
		BackupRoot: tmpDir,
		Entries: []config.Entry{
			{Name: "test-entry", Backup: "./test", Targets: map[string]string{"linux": filepath.Join(tmpDir, "target")}},
		},
	}

	plat := &platform.Platform{OS: platform.OSLinux}
	mgr := New(cfg, plat)
	mgr.Verbose = true

	// Create source directory
	srcDir := filepath.Join(tmpDir, "test")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(srcDir, "config.txt"), []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	return mgr
}

func TestRestore_ContextCancellation(t *testing.T) {
	t.Parallel()
	m := setupTestManager(t)

	// Create context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before operation starts

	err := m.RestoreWithContext(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("RestoreWithContext() error = %v, want context.Canceled", err)
	}
}

func TestRestore_ContextTimeout(t *testing.T) {
	t.Parallel()
	m := setupTestManager(t)

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond) // Ensure timeout

	err := m.RestoreWithContext(ctx)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("RestoreWithContext() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestBackup_ContextCancellation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create source directory with files
	homeDir := filepath.Join(tmpDir, "home")
	nvimDir := filepath.Join(homeDir, ".config", "nvim")
	os.MkdirAll(nvimDir, 0755)
	os.WriteFile(filepath.Join(nvimDir, "init.lua"), []byte("vim config"), 0644)

	backupRoot := filepath.Join(tmpDir, "backup")
	os.MkdirAll(filepath.Join(backupRoot, "nvim"), 0755)

	cfg := &config.Config{
		Version:    2,
		BackupRoot: backupRoot,
		Entries: []config.Entry{
			{
				Name:   "nvim",
				Backup: "./nvim",
				Targets: map[string]string{
					"linux": nvimDir,
				},
			},
		},
	}

	plat := &platform.Platform{OS: platform.OSLinux}
	mgr := New(cfg, plat)

	// Create context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before operation starts

	err := mgr.BackupWithContext(ctx)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("BackupWithContext() error = %v, want context.Canceled", err)
	}
}

func TestBackup_ContextTimeout(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	homeDir := filepath.Join(tmpDir, "home")
	nvimDir := filepath.Join(homeDir, ".config", "nvim")
	os.MkdirAll(nvimDir, 0755)
	os.WriteFile(filepath.Join(nvimDir, "init.lua"), []byte("vim config"), 0644)

	backupRoot := filepath.Join(tmpDir, "backup")
	os.MkdirAll(filepath.Join(backupRoot, "nvim"), 0755)

	cfg := &config.Config{
		Version:    2,
		BackupRoot: backupRoot,
		Entries: []config.Entry{
			{
				Name:   "nvim",
				Backup: "./nvim",
				Targets: map[string]string{
					"linux": nvimDir,
				},
			},
		},
	}

	plat := &platform.Platform{OS: platform.OSLinux}
	mgr := New(cfg, plat)

	// Create context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond) // Ensure timeout

	err := mgr.BackupWithContext(ctx)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("BackupWithContext() error = %v, want context.DeadlineExceeded", err)
	}
}
