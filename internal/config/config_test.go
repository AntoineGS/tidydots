package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Load() expected error for non-existent file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
			got := spec.GetTarget(tt.osType)
			if got != tt.want {
				t.Errorf("GetTarget(%q) = %q, want %q", tt.osType, got, tt.want)
			}
		})
	}
}

func TestPathSpecIsFolder(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			spec := PathSpec{Files: tt.files}
			if got := spec.IsFolder(); got != tt.want {
				t.Errorf("IsFolder() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpandPaths(t *testing.T) {
	t.Parallel()
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

func TestSave(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{
		Version:    1,
		BackupRoot: "/home/user/dotfiles",
		Paths: []PathSpec{
			{
				Name:   "neovim",
				Files:  []string{},
				Backup: "./nvim",
				Targets: map[string]string{
					"linux":   "~/.config/nvim",
					"windows": "~/AppData/Local/nvim",
				},
			},
		},
		RootPaths: []PathSpec{
			{
				Name:   "pacman",
				Files:  []string{"hook.conf"},
				Backup: "./pacman",
				Targets: map[string]string{
					"linux": "/etc/pacman.d/hooks",
				},
			},
		},
	}

	err := Save(cfg, configPath)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load it back and verify
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() after Save() error = %v", err)
	}

	if loaded.Version != cfg.Version {
		t.Errorf("Version = %d, want %d", loaded.Version, cfg.Version)
	}

	if loaded.BackupRoot != cfg.BackupRoot {
		t.Errorf("BackupRoot = %q, want %q", loaded.BackupRoot, cfg.BackupRoot)
	}

	if len(loaded.Paths) != len(cfg.Paths) {
		t.Errorf("len(Paths) = %d, want %d", len(loaded.Paths), len(cfg.Paths))
	}

	if loaded.Paths[0].Name != cfg.Paths[0].Name {
		t.Errorf("Paths[0].Name = %q, want %q", loaded.Paths[0].Name, cfg.Paths[0].Name)
	}
}

func TestSaveToNonexistentDirectory(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent", "subdir", "config.yaml")

	cfg := &Config{Version: 1}

	err := Save(cfg, configPath)
	if err == nil {
		t.Error("Save() should error when directory doesn't exist")
	}
}

func TestLoadDefaultVersion(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Config without explicit version
	configContent := `
backup_root: "~/dotfiles"
paths: []
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should default to version 1
	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1 (default)", cfg.Version)
	}
}

func TestExpandPathsWithRootPaths(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		BackupRoot: "~/dotfiles",
		RootPaths: []PathSpec{
			{
				Name:   "system",
				Backup: "./system",
				Files:  []string{"$VAR_FILE"},
				Targets: map[string]string{
					"linux": "~/.system",
				},
			},
		},
	}

	envVars := map[string]string{
		"VAR_FILE": "expanded_file",
	}

	cfg.ExpandPaths(envVars)

	home, _ := os.UserHomeDir()
	expectedTarget := filepath.Join(home, ".system")

	if cfg.RootPaths[0].Targets["linux"] != expectedTarget {
		t.Errorf("RootPaths target = %q, want %q", cfg.RootPaths[0].Targets["linux"], expectedTarget)
	}

	if cfg.RootPaths[0].Files[0] != "expanded_file" {
		t.Errorf("RootPaths files = %q, want %q", cfg.RootPaths[0].Files[0], "expanded_file")
	}
}

func TestExpandPathsWithHooks(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		BackupRoot: "~/dotfiles",
		Hooks: Hooks{
			PostRestore: map[string][]Hook{
				"linux": {
					{
						Type:   "test",
						Source: "~/source",
						Plugins: []Plugin{
							{
								Name: "plugin",
								Path: "~/plugins/test",
							},
						},
					},
				},
			},
		},
	}

	cfg.ExpandPaths(nil)

	home, _ := os.UserHomeDir()

	expectedSource := filepath.Join(home, "source")
	if cfg.Hooks.PostRestore["linux"][0].Source != expectedSource {
		t.Errorf("Hook source = %q, want %q", cfg.Hooks.PostRestore["linux"][0].Source, expectedSource)
	}

	expectedPluginPath := filepath.Join(home, "plugins/test")
	if cfg.Hooks.PostRestore["linux"][0].Plugins[0].Path != expectedPluginPath {
		t.Errorf("Plugin path = %q, want %q", cfg.Hooks.PostRestore["linux"][0].Plugins[0].Path, expectedPluginPath)
	}
}

func TestPathSpecGetTargetEmptyTargets(t *testing.T) {
	t.Parallel()
	spec := PathSpec{
		Name:    "test",
		Targets: map[string]string{},
	}

	got := spec.GetTarget("linux")
	if got != "" {
		t.Errorf("GetTarget() = %q, want empty string", got)
	}
}

func TestPathSpecGetTargetNilTargets(t *testing.T) {
	t.Parallel()
	spec := PathSpec{
		Name:    "test",
		Targets: nil,
	}

	got := spec.GetTarget("linux")
	if got != "" {
		t.Errorf("GetTarget() = %q, want empty string", got)
	}
}

func TestLoadWithPackages(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
version: 1
backup_root: "~/dotfiles"
paths: []

packages:
  default_manager: "pacman"
  manager_priority:
    - paru
    - yay
    - pacman
  items:
    - name: neovim
      description: "Editor"
      managers:
        pacman: neovim
        apt: neovim
      tags:
        - editor
        - dev
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Packages.DefaultManager != "pacman" {
		t.Errorf("DefaultManager = %q, want %q", cfg.Packages.DefaultManager, "pacman")
	}

	if len(cfg.Packages.ManagerPriority) != 3 {
		t.Errorf("len(ManagerPriority) = %d, want 3", len(cfg.Packages.ManagerPriority))
	}

	if len(cfg.Packages.Items) != 1 {
		t.Errorf("len(Items) = %d, want 1", len(cfg.Packages.Items))
	}

	if cfg.Packages.Items[0].Name != "neovim" {
		t.Errorf("Items[0].Name = %q, want %q", cfg.Packages.Items[0].Name, "neovim")
	}

	if len(cfg.Packages.Items[0].Tags) != 2 {
		t.Errorf("len(Tags) = %d, want 2", len(cfg.Packages.Items[0].Tags))
	}
}

func TestExpandPathOnlyTilde(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		BackupRoot: "~",
		Paths:      []PathSpec{},
	}

	cfg.ExpandPaths(nil)

	home, _ := os.UserHomeDir()
	if cfg.BackupRoot != home {
		t.Errorf("BackupRoot = %q, want %q", cfg.BackupRoot, home)
	}
}

func TestExpandPathEmpty(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		BackupRoot: "",
		Paths:      []PathSpec{},
	}

	cfg.ExpandPaths(nil)

	if cfg.BackupRoot != "" {
		t.Errorf("BackupRoot = %q, want empty string", cfg.BackupRoot)
	}
}

func TestLoadWithURLInstall(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
version: 1
backup_root: "~/dotfiles"
paths: []

packages:
  items:
    - name: custom-tool
      url:
        linux:
          url: "https://example.com/tool.tar.gz"
          command: "tar xzf {file} -C /usr/local/bin"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Packages.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(cfg.Packages.Items))
	}

	urlSpec := cfg.Packages.Items[0].URL["linux"]
	if urlSpec.URL != "https://example.com/tool.tar.gz" {
		t.Errorf("URL = %q, want %q", urlSpec.URL, "https://example.com/tool.tar.gz")
	}

	if urlSpec.Command != "tar xzf {file} -C /usr/local/bin" {
		t.Errorf("Command = %q", urlSpec.Command)
	}
}

func TestLoadWithCustomInstall(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
version: 1
backup_root: "~/dotfiles"
paths: []

packages:
  items:
    - name: custom-tool
      custom:
        linux: "curl -fsSL https://example.com/install.sh | bash"
        windows: "iwr https://example.com/install.ps1 | iex"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Packages.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(cfg.Packages.Items))
	}

	custom := cfg.Packages.Items[0].Custom
	if custom["linux"] != "curl -fsSL https://example.com/install.sh | bash" {
		t.Errorf("Custom[linux] = %q", custom["linux"])
	}
}

func TestLoadWithFzfSymlinks(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
version: 1
backup_root: "~/dotfiles"
paths: []

hooks:
  post_restore:
    linux:
      - type: "zsh-plugins"
        plugins:
          - name: "fzf"
            repo: "https://github.com/junegunn/fzf.git"
            path: "/usr/share/fzf"
        fzf_symlinks:
          - target: "shell/completion.zsh"
            link: "completion.zsh"
          - target: "shell/key-bindings.zsh"
            link: "key-bindings.zsh"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	hooks := cfg.Hooks.PostRestore["linux"]
	if len(hooks) != 1 {
		t.Fatalf("len(hooks) = %d, want 1", len(hooks))
	}

	fzfSymlinks := hooks[0].FzfSymlinks
	if len(fzfSymlinks) != 2 {
		t.Fatalf("len(FzfSymlinks) = %d, want 2", len(fzfSymlinks))
	}

	if fzfSymlinks[0].Target != "shell/completion.zsh" {
		t.Errorf("FzfSymlinks[0].Target = %q", fzfSymlinks[0].Target)
	}
}
