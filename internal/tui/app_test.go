package tui

import (
	"reflect"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/AntoineGS/tidydots/internal/platform"
)

func TestNewModelWithManagerPrefersRepositoryHostnames(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := config.SaveAppConfig(&config.AppConfig{
		ConfigDir: t.TempDir(),
		Hostnames: []string{"local"},
	}); err != nil {
		t.Fatalf("SaveAppConfig() error = %v", err)
	}

	cfg := &config.Config{Version: 3, Hostnames: []string{"desktop", "laptop"}}
	plat := &platform.Platform{OS: platform.OSLinux}
	model := NewModelWithManager(cfg, plat, manager.New(cfg, plat), "")

	if !reflect.DeepEqual(model.HostnameChoices, cfg.Hostnames) {
		t.Errorf("HostnameChoices = %v, want repository hostnames %v", model.HostnameChoices, cfg.Hostnames)
	}
}

func TestNewModelWithManagerFallsBackToLocalHostnames(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	want := []string{"desktop", "laptop"}
	if err := config.SaveAppConfig(&config.AppConfig{
		ConfigDir: t.TempDir(),
		Hostnames: want,
	}); err != nil {
		t.Fatalf("SaveAppConfig() error = %v", err)
	}

	cfg := &config.Config{Version: 3}
	plat := &platform.Platform{OS: platform.OSLinux}
	model := NewModelWithManager(cfg, plat, manager.New(cfg, plat), "")

	if !reflect.DeepEqual(model.HostnameChoices, want) {
		t.Errorf("HostnameChoices = %v, want local fallback %v", model.HostnameChoices, want)
	}
}
