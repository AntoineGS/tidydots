package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
)

// --- helpers ---

// minimalTidydotsYAML is a valid v3 tidydots.yaml with no applications.
const minimalTidydotsYAML = `version: 3
applications: []
`

// setupConfigDir creates a temp dir with a minimal tidydots.yaml and points
// the global configDir flag at that dir so loadConfig picks it up.
// The original value is restored after the test.
func setupConfigDir(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "tidydots.yaml")
	if err := os.WriteFile(yamlPath, []byte(minimalTidydotsYAML), 0o600); err != nil {
		t.Fatalf("writing tidydots.yaml: %v", err)
	}

	orig := configDir
	configDir = dir
	t.Cleanup(func() { configDir = orig })
}

// --- loadConfig ---

func TestLoadConfig_ValidConfig(t *testing.T) {
	setupConfigDir(t)

	cfg, plat, cfgFile, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("loadConfig() returned nil config")
	}
	if plat == nil {
		t.Fatal("loadConfig() returned nil platform")
	}
	if cfgFile == "" {
		t.Fatal("loadConfig() returned empty config file path")
	}
}

func TestLoadConfig_MissingConfigDir(t *testing.T) {
	// Remove app config so getConfigDir() fails when configDir flag is empty
	orig := configDir
	configDir = ""
	t.Cleanup(func() { configDir = orig })

	appConfigPath := appConfigPathForTest(t)
	removeAndRestoreFile(t, appConfigPath)

	_, _, _, err := loadConfig()
	if err == nil {
		t.Fatal("loadConfig() expected error when config dir missing, got nil")
	}
}

func TestLoadConfig_InvalidOSOverride(t *testing.T) {
	setupConfigDir(t)

	origOS := osOverride
	osOverride = "invalid-os"
	t.Cleanup(func() { osOverride = origOS })

	_, _, _, err := loadConfig()
	if err == nil {
		t.Fatal("loadConfig() expected error for invalid OS override, got nil")
	}
	if !contains(err.Error(), "invalid OS override") {
		t.Errorf("loadConfig() error = %v, want 'invalid OS override'", err)
	}
}

func TestLoadConfig_ValidOSOverrides(t *testing.T) {
	tests := []struct {
		name string
		os   string
	}{
		{"linux override", "linux"},
		{"windows override", "windows"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupConfigDir(t)

			origOS := osOverride
			osOverride = tt.os
			t.Cleanup(func() { osOverride = origOS })

			cfg, plat, _, err := loadConfig()
			if err != nil {
				t.Fatalf("loadConfig() unexpected error: %v", err)
			}
			if cfg == nil {
				t.Fatal("loadConfig() returned nil config")
			}
			if plat.OS != tt.os {
				t.Errorf("platform.OS = %q, want %q", plat.OS, tt.os)
			}
		})
	}
}

func TestLoadConfig_MissingYAML(t *testing.T) {
	// configDir set to an empty dir (no tidydots.yaml)
	dir := t.TempDir()

	orig := configDir
	configDir = dir
	t.Cleanup(func() { configDir = orig })

	_, _, _, err := loadConfig()
	if err == nil {
		t.Fatal("loadConfig() expected error when tidydots.yaml missing, got nil")
	}
}

// --- runWithCancellation ---

func TestRunWithCancellation_Success(t *testing.T) {
	called := false
	fn := func(_ context.Context) error {
		called = true
		return nil
	}

	if err := runWithCancellation(fn); err != nil {
		t.Errorf("runWithCancellation() unexpected error: %v", err)
	}
	if !called {
		t.Error("runWithCancellation() did not call the function")
	}
}

func TestRunWithCancellation_PropagatesError(t *testing.T) {
	sentinel := errors.New("sentinel error")
	fn := func(_ context.Context) error {
		return sentinel
	}

	err := runWithCancellation(fn)
	if !errors.Is(err, sentinel) {
		t.Errorf("runWithCancellation() error = %v, want %v", err, sentinel)
	}
}

// --- runRestoreWithManager / runBackupWithManager / runListWithManager ---

type mockRestorer struct {
	err error
}

func (m *mockRestorer) Restore() error { return m.err }
func (m *mockRestorer) RestoreWithContext(_ context.Context) error {
	return m.err
}

type mockBackuper struct {
	err error
}

func (m *mockBackuper) Backup() error { return m.err }
func (m *mockBackuper) BackupWithContext(_ context.Context) error {
	return m.err
}

type mockLister struct {
	err error
}

func (m *mockLister) List() error {
	return m.err
}

func TestRunRestoreWithManager_Success(t *testing.T) {
	if err := runRestoreWithManager(&mockRestorer{}); err != nil {
		t.Errorf("runRestoreWithManager() unexpected error: %v", err)
	}
}

func TestRunRestoreWithManager_Error(t *testing.T) {
	sentinel := errors.New("restore error")
	err := runRestoreWithManager(&mockRestorer{err: sentinel})
	if !errors.Is(err, sentinel) {
		t.Errorf("runRestoreWithManager() error = %v, want %v", err, sentinel)
	}
}

func TestRunBackupWithManager_Success(t *testing.T) {
	if err := runBackupWithManager(&mockBackuper{}); err != nil {
		t.Errorf("runBackupWithManager() unexpected error: %v", err)
	}
}

func TestRunBackupWithManager_Error(t *testing.T) {
	sentinel := errors.New("backup error")
	err := runBackupWithManager(&mockBackuper{err: sentinel})
	if !errors.Is(err, sentinel) {
		t.Errorf("runBackupWithManager() error = %v, want %v", err, sentinel)
	}
}

func TestRunListWithManager_Success(t *testing.T) {
	if err := runListWithManager(&mockLister{}); err != nil {
		t.Errorf("runListWithManager() unexpected error: %v", err)
	}
}

func TestRunListWithManager_Error(t *testing.T) {
	sentinel := errors.New("list error")
	err := runListWithManager(&mockLister{err: sentinel})
	if !errors.Is(err, sentinel) {
		t.Errorf("runListWithManager() error = %v, want %v", err, sentinel)
	}
}

// --- helpers ---

// appConfigPathForTest returns the app config path using the exported helper.
func appConfigPathForTest(t *testing.T) string {
	t.Helper()
	return config.AppConfigPath()
}

// removeAndRestoreFile removes a file and restores it (or removes it again) on cleanup.
func removeAndRestoreFile(t *testing.T, path string) {
	t.Helper()

	var originalContent []byte
	var hadFile bool
	if data, err := os.ReadFile(path); err == nil { //nolint:gosec // test file path is controlled
		originalContent = data
		hadFile = true
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removing %s: %v", path, err)
	}

	t.Cleanup(func() {
		if hadFile {
			_ = os.WriteFile(path, originalContent, 0o600)
		} else {
			_ = os.Remove(path)
		}
	})
}
