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
version: 2
backup_root: "~/gits/configurations"

entries:
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

  - name: "pacman-hooks"
    root: true
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
	if cfg.Version != 2 {
		t.Errorf("Version = %d, want 2", cfg.Version)
	}

	// Test backup root
	if cfg.BackupRoot != "~/gits/configurations" {
		t.Errorf("BackupRoot = %q, want %q", cfg.BackupRoot, "~/gits/configurations")
	}

	// Test entries count
	if len(cfg.Entries) != 3 {
		t.Errorf("len(Entries) = %d, want 3", len(cfg.Entries))
	}

	// Test first entry (neovim)
	if cfg.Entries[0].Name != "neovim" {
		t.Errorf("Entries[0].Name = %q, want %q", cfg.Entries[0].Name, "neovim")
	}

	if !cfg.Entries[0].IsFolder() {
		t.Error("Entries[0].IsFolder() = false, want true")
	}

	// Test second entry (bash)
	if cfg.Entries[1].Name != "bash" {
		t.Errorf("Entries[1].Name = %q, want %q", cfg.Entries[1].Name, "bash")
	}

	if cfg.Entries[1].IsFolder() {
		t.Error("Entries[1].IsFolder() = true, want false")
	}

	if len(cfg.Entries[1].Files) != 2 {
		t.Errorf("len(Entries[1].Files) = %d, want 2", len(cfg.Entries[1].Files))
	}

	// Test third entry (root entry)
	if cfg.Entries[2].Name != "pacman-hooks" {
		t.Errorf("Entries[2].Name = %q, want %q", cfg.Entries[2].Name, "pacman-hooks")
	}

	if !cfg.Entries[2].Root {
		t.Error("Entries[2].Root = false, want true")
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

func TestLoadUnsupportedVersion(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
version: 1
backup_root: "~/dotfiles"
entries: []
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() expected error for unsupported version 1")
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
		Version:    2,
		BackupRoot: "~/gits/configs",
		Entries: []Entry{
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
	if cfg.Entries[0].Files[0] != "expanded_value" {
		t.Errorf("Files[0] = %q, want %q", cfg.Entries[0].Files[0], "expanded_value")
	}

	// Test target expansion
	expectedTarget := filepath.Join(home, ".config/test")
	if cfg.Entries[0].Targets["linux"] != expectedTarget {
		t.Errorf("Targets[linux] = %q, want %q", cfg.Entries[0].Targets["linux"], expectedTarget)
	}
}

func TestSave(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := &Config{
		Version:    2,
		BackupRoot: "/home/user/dotfiles",
		Entries: []Entry{
			{
				Name:   "neovim",
				Files:  []string{},
				Backup: "./nvim",
				Targets: map[string]string{
					"linux":   "~/.config/nvim",
					"windows": "~/AppData/Local/nvim",
				},
			},
			{
				Name:   "pacman",
				Root:   true,
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

	if len(loaded.Entries) != len(cfg.Entries) {
		t.Errorf("len(Entries) = %d, want %d", len(loaded.Entries), len(cfg.Entries))
	}

	if loaded.Entries[0].Name != cfg.Entries[0].Name {
		t.Errorf("Entries[0].Name = %q, want %q", loaded.Entries[0].Name, cfg.Entries[0].Name)
	}
}

func TestSaveToNonexistentDirectory(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent", "subdir", "config.yaml")

	cfg := &Config{Version: 2}

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
entries: []
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should default to version 2
	if cfg.Version != 2 {
		t.Errorf("Version = %d, want 2 (default)", cfg.Version)
	}
}

func TestExpandPathsWithHooks(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Version:    2,
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
version: 2
backup_root: "~/dotfiles"
default_manager: "pacman"
manager_priority:
  - paru
  - yay
  - pacman

entries:
  - name: neovim
    description: "Editor"
    tags:
      - editor
      - dev
    package:
      managers:
        pacman: neovim
        apt: neovim
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DefaultManager != "pacman" {
		t.Errorf("DefaultManager = %q, want %q", cfg.DefaultManager, "pacman")
	}

	if len(cfg.ManagerPriority) != 3 {
		t.Errorf("len(ManagerPriority) = %d, want 3", len(cfg.ManagerPriority))
	}

	if len(cfg.Entries) != 1 {
		t.Errorf("len(Entries) = %d, want 1", len(cfg.Entries))
	}

	if cfg.Entries[0].Name != "neovim" {
		t.Errorf("Entries[0].Name = %q, want %q", cfg.Entries[0].Name, "neovim")
	}

	if len(cfg.Entries[0].Tags) != 2 {
		t.Errorf("len(Tags) = %d, want 2", len(cfg.Entries[0].Tags))
	}

	if cfg.Entries[0].Package == nil {
		t.Fatal("Entries[0].Package is nil, want non-nil")
	}

	if cfg.Entries[0].Package.Managers["pacman"] != "neovim" {
		t.Errorf("Package.Managers[pacman] = %q, want %q", cfg.Entries[0].Package.Managers["pacman"], "neovim")
	}
}

func TestExpandPathOnlyTilde(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Version:    2,
		BackupRoot: "~",
		Entries:    []Entry{},
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
		Version:    2,
		BackupRoot: "",
		Entries:    []Entry{},
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
version: 2
backup_root: "~/dotfiles"

entries:
  - name: custom-tool
    package:
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

	if len(cfg.Entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1", len(cfg.Entries))
	}

	if cfg.Entries[0].Package == nil {
		t.Fatal("Package is nil")
	}

	urlSpec := cfg.Entries[0].Package.URL["linux"]
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
version: 2
backup_root: "~/dotfiles"

entries:
  - name: custom-tool
    package:
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

	if len(cfg.Entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1", len(cfg.Entries))
	}

	if cfg.Entries[0].Package == nil {
		t.Fatal("Package is nil")
	}

	custom := cfg.Entries[0].Package.Custom
	if custom["linux"] != "curl -fsSL https://example.com/install.sh | bash" {
		t.Errorf("Custom[linux] = %q", custom["linux"])
	}
}

func TestLoadWithFzfSymlinks(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
version: 2
backup_root: "~/dotfiles"
entries: []

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

func TestGetConfigEntries(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Version: 2,
		Entries: []Entry{
			{Name: "neovim", Backup: "./nvim", Targets: map[string]string{"linux": "~/.config/nvim"}},
			{Name: "pacman", Root: true, Backup: "./pacman", Targets: map[string]string{"linux": "/etc/pacman.d"}},
			{Name: "ripgrep", Package: &EntryPackage{Managers: map[string]string{"pacman": "ripgrep"}}},
		},
	}

	// Test getting non-root entries
	entries := cfg.GetConfigEntries(false)
	if len(entries) != 1 {
		t.Errorf("GetConfigEntries(false) returned %d entries, want 1", len(entries))
	}
	if entries[0].Name != "neovim" {
		t.Errorf("GetConfigEntries(false)[0].Name = %q, want %q", entries[0].Name, "neovim")
	}

	// Test getting root entries
	rootEntries := cfg.GetConfigEntries(true)
	if len(rootEntries) != 1 {
		t.Errorf("GetConfigEntries(true) returned %d entries, want 1", len(rootEntries))
	}
	if rootEntries[0].Name != "pacman" {
		t.Errorf("GetConfigEntries(true)[0].Name = %q, want %q", rootEntries[0].Name, "pacman")
	}
}

func TestGetPackageEntries(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Version: 2,
		Entries: []Entry{
			{Name: "neovim", Backup: "./nvim", Targets: map[string]string{"linux": "~/.config/nvim"}},
			{Name: "ripgrep", Package: &EntryPackage{Managers: map[string]string{"pacman": "ripgrep"}}},
			{Name: "both", Backup: "./both", Targets: map[string]string{"linux": "~/.both"}, Package: &EntryPackage{Managers: map[string]string{"pacman": "both"}}},
		},
	}

	entries := cfg.GetPackageEntries()
	if len(entries) != 2 {
		t.Errorf("GetPackageEntries() returned %d entries, want 2", len(entries))
	}

	// Should include both ripgrep and "both" entries
	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["ripgrep"] {
		t.Error("GetPackageEntries() should include 'ripgrep'")
	}
	if !names["both"] {
		t.Error("GetPackageEntries() should include 'both'")
	}
}
