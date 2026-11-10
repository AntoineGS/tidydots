package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
)

func TestApplicationRenameJournalRecoversBothDocuments(t *testing.T) {
	for _, phase := range []string{"publish-public-after", "publish-private-after", "documents-commit-marker-after"} {
		t.Run(phase, func(t *testing.T) {
			publicDir, privateDir := t.TempDir(), t.TempDir()
			saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "old", Description: "public"}}})
			saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "old", Description: "private"}}})
			w, err := Load(publicDir, privateDir)
			if err != nil {
				t.Fatal(err)
			}
			w.JournalDir = filepath.Join(t.TempDir(), "journals")
			faultHook = func(got string) error {
				if got == phase {
					return errSimulatedCrash
				}
				return nil
			}
			err = w.RenameApplication(context.Background(), "old", "new", false)
			faultHook = nil
			if err == nil {
				t.Fatal("fault was not injected")
			}
			var te *TransactionError
			if !errors.As(err, &te) {
				t.Fatalf("error type %T", err)
			}
			pending, err := PendingTransactions(context.Background(), w.JournalDir, publicDir)
			if err != nil || len(pending) != 1 {
				t.Fatalf("pending=%v err=%v", pending, err)
			}
			if _, err := MutationGuard(context.Background(), w.JournalDir, publicDir); err == nil {
				t.Fatal("mutation guard allowed a pending transaction")
			}
			if err := Recover(context.Background(), w.JournalDir, te.ID, false); err != nil {
				t.Fatal(err)
			}
			pub, err := config.Load(filepath.Join(publicDir, "tidydots.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			priv, err := config.Load(filepath.Join(privateDir, "tidydots.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			want := "old"
			if phase == "documents-commit-marker-after" {
				want = "new"
			}
			if pub.Applications[0].Name != want || priv.Applications[0].Name != want {
				t.Fatalf("recovered names public=%q private=%q want=%q", pub.Applications[0].Name, priv.Applications[0].Name, want)
			}
		})
	}
}

func TestDryRunCRUDDoesNotCreateJournalOrChangeMemory(t *testing.T) {
	root := t.TempDir()
	saveRepository(t, root, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "entry"}}}}})
	w, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "not-created")
	if err := w.DeleteEntry(context.Background(), EntryRef{Repository: Public, App: "app", Name: "entry"}, true); err != nil {
		t.Fatal(err)
	}
	if len(w.Repositories[0].Config.Applications) != 1 {
		t.Fatal("dry-run changed loaded config")
	}
	if _, err := os.Stat(w.JournalDir); !os.IsNotExist(err) {
		t.Fatalf("dry-run created journal directory: %v", err)
	}
}
