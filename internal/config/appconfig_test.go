package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Note: Tests that modify HOME environment variable cannot run in parallel
// because os.Setenv affects the entire process.

func TestSaveAppConfig(t *testing.T) {
	// Create a temporary home directory
	tmpDir := t.TempDir()

	// Override HOME for this test
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &AppConfig{
		ConfigDir: "/path/to/configs",
	}

	err := SaveAppConfig(cfg)
	if err != nil {
		t.Fatalf("SaveAppConfig() error = %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, appConfigDir, appConfigFile)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Verify content
	data, _ := os.ReadFile(configPath)
	content := string(data)

	if !strings.Contains(content, "config_dir: /path/to/configs") {
		t.Errorf("Config file should contain config_dir, got: %s", content)
	}

	if !strings.Contains(content, "# dot-manager app configuration") {
		t.Error("Config file should have header comment")
	}
}

func TestLoadAppConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Override HOME
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create config directory
	configDir := filepath.Join(tmpDir, appConfigDir)
	os.MkdirAll(configDir, 0755)

	// Create a configs directory that will be referenced
	configsRepo := filepath.Join(tmpDir, "my-dotfiles")
	os.MkdirAll(configsRepo, 0755)

	// Write app config
	configContent := "config_dir: " + configsRepo + "\n"
	configPath := filepath.Join(configDir, appConfigFile)
	os.WriteFile(configPath, []byte(configContent), 0644)

	// Load it
	cfg, err := LoadAppConfig()
	if err != nil {
		t.Fatalf("LoadAppConfig() error = %v", err)
	}

	if cfg.ConfigDir != configsRepo {
		t.Errorf("ConfigDir = %q, want %q", cfg.ConfigDir, configsRepo)
	}
}

func TestLoadAppConfigWithTilde(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create config directory
	configDir := filepath.Join(tmpDir, appConfigDir)
	os.MkdirAll(configDir, 0755)

	// Create a configs directory
	configsRepo := filepath.Join(tmpDir, "dotfiles")
	os.MkdirAll(configsRepo, 0755)

	// Write app config with tilde
	configContent := "config_dir: ~/dotfiles\n"
	configPath := filepath.Join(configDir, appConfigFile)
	os.WriteFile(configPath, []byte(configContent), 0644)

	cfg, err := LoadAppConfig()
	if err != nil {
		t.Fatalf("LoadAppConfig() error = %v", err)
	}

	expected := filepath.Join(tmpDir, "dotfiles")
	if cfg.ConfigDir != expected {
		t.Errorf("ConfigDir = %q, want %q", cfg.ConfigDir, expected)
	}
}

func TestLoadAppConfigNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	_, err := LoadAppConfig()
	if err == nil {
		t.Error("LoadAppConfig() should error when config doesn't exist")
	}

	if !strings.Contains(err.Error(), "app config not found") {
		t.Errorf("Error should mention 'app config not found', got: %v", err)
	}
}

func TestLoadAppConfigInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	configDir := filepath.Join(tmpDir, appConfigDir)
	os.MkdirAll(configDir, 0755)

	// Write invalid YAML
	configPath := filepath.Join(configDir, appConfigFile)
	os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644)

	_, err := LoadAppConfig()
	if err == nil {
		t.Error("LoadAppConfig() should error for invalid YAML")
	}
}

func TestLoadAppConfigEmptyConfigDir(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	configDir := filepath.Join(tmpDir, appConfigDir)
	os.MkdirAll(configDir, 0755)

	// Write config with empty config_dir
	configPath := filepath.Join(configDir, appConfigFile)
	os.WriteFile(configPath, []byte("config_dir: \"\"\n"), 0644)

	_, err := LoadAppConfig()
	if err == nil {
		t.Error("LoadAppConfig() should error when config_dir is empty")
	}

	if !strings.Contains(err.Error(), "config_dir not set") {
		t.Errorf("Error should mention 'config_dir not set', got: %v", err)
	}
}

func TestLoadAppConfigNonexistentConfigDir(t *testing.T) {
	tmpDir := t.TempDir()

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	configDir := filepath.Join(tmpDir, appConfigDir)
	os.MkdirAll(configDir, 0755)

	// Write config pointing to nonexistent directory
	configPath := filepath.Join(configDir, appConfigFile)
	os.WriteFile(configPath, []byte("config_dir: /nonexistent/path\n"), 0644)

	_, err := LoadAppConfig()
	if err == nil {
		t.Error("LoadAppConfig() should error when config_dir doesn't exist")
	}

	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("Error should mention 'does not exist', got: %v", err)
	}
}

func TestGetRepoConfigPath(t *testing.T) {
	t.Parallel()

	cfg := &AppConfig{
		ConfigDir: "/home/user/dotfiles",
	}

	got := cfg.GetRepoConfigPath()
	want := "/home/user/dotfiles/dot-manager.yaml"

	if got != want {
		t.Errorf("GetRepoConfigPath() = %q, want %q", got, want)
	}
}

func TestAppConfigPath(t *testing.T) {
	// Cannot run in parallel - uses os.UserHomeDir()
	path := AppConfigPath()

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config/dot-manager/config.yaml")

	if path != expected {
		t.Errorf("AppConfigPath() = %q, want %q", path, expected)
	}
}
