package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
)

func TestIsPackageInstalledFromPackage_GitMethod(t *testing.T) {
	t.Parallel()

	clonedDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(clonedDir, ".git"), 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	bareDir := t.TempDir()

	tests := []struct {
		name   string
		pkg    *config.EntryPackage
		method string
		osType string
		want   bool
	}{
		{
			name: "git repo is cloned",
			pkg: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"git": {Git: &config.GitPackage{
						URL:     "https://github.com/catppuccin/lazygit",
						Targets: map[string]string{"linux": clonedDir},
					}},
				},
			},
			method: TypeGit,
			osType: "linux",
			want:   true,
		},
		{
			name: "git repo not cloned",
			pkg: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"git": {Git: &config.GitPackage{
						URL:     "https://github.com/catppuccin/delta",
						Targets: map[string]string{"linux": bareDir},
					}},
				},
			},
			method: TypeGit,
			osType: "linux",
			want:   false,
		},
		{
			name: "git pkg with no manager entry",
			pkg: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{},
			},
			method: TypeGit,
			osType: "linux",
			want:   false,
		},
		{
			name:   "nil package returns false",
			pkg:    nil,
			method: TypeGit,
			osType: "linux",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isPackageInstalledFromPackage(tt.pkg, tt.method, "test-entry", tt.osType)
			if got != tt.want {
				t.Errorf("isPackageInstalledFromPackage() = %v, want %v", got, tt.want)
			}
		})
	}
}
