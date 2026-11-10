package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

func selectedDirectoryFixture(t *testing.T) (*Workspace, *MovePlan, string, string, string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	public, private, live := t.TempDir(), t.TempDir(), t.TempDir()
	nested := filepath.Join(public, "shell", "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "original"), []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(public, "shell", "sibling"), []byte("other owner\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	saveRepository(t, public, &config.Config{Version: 3, Applications: []config.Application{{Name: "shell", Entries: []config.SubEntry{{Name: "nested", Backup: "./shell", Files: []string{"nested"}, Targets: map[string]string{"linux": live}}}}}})
	saveRepository(t, private, &config.Config{Version: 3})
	w, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journal")
	plan, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: Public, App: "shell", Name: "nested"}, Destination: Private}, &platform.Platform{OS: platform.OSLinux})
	if err != nil {
		t.Fatal(err)
	}
	return w, plan, public, private, nested
}

func TestSelectedDirectoryMoveRetainsUnselectedSibling(t *testing.T) {
	w, plan, public, private, _ := selectedDirectoryFixture(t)
	faultHook = func(phase string) error {
		if phase == "cleanup-source-after:nested/original" {
			return errSimulatedCrash
		}
		return nil
	}
	err := w.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	var transactionErr *TransactionError
	if !errors.As(err, &transactionErr) || transactionErr.Outcome != OutcomeCommittedCleanupPending {
		t.Fatalf("ApplyMove error = %v; want committed cleanup pending", err)
	}
	if err := Recover(context.Background(), w.JournalDir, transactionErr.ID, false); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(private, "shell", "nested", "original")); err != nil || string(got) != "original\n" {
		t.Fatalf("selected subtree not moved: %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(public, "shell", "sibling")); err != nil || string(got) != "other owner\n" {
		t.Fatalf("unselected sibling changed: %q, %v", got, err)
	}
}

func TestSelectedDirectoryLateMemberAfterCommitRemainsUnowned(t *testing.T) {
	w, plan, public, private, nested := selectedDirectoryFixture(t)
	latePath := filepath.Join(nested, "late-after-commit")
	faultHook = func(phase string) error {
		if phase == "commit-marker-after" {
			return os.WriteFile(latePath, []byte("external late data\n"), 0o600)
		}
		return nil
	}
	defer func() { faultHook = nil }()
	err := w.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	var transactionErr *TransactionError
	if !errors.As(err, &transactionErr) || transactionErr.Outcome != OutcomeCommittedCleanupPending {
		t.Fatalf("ApplyMove error = %v; want committed cleanup pending", err)
	}
	privateConfig, loadErr := config.Load(filepath.Join(private, "tidydots.yaml"))
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(privateConfig.Applications) != 1 {
		t.Fatalf("committed owner not published: %#v", privateConfig.Applications)
	}
	publicConfig, loadErr := config.Load(filepath.Join(public, "tidydots.yaml"))
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if len(publicConfig.Applications) != 0 {
		t.Fatalf("source owner retained moved entry: %#v", publicConfig.Applications)
	}
	if got, readErr := os.ReadFile(latePath); readErr != nil || string(got) != "external late data\n" {
		t.Fatalf("late external file changed: %q, %v", got, readErr)
	}
	if err := Recover(context.Background(), w.JournalDir, transactionErr.ID, false); err == nil {
		t.Fatal("recovery accepted late selected-directory member")
	}
	if _, err := os.Stat(filepath.Join(w.JournalDir, transactionErr.ID+".json")); err != nil {
		t.Fatalf("pending journal was not retained: %v", err)
	}
}
