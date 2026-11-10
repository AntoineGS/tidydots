package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

func TestControllerStaleBackupCannotReintroducePrivateSource(t *testing.T) {
	for _, stale := range []bool{true, false} {
		name := "unchanged owner still backs up"
		if stale {
			name = "changed owner is rejected"
		}
		t.Run(name, func(t *testing.T) {
			useIsolatedHome(t)
			resetCLIFlags(t)
			publicRoot, privateRoot, liveRoot := t.TempDir(), t.TempDir(), t.TempDir()
			privateConfig := &config.Config{
				Version: 3,
				Applications: []config.Application{{
					Name: "app",
					Entries: []config.SubEntry{{
						Name:   "entry",
						Method: "copy",
						Backup: "./source",
						Files:  []string{"secret"},
						Targets: map[string]string{
							platform.OSLinux: liveRoot,
						},
					}},
				}},
			}
			emptyConfig := &config.Config{Version: 3}
			saveRepoConfig(t, publicRoot, privateConfig)
			saveRepoConfig(t, privateRoot, emptyConfig)

			loaded, err := workspace.Load(publicRoot, privateRoot)
			if err != nil {
				t.Fatal(err)
			}
			if stale {
				// Simulate an owner move after CLI loading but before acquiring its
				// mutation guard: the private source is now authoritative.
				saveRepoConfig(t, publicRoot, emptyConfig)
				saveRepoConfig(t, privateRoot, privateConfig)
			}
			if err := os.WriteFile(filepath.Join(liveRoot, "secret"), []byte("synthetic-private-value\n"), 0600); err != nil {
				t.Fatal(err)
			}

			err = runWorkspaceBackup(loaded, &platform.Platform{OS: platform.OSLinux}, []string{"app", "entry"})
			publicCopy := filepath.Join(publicRoot, "source", "secret")
			if stale {
				if err == nil || !strings.Contains(err.Error(), "configuration changed") {
					t.Fatalf("stale backup error = %v, want changed-configuration rejection", err)
				}
				if _, statErr := os.Lstat(publicCopy); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("stale owner copied private live bytes to old public source: %v", statErr)
				}
				if _, statErr := os.Lstat(filepath.Join(publicRoot, ".tidydots.db")); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("stale owner opened writable state before rejection: %v", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("backup with unchanged owner config: %v", err)
			}
			got, err := os.ReadFile(publicCopy)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "synthetic-private-value\n" {
				t.Fatalf("backup bytes = %q, want unchanged-owner source bytes", got)
			}
		})
	}
}

func saveRepoConfig(t *testing.T, root string, cfg *config.Config) {
	t.Helper()
	if err := config.Save(cfg, filepath.Join(root, "tidydots.yaml")); err != nil {
		t.Fatal(err)
	}
}
