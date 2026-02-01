package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
version: 1
backup_root: "~/gits/configurations"

paths:
  - name: "neovim"
    files: []
    backup: "./Both/Neovim/nvim"
    targets:
      linux: "~/.config/nvim"
      windows: "~/AppData/Local/nvim"

  - name: "bash"
    files: [".bashrc", ".bash_profile"]
    backup: "./Linux/Bash"
    targets:
      linux: "~"

root_paths:
  - name: "pacman-hooks"
    files: ["pkg-backup.hook"]
    backup: "./Linux/pacman"
    targets:
      linux: "/etc/pacman.d/hooks"

hooks:
  post_restore:
    linux:
      - type: "test-hook"
        skip_on_arch: true
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Test version
	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}

	// Test backup root
	if cfg.BackupRoot != "~/gits/configurations" {
		t.Errorf("BackupRoot = %q, want %q", cfg.BackupRoot, "~/gits/configurations")
	}

	// Test paths count
	if len(cfg.Paths) != 2 {
		t.Errorf("len(Paths) = %d, want 2", len(cfg.Paths))
	}

	// Test first path
	if cfg.Paths[0].Name != "neovim" {
		t.Errorf("Paths[0].Name = %q, want %q", cfg.Paths[0].Name, "neovim")
	}

	if !cfg.Paths[0].IsFolder() {
		t.Error("Paths[0].IsFolder() = false, want true")
	}

	// Test second path
	if cfg.Paths[1].Name != "bash" {
		t.Errorf("Paths[1].Name = %q, want %q", cfg.Paths[1].Name, "bash")
	}

	if cfg.Paths[1].IsFolder() {
		t.Error("Paths[1].IsFolder() = true, want false")
	}

	if len(cfg.Paths[1].Files) != 2 {
		t.Errorf("len(Paths[1].Files) = %d, want 2", len(cfg.Paths[1].Files))
	}

	// Test root paths
	if len(cfg.RootPaths) != 1 {
		t.Errorf("len(RootPaths) = %d, want 1", len(cfg.RootPaths))
	}

	// Test hooks
	if len(cfg.Hooks.PostRestore["linux"]) != 1 {
		t.Errorf("len(Hooks.PostRestore[linux]) = %d, want 1", len(cfg.Hooks.PostRestore["linux"]))
	}

	if !cfg.Hooks.PostRestore["linux"][0].SkipOnArch {
		t.Error("Hooks.PostRestore[linux][0].SkipOnArch = false, want true")
	}
}

func TestLoadNonExistent(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load() expected error for non-existent file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() expected error for invalid YAML")
	}
}

func TestPathSpecGetTarget(t *testing.T) {
	spec := PathSpec{
		Name: "test",
		Targets: map[string]string{
			"linux":   "/home/user/.config",
			"windows": "C:\\Users\\user",
		},
	}

	tests := []struct {
		osType string
		want   string
	}{
		{"linux", "/home/user/.config"},
		{"windows", "C:\\Users\\user"},
		{"darwin", ""},
	}

	for _, tt := range tests {
		t.Run(tt.osType, func(t *testing.T) {
			got := spec.GetTarget(tt.osType)
			if got != tt.want {
				t.Errorf("GetTarget(%q) = %q, want %q", tt.osType, got, tt.want)
			}
		})
	}
}

func TestPathSpecIsFolder(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  bool
	}{
		{"empty files", []string{}, true},
		{"nil files", nil, true},
		{"with files", []string{"file1", "file2"}, false},
		{"single file", []string{"file"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := PathSpec{Files: tt.files}
			if got := spec.IsFolder(); got != tt.want {
				t.Errorf("IsFolder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpandPaths(t *testing.T) {
	cfg := &Config{
		BackupRoot: "~/gits/configs",
		Paths: []PathSpec{
			{
				Name:   "test",
				Backup: "./test",
				Files:  []string{"$CUSTOM_VAR"},
				Targets: map[string]string{
					"linux": "~/.config/test",
				},
			},
		},
	}

	envVars := map[string]string{
		"CUSTOM_VAR": "expanded_value",
	}

	cfg.ExpandPaths(envVars)

	home, _ := os.UserHomeDir()

	// Test backup root expansion
	expectedBackupRoot := filepath.Join(home, "gits/configs")
	if cfg.BackupRoot != expectedBackupRoot {
		t.Errorf("BackupRoot = %q, want %q", cfg.BackupRoot, expectedBackupRoot)
	}

	// Test file variable expansion
	if cfg.Paths[0].Files[0] != "expanded_value" {
		t.Errorf("Files[0] = %q, want %q", cfg.Paths[0].Files[0], "expanded_value")
	}

	// Test target expansion
	expectedTarget := filepath.Join(home, ".config/test")
	if cfg.Paths[0].Targets["linux"] != expectedTarget {
		t.Errorf("Targets[linux] = %q, want %q", cfg.Paths[0].Targets["linux"], expectedTarget)
	}
}
