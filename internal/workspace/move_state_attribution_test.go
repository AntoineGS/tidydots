package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/state"
)

func TestPlanRenderTransferIncludesStoredCopyTargetsAcrossHostsAndOSes(t *testing.T) {
	ctx := context.Background()
	publicRoot, privateRoot := t.TempDir(), t.TempDir()
	backup := filepath.Join(publicRoot, "app")
	if err := os.MkdirAll(backup, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "f.tmpl"), []byte("template"), 0600); err != nil {
		t.Fatal(err)
	}
	entry := config.SubEntry{Name: "e", Backup: "./app", Files: []string{"f.tmpl"}, Method: config.MethodCopy, Targets: map[string]string{"linux": "~/config", "windows": `C:\Users\someone\config`}}
	saveRepository(t, publicRoot, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{entry}}}})
	saveRepository(t, privateRoot, &config.Config{Version: 3})
	store, err := state.Open(ctx, filepath.Join(publicRoot, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	stored := []state.RenderRecord{
		{TemplatePath: fmt.Sprintf("copy:%q:%q", "./app/f.tmpl", "/home/another-host/config/f"), PureRender: []byte("linux-other-home"), TemplateHash: "linux", RenderedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC), PlatformOS: platform.OSLinux, PlatformHost: "another-host"},
		{TemplatePath: fmt.Sprintf("copy:%q:%q", "./app/f.tmpl", `C:\Users\another-host\config\f`), PureRender: []byte("windows-origin"), TemplateHash: "windows", RenderedAt: time.Date(2025, 1, 2, 3, 4, 6, 0, time.UTC), PlatformOS: platform.OSWindows, PlatformHost: "windows-host"},
	}
	for _, record := range stored {
		if err := store.SaveRender(ctx, record.TemplatePath, record.PureRender, record.TemplateHash, record.PlatformOS, record.PlatformHost); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = state.OpenReadOnly(ctx, filepath.Join(publicRoot, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	d := &moveData{source: &Repository{Root: publicRoot}, destination: &Repository{Root: privateRoot}, entry: entry,
		backupRel: "app", sourceFiles: []fileSnapshot{{rel: "f.tmpl"}}, platform: &platform.Platform{OS: platform.OSLinux}}
	if err := planRenderTransfer(ctx, d, d.platform); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if len(d.stateRecords) != len(stored) {
		t.Fatalf("attributed %d records, want %d: %#v", len(d.stateRecords), len(stored), d.stateRecords)
	}
	for i := range stored {
		if d.stateRecords[i].TemplatePath != stored[i].TemplatePath || d.stateRecords[i].PlatformHost != stored[i].PlatformHost || string(d.stateRecords[i].PureRender) != string(stored[i].PureRender) {
			t.Errorf("record[%d] = %#v, want preserved source record %#v", i, d.stateRecords[i], stored[i])
		}
	}
	j := &moveJournal{ID: "copy-attribution-test", DestinationRoot: privateRoot, StateKeys: d.stateKeys, StateRecords: cloneRenderRecords(d.stateRecords)}
	if err := importMoveState(ctx, j.ID, j); err != nil {
		t.Fatal(err)
	}
	destination, err := state.OpenReadOnly(ctx, filepath.Join(privateRoot, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	if destination == nil {
		t.Fatal("copy-template state import did not create destination history")
	}
	defer func() { _ = destination.Close() }() //nolint:errcheck // best-effort test cleanup
	for _, record := range stored {
		got, err := destination.ExportRenders(ctx, []string{record.TemplatePath})
		if err != nil || len(got) != 1 || got[0].PlatformHost != record.PlatformHost || string(got[0].PureRender) != string(record.PureRender) {
			t.Errorf("destination history for %q = %#v, %v", record.TemplatePath, got, err)
		}
	}
}

func TestMoveCopyHistoryAttributionDoesNotDependOnCurrentMethod(t *testing.T) {
	for _, method := range []string{config.MethodCopy, config.MethodSymlink, ""} {
		name := method
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
			store := openStateStore(t, filepath.Join(sourceRoot, ".tidydots.db"))
			selectedKey := fmt.Sprintf("copy:%q:%q", "./app/nested/f.tmpl", "C:/Users/other/config/f")
			unrelatedKey := fmt.Sprintf("copy:%q:%q", "./other/nested/f.tmpl", "C:/Users/other/config/f")
			for _, key := range []string{selectedKey, unrelatedKey} {
				if err := store.SaveRender(ctx, key, []byte(key), "hash", "windows", "other-host"); err != nil {
					t.Fatal(err)
				}
			}
			d := &moveData{source: &Repository{Root: sourceRoot}, destination: &Repository{Root: destinationRoot},
				entry: config.SubEntry{Method: method}, backupRel: "app", sourceFiles: []fileSnapshot{{rel: "nested/f.tmpl"}}}
			if err := planRenderTransfer(ctx, d, &platform.Platform{OS: platform.OSLinux}); err != nil {
				t.Fatal(err)
			}
			if len(d.stateRecords) != 1 || d.stateRecords[0].TemplatePath != selectedKey {
				t.Fatalf("method %q attributed %d rows, want only selected source tuple: %#v", method, len(d.stateRecords), d.stateRecords)
			}
			j := &moveJournal{ID: "method-independent-state", SourceRoot: sourceRoot, SourceRel: "app", DestinationRoot: destinationRoot,
				StateKeys: append([]string(nil), d.stateKeys...), StateRecords: cloneRenderRecords(d.stateRecords)}
			if err := revalidateMoveSourceState(ctx, j); err != nil {
				t.Fatalf("precommit state revalidation: %v", err)
			}
			if err := importMoveState(ctx, j.ID, j); err != nil {
				t.Fatal(err)
			}
			if err := verifyCommittedMoveState(ctx, j); err != nil {
				t.Fatalf("verify imported state: %v", err)
			}
			if err := cleanupMoveState(ctx, j); err != nil {
				t.Fatalf("clean exact source snapshot: %v", err)
			}
			remaining, err := store.EnumerateRenders(ctx)
			if err != nil || len(remaining) != 1 || remaining[0].TemplatePath != unrelatedKey {
				t.Fatalf("source history after cleanup = %#v, %v; want unrelated same-target key only", remaining, err)
			}
			destination, err := state.OpenReadOnly(ctx, filepath.Join(destinationRoot, ".tidydots.db"))
			if err != nil {
				t.Fatal(err)
			}
			if destination == nil {
				t.Fatal("destination render history missing after import")
			}
			defer func() { _ = destination.Close() }() //nolint:errcheck // best-effort test cleanup
			imported, err := destination.EnumerateRenders(ctx)
			if err != nil || len(imported) != 1 || imported[0].TemplatePath != selectedKey {
				t.Fatalf("destination history = %#v, %v; want selected source tuple only", imported, err)
			}
		})
	}
}

func TestPlanRenderTransferRemapsUniqueLegacyKeyReversibly(t *testing.T) {
	ctx := context.Background()
	sourceRoot := t.TempDir()
	backup := filepath.Join(sourceRoot, "app")
	if err := os.MkdirAll(backup, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "f.tmpl"), []byte("template"), 0600); err != nil {
		t.Fatal(err)
	}
	store := openStateStore(t, filepath.Join(sourceRoot, ".tidydots.db"))
	if err := store.SaveRender(ctx, "f.tmpl", []byte("legacy baseline"), "hash", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	d := &moveData{source: &Repository{Root: sourceRoot}, destination: &Repository{Root: t.TempDir()}, entry: config.SubEntry{}, backupRel: "app", sourceFiles: []fileSnapshot{{rel: "f.tmpl"}}}
	if err := planRenderTransfer(ctx, d, &platform.Platform{OS: platform.OSLinux}); err != nil {
		t.Fatal(err)
	}
	if len(d.stateRecords) != 1 || d.stateRecords[0].TemplatePath != "f.tmpl" {
		t.Fatalf("source cleanup snapshot lost original key: %#v", d.stateRecords)
	}
	j := &moveJournal{ID: "legacy-remap-test", DestinationRoot: d.destination.Root, SourceRoot: sourceRoot, SourceRel: "app", StateKeys: d.stateKeys, StateRecords: cloneRenderRecords(d.stateRecords)}
	if err := importMoveState(ctx, j.ID, j); err != nil {
		t.Fatal(err)
	}
	destination, err := state.OpenReadOnly(ctx, filepath.Join(j.DestinationRoot, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	if destination == nil {
		t.Fatal("unique legacy history was not imported")
	}
	defer func() { _ = destination.Close() }() //nolint:errcheck // best-effort test cleanup
	canonical, err := destination.ExportRenders(ctx, []string{"./app/f.tmpl"})
	if err != nil || len(canonical) != 1 || string(canonical[0].PureRender) != "legacy baseline" {
		t.Fatalf("canonical destination history = %#v, %v", canonical, err)
	}
	if err := verifyCommittedMoveState(ctx, j); err != nil {
		t.Fatalf("verify remapped destination history: %v", err)
	}
	if err := cleanupMoveState(ctx, j); err != nil {
		t.Fatalf("cleanup original legacy source snapshot: %v", err)
	}
	if remaining, err := store.ExportRenders(ctx, []string{"f.tmpl"}); err != nil || len(remaining) != 0 {
		t.Fatalf("legacy source history remained after cleanup: %#v, %v", remaining, err)
	}
}

func TestSymlinkMoveRemapsUniqueLegacyCopyTupleAndCleansOriginal(t *testing.T) {
	ctx := context.Background()
	sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
	backup := filepath.Join(sourceRoot, "app", "nested")
	if err := os.MkdirAll(backup, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "f.tmpl"), []byte("template"), 0600); err != nil {
		t.Fatal(err)
	}
	legacyKey := fmt.Sprintf("copy:%q:%q", "nested/f.tmpl", "C:/Users/other/config/f")
	canonicalKey := fmt.Sprintf("copy:%q:%q", "./app/nested/f.tmpl", "C:/Users/other/config/f")
	store := openStateStore(t, filepath.Join(sourceRoot, ".tidydots.db"))
	if err := store.SaveRender(ctx, legacyKey, []byte("old copy baseline"), "hash", "windows", "other-host"); err != nil {
		t.Fatal(err)
	}
	d := &moveData{source: &Repository{Root: sourceRoot}, destination: &Repository{Root: destinationRoot},
		entry: config.SubEntry{Method: config.MethodSymlink}, backupRel: "app", sourceFiles: []fileSnapshot{{rel: "nested/f.tmpl"}}}
	if err := planRenderTransfer(ctx, d, &platform.Platform{OS: platform.OSLinux}); err != nil {
		t.Fatal(err)
	}
	if len(d.stateRecords) != 1 || d.stateRecords[0].TemplatePath != legacyKey {
		t.Fatalf("legacy source snapshot = %#v, want exact original tuple", d.stateRecords)
	}
	j := &moveJournal{ID: "symlink-legacy-copy", SourceRoot: sourceRoot, SourceRel: "app", DestinationRoot: destinationRoot,
		StateKeys: d.stateKeys, StateRecords: cloneRenderRecords(d.stateRecords)}
	if err := revalidateMoveSourceState(ctx, j); err != nil {
		t.Fatalf("revalidate captured legacy copy row: %v", err)
	}
	if err := importMoveState(ctx, j.ID, j); err != nil {
		t.Fatal(err)
	}
	destination, err := state.OpenReadOnly(ctx, filepath.Join(destinationRoot, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	if destination == nil {
		t.Fatal("legacy copy tuple was not imported")
	}
	if err := destination.VerifyImportedRenders(ctx, j.ID, destinationMoveStateRecords(j)); err != nil {
		t.Fatalf("verify remapped copy receipt: %v", err)
	}
	canonical, err := destination.ExportRenders(ctx, []string{canonicalKey})
	if err != nil || len(canonical) != 1 || string(canonical[0].PureRender) != "old copy baseline" {
		t.Fatalf("remapped destination tuple = %#v, %v", canonical, err)
	}
	if err := destination.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cleanupMoveState(ctx, j); err != nil {
		t.Fatalf("cleanup original legacy tuple: %v", err)
	}
	remaining, err := store.ExportRenders(ctx, []string{legacyKey})
	if err != nil || len(remaining) != 0 {
		t.Fatalf("original legacy source tuple remains: %#v, %v", remaining, err)
	}
}

func TestPlanRenderTransferRejectsAmbiguousLegacyKeyAcrossEntries(t *testing.T) {
	ctx := context.Background()
	sourceRoot := t.TempDir()
	for _, dir := range []string{"app", "other"} {
		path := filepath.Join(sourceRoot, dir)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "f.tmpl"), []byte("template"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	entry := config.SubEntry{Name: "move", Backup: "./app", Files: []string{"f.tmpl"}}
	other := config.SubEntry{Name: "other", Backup: "./other", Files: []string{"f.tmpl"}}
	store := openStateStore(t, filepath.Join(sourceRoot, ".tidydots.db"))
	if err := store.SaveRender(ctx, "f.tmpl", []byte("ambiguous"), "hash", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	d := &moveData{source: &Repository{Root: sourceRoot, Config: &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{entry}}, {Name: "other", Entries: []config.SubEntry{other}}}}},
		entry: entry, backupRel: "app", sourceFiles: []fileSnapshot{{rel: "f.tmpl"}}}
	if err := planRenderTransfer(ctx, d, &platform.Platform{OS: platform.OSLinux}); err == nil {
		t.Fatal("legacy history key shared by two configured sources was attributed to the moved entry")
	}
}

func TestRollbackMoveStatePreservesUnrelatedDestinationHistory(t *testing.T) {
	ctx := context.Background()
	sourceRoot := t.TempDir()
	source := openStateStore(t, filepath.Join(sourceRoot, ".tidydots.db"))
	if err := source.SaveRender(ctx, "./app/f.tmpl", []byte("baseline"), "hash", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	rows, err := source.ExportRenders(ctx, []string{"./app/f.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	destinationRoot := t.TempDir()
	j := &moveJournal{ID: "rollback-state-test", DestinationRoot: destinationRoot, StateRecords: rows}
	if err := importMoveState(ctx, j.ID, j); err != nil {
		t.Fatal(err)
	}
	destination := openStateStore(t, filepath.Join(destinationRoot, ".tidydots.db"))
	if err := destination.SaveRender(ctx, "./unrelated.tmpl", []byte("external"), "hash", "linux", "other-host"); err != nil {
		t.Fatal(err)
	}
	if err := destination.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rollbackMoveState(ctx, j); err != nil {
		t.Fatal(err)
	}
	check, err := state.OpenReadOnly(ctx, filepath.Join(destinationRoot, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	if check == nil {
		t.Fatal("rollback removed destination state database containing unrelated history")
	}
	defer func() { _ = check.Close() }() //nolint:errcheck // best-effort test cleanup
	unrelated, err := check.ExportRenders(ctx, []string{"./unrelated.tmpl"})
	if err != nil || len(unrelated) != 1 || string(unrelated[0].PureRender) != "external" {
		t.Fatalf("unrelated history after rollback = %#v, %v", unrelated, err)
	}
	imported, err := check.ExportRenders(ctx, []string{"./app/f.tmpl"})
	if err != nil || len(imported) != 0 {
		t.Fatalf("transaction-owned imported rows after rollback = %#v, %v", imported, err)
	}
}

func TestVerifyCommittedMoveStateRejectsMissingImportedHistory(t *testing.T) {
	ctx := context.Background()
	sourceRoot := t.TempDir()
	source := openStateStore(t, filepath.Join(sourceRoot, ".tidydots.db"))
	if err := source.SaveRender(ctx, "./app/f.tmpl", []byte("baseline"), "hash", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	rows, err := source.ExportRenders(ctx, []string{"./app/f.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	destinationRoot := t.TempDir()
	j := &moveJournal{ID: "committed-state-test", SourceRoot: sourceRoot, DestinationRoot: destinationRoot, StateRecords: rows, StateKeys: []string{"./app/f.tmpl"}}
	if err := importMoveState(ctx, j.ID, j); err != nil {
		t.Fatal(err)
	}
	imported := openStateStore(t, filepath.Join(destinationRoot, ".tidydots.db"))
	if err := imported.DeleteImportedRenders(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	if err := imported.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cleanupMoveState(ctx, j); err == nil {
		t.Fatal("committed cleanup accepted missing destination import/receipt")
	}
	remainingSource, err := source.ExportRenders(ctx, []string{"./app/f.tmpl"})
	if err != nil || len(remainingSource) != 1 || string(remainingSource[0].PureRender) != "baseline" {
		t.Fatalf("source history was deleted after destination state loss: %#v, %v", remainingSource, err)
	}
}

func TestRevalidateMoveSourceStateRejectsNewCopyTargetHistory(t *testing.T) {
	ctx := context.Background()
	sourceRoot := t.TempDir()
	store := openStateStore(t, filepath.Join(sourceRoot, ".tidydots.db"))
	firstKey := fmt.Sprintf("copy:%q:%q", "./app/f.tmpl", "/home/host-a/config/f")
	if err := store.SaveRender(ctx, firstKey, []byte("captured"), "hash", "linux", "host-a"); err != nil {
		t.Fatal(err)
	}
	captured, err := store.ExportRenders(ctx, []string{firstKey})
	if err != nil {
		t.Fatal(err)
	}
	j := &moveJournal{SourceRoot: sourceRoot, SourceRel: "app", StateKeys: []string{"./app/f.tmpl", firstKey}, StateRecords: captured}
	secondKey := fmt.Sprintf("copy:%q:%q", "./app/f.tmpl", `C:\Users\host-b\config\f`)
	if err := store.SaveRender(ctx, secondKey, []byte("new windows baseline"), "hash", "windows", "host-b"); err != nil {
		t.Fatal(err)
	}
	if err := revalidateMoveSourceState(ctx, j); err == nil {
		t.Fatal("source revalidation accepted a new copy target tuple for the selected template")
	}
}

func TestRevalidateMoveSourceStateDetectsDatabaseCreatedAfterPlanning(t *testing.T) {
	ctx := context.Background()
	sourceRoot := t.TempDir()
	backup := filepath.Join(sourceRoot, "app")
	if err := os.MkdirAll(backup, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "f.tmpl"), []byte("template"), 0600); err != nil {
		t.Fatal(err)
	}
	d := &moveData{source: &Repository{Root: sourceRoot}, entry: config.SubEntry{}, backupRel: "app", sourceFiles: []fileSnapshot{{rel: "f.tmpl"}}}
	if err := planRenderTransfer(ctx, d, &platform.Platform{OS: platform.OSLinux}); err != nil {
		t.Fatal(err)
	}
	if len(d.stateKeys) != 1 || d.stateKeys[0] != "./app/f.tmpl" {
		t.Fatalf("source keys captured without existing DB = %#v", d.stateKeys)
	}
	j := &moveJournal{SourceRoot: sourceRoot, SourceRel: "app", StateKeys: d.stateKeys, StateRecords: d.stateRecords}
	store := openStateStore(t, filepath.Join(sourceRoot, ".tidydots.db"))
	if err := store.SaveRender(ctx, "./app/f.tmpl", []byte("added later"), "hash", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	if err := revalidateMoveSourceState(ctx, j); err == nil {
		t.Fatal("source revalidation accepted render history created after planning")
	}
}

func openStateStore(t *testing.T, path string) *state.Store {
	t.Helper()
	store, err := state.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() }) //nolint:errcheck // best-effort test cleanup
	return store
}
