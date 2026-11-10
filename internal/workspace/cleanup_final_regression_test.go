package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

func TestBoundaryFinalTemplateWholeFolderRootReplay(t *testing.T) {
	for _, crashPhase := range []string{"cleanup-source-after:.", "cleanup-source-recorded:."} {
		t.Run(crashPhase, func(t *testing.T) {
			public, private, live := t.TempDir(), t.TempDir(), t.TempDir()
			for _, root := range []string{public, private} {
				if output, err := exec.CommandContext(context.Background(), "git", "init", root).CombinedOutput(); err != nil {
					t.Fatalf("git init: %s: %v", output, err)
				}
				if err := RegisterGitExcludes(root); err != nil {
					t.Fatal(err)
				}
			}
			backup := filepath.Join(public, "app")
			if err := os.MkdirAll(backup, 0700); err != nil {
				t.Fatal(err)
			}
			for name, data := range map[string]string{
				"f.tmpl":          "template",
				"f.tmpl.rendered": "edited render",
				"f.tmpl.conflict": "saved recovery",
			} {
				if err := os.WriteFile(filepath.Join(backup, name), []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink("f.tmpl.rendered", filepath.Join(backup, "f")); err != nil {
				t.Fatal(err)
			}
			entry := config.SubEntry{Name: "entry", Backup: "./app", Targets: map[string]string{"linux": live}}
			saveRepository(t, public, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{entry}}}})
			saveRepository(t, private, &config.Config{Version: 3})
			w, err := Load(public, private)
			if err != nil {
				t.Fatal(err)
			}
			w.JournalDir = filepath.Join(t.TempDir(), "journal")
			plan, err := w.PlanMove(context.Background(), MoveRequest{
				Ref: EntryRef{Repository: Public, App: "app", Name: "entry"}, Destination: Private,
			}, &platform.Platform{OS: platform.OSLinux})
			if err != nil {
				t.Fatal(err)
			}
			faultHook = func(phase string) error {
				if phase == crashPhase {
					return errSimulatedCrash
				}
				return nil
			}
			err = w.ApplyMove(context.Background(), plan, false)
			faultHook = nil
			var txErr *TransactionError
			if !errors.As(err, &txErr) {
				t.Fatalf("expected root-removal interruption at %s, got %v", crashPhase, err)
			}
			if err := Recover(context.Background(), w.JournalDir, txErr.ID, false); err != nil {
				t.Fatalf("template-folder replay after root removal failed: %v", err)
			}
			if got, err := os.ReadFile(filepath.Join(private, "app", "f.tmpl.rendered")); err != nil || string(got) != "edited render" {
				t.Fatalf("destination rendered output changed: %q, %v", got, err)
			}
		})
	}
}
