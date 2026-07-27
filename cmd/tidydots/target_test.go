package main

import (
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/spf13/cobra"
)

type targetTestRenderer struct{}

func (targetTestRenderer) RenderString(_ string, value string) (string, error) {
	return value, nil
}

func targetTestConfig() *config.Config {
	return &config.Config{
		Version:         3,
		BackupRoot:      "/repo",
		DefaultManager:  "pacman",
		ManagerPriority: []string{"pacman", "apt"},
		Applications: []config.Application{
			{
				Name:    "nvim",
				Package: &config.EntryPackage{Managers: map[string]config.ManagerValue{"pacman": {PackageName: "neovim"}}},
				Entries: []config.SubEntry{
					{Name: "config", Backup: "./nvim", Targets: map[string]string{"linux": "~/.config/nvim"}},
					{Name: "setup", Check: map[string]string{"linux": "test -x nvim"}, Run: map[string]string{"linux": "install-nvim"}},
				},
			},
			{
				Name: "shell",
				Entries: []config.SubEntry{
					{Name: "config", Backup: "./shell", Targets: map[string]string{"linux": "~/.config/shell"}},
				},
			},
			{
				Name: "windows-only",
				When: "false",
				Entries: []config.SubEntry{
					{Name: "config", Backup: "./windows", Targets: map[string]string{"windows": "~/windows"}},
				},
			},
		},
	}
}

func TestSelectConfigTarget_NoTargetReturnsOriginalConfig(t *testing.T) {
	cfg := targetTestConfig()

	got, err := selectConfigTarget(cfg, targetTestRenderer{}, nil, true)
	if err != nil {
		t.Fatalf("selectConfigTarget() error = %v", err)
	}
	if got != cfg {
		t.Fatal("selectConfigTarget() should return the original config when no target is supplied")
	}
}

func TestSelectConfigTarget_Application(t *testing.T) {
	cfg := targetTestConfig()

	got, err := selectConfigTarget(cfg, targetTestRenderer{}, []string{"nvim"}, true)
	if err != nil {
		t.Fatalf("selectConfigTarget() error = %v", err)
	}
	if len(got.Applications) != 1 || got.Applications[0].Name != "nvim" {
		t.Fatalf("selected applications = %#v, want only nvim", got.Applications)
	}
	if len(got.Applications[0].Entries) != 2 {
		t.Fatalf("selected entries = %d, want 2", len(got.Applications[0].Entries))
	}
	if len(cfg.Applications) != 3 || len(cfg.Applications[0].Entries) != 2 {
		t.Fatal("selectConfigTarget() mutated the source config")
	}
	if got.BackupRoot != cfg.BackupRoot || got.DefaultManager != cfg.DefaultManager {
		t.Fatal("selectConfigTarget() did not preserve global config settings")
	}
	if got.Applications[0].Package == nil {
		t.Fatal("application selection should retain package metadata")
	}
}

func TestSelectConfigTarget_EntryIsScopedToApplication(t *testing.T) {
	cfg := targetTestConfig()

	got, err := selectConfigTarget(cfg, targetTestRenderer{}, []string{"nvim", "config"}, false)
	if err != nil {
		t.Fatalf("selectConfigTarget() error = %v", err)
	}
	if len(got.Applications) != 1 || got.Applications[0].Name != "nvim" {
		t.Fatalf("selected applications = %#v, want only nvim", got.Applications)
	}
	if len(got.Applications[0].Entries) != 1 || got.Applications[0].Entries[0].Backup != "./nvim" {
		t.Fatalf("selected entries = %#v, want nvim/config", got.Applications[0].Entries)
	}
	if got.Applications[0].Package != nil {
		t.Fatal("entry selection should remove package metadata from the copied application")
	}
	if len(cfg.Applications[0].Entries) != 2 || cfg.Applications[0].Package == nil {
		t.Fatal("selectConfigTarget() mutated the source application's entries")
	}
}

func TestSelectConfigTarget_Errors(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		allowSetup  bool
		errContains string
	}{
		{name: "unknown application", args: []string{"missing"}, allowSetup: true, errContains: `application "missing" not found`},
		{name: "names are case sensitive", args: []string{"Nvim"}, allowSetup: true, errContains: `application "Nvim" not found`},
		{name: "excluded application", args: []string{"windows-only"}, allowSetup: true, errContains: `application "windows-only" does not match current conditions`},
		{name: "unknown entry", args: []string{"nvim", "missing"}, allowSetup: true, errContains: `entry "missing" not found in application "nvim"`},
		{name: "setup rejected", args: []string{"nvim", "setup"}, allowSetup: false, errContains: `entry "setup" in application "nvim" is a setup entry`},
		{name: "too many arguments", args: []string{"nvim", "config", "extra"}, allowSetup: true, errContains: "accepts at most 2 arguments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := selectConfigTarget(targetTestConfig(), targetTestRenderer{}, tt.args, tt.allowSetup)
			if err == nil || !strings.Contains(err.Error(), tt.errContains) {
				t.Fatalf("selectConfigTarget() error = %v, want substring %q", err, tt.errContains)
			}
		})
	}
}

func TestSelectConfigTarget_RestoreAllowsSetupEntry(t *testing.T) {
	got, err := selectConfigTarget(targetTestConfig(), targetTestRenderer{}, []string{"nvim", "setup"}, true)
	if err != nil {
		t.Fatalf("selectConfigTarget() error = %v", err)
	}
	if len(got.Applications[0].Entries) != 1 || !got.Applications[0].Entries[0].IsSetup() {
		t.Fatalf("selected entries = %#v, want nvim/setup", got.Applications[0].Entries)
	}
}

func TestValidateTargetArgs(t *testing.T) {
	cmd := &cobra.Command{Use: "restore"}
	if err := validateTargetArgs(cmd, []string{"nvim", "config"}); err != nil {
		t.Fatalf("validateTargetArgs() error = %v for two arguments", err)
	}
	if err := validateTargetArgs(cmd, []string{"nvim", "config", "extra"}); err == nil {
		t.Fatal("validateTargetArgs() expected an error for three arguments")
	}
}

func TestValidateInteractiveTarget(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		interactive bool
		wantErr     bool
	}{
		{name: "non-interactive target", args: []string{"nvim"}},
		{name: "interactive without target", interactive: true},
		{name: "interactive app target", interactive: true, args: []string{"nvim"}, wantErr: true},
		{name: "interactive entry target", interactive: true, args: []string{"nvim", "config"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInteractiveTarget(tt.interactive, tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateInteractiveTarget() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "cannot be used with --interactive") {
				t.Fatalf("validateInteractiveTarget() error = %v, want interactive conflict", err)
			}
		})
	}
}
