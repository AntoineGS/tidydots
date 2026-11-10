package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/state"
)

func round1Fixture(t *testing.T, entry config.SubEntry) (*Workspace, string, string) {
	t.Helper()
	publicRoot, privateRoot, target := t.TempDir(), t.TempDir(), t.TempDir()
	if entry.Name == "" {
		entry = config.SubEntry{Name: "entry", Backup: "./app", Files: []string{"file"}, Targets: map[string]string{"linux": target}}
	}
	if err := os.MkdirAll(filepath.Join(publicRoot, "app"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(publicRoot, "app", "file"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	saveRepository(t, publicRoot, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{entry}}}})
	saveRepository(t, privateRoot, &config.Config{Version: 3})
	w, err := Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journals")
	return w, publicRoot, privateRoot
}

func round1Plan(t *testing.T, w *Workspace, updated config.SubEntry) *MovePlan {
	t.Helper()
	plan, err := w.PlanMove(context.Background(), MoveRequest{
		Ref:         EntryRef{Repository: Public, App: "app", Name: "entry"},
		Destination: Private,
		Updated:     updated,
	}, &platform.Platform{OS: platform.OSLinux})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestRound1PlanMoveRejectsStaleLoadedManifest(t *testing.T) {
	w, publicRoot, _ := round1Fixture(t, config.SubEntry{})
	external := cloneConfig(w.Repositories[0].Config)
	external.Applications = append(external.Applications, config.Application{Name: "external"})
	saveRepository(t, publicRoot, external)
	if _, err := w.PlanMove(context.Background(), MoveRequest{
		Ref: EntryRef{Repository: Public, App: "app", Name: "entry"}, Destination: Private,
	}, &platform.Platform{OS: platform.OSLinux}); err == nil {
		t.Fatal("PlanMove accepted stale parsed config and would discard an external application")
	}
}

func TestRound1MoveRevalidatesManifestImmediatelyBeforePublish(t *testing.T) {
	w, publicRoot, _ := round1Fixture(t, config.SubEntry{})
	plan := round1Plan(t, w, config.SubEntry{})
	faultHook = func(phase string) error {
		if phase == "publish-public-before" {
			external := cloneConfig(w.Repositories[0].Config)
			external.Applications = append(external.Applications, config.Application{Name: "external"})
			return config.Save(external, filepath.Join(publicRoot, "tidydots.yaml"))
		}
		return nil
	}
	defer func() { faultHook = nil }()
	err := w.ApplyMove(context.Background(), plan, false)
	if err == nil {
		t.Fatal("ApplyMove overwrote a manifest edit made after initial fingerprint validation")
	}
	actual, loadErr := config.Load(filepath.Join(publicRoot, "tidydots.yaml"))
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	for _, app := range actual.Applications {
		if app.Name == "external" {
			return
		}
	}
	t.Fatal("external manifest edit was lost")
}

func TestRound1MoveRevalidatesNewSourceHistoryAfterPublicationHook(t *testing.T) {
	publicRoot, privateRoot, liveTarget := t.TempDir(), t.TempDir(), t.TempDir()
	backup := filepath.Join(publicRoot, "app")
	if err := os.MkdirAll(backup, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "file.tmpl"), []byte("{{ .OS }}"), 0600); err != nil {
		t.Fatal(err)
	}
	entry := config.SubEntry{Name: "entry", Backup: "./app", Files: []string{"file.tmpl"}, Targets: map[string]string{"linux": liveTarget}}
	saveRepository(t, publicRoot, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{entry}}}})
	saveRepository(t, privateRoot, &config.Config{Version: 3})
	w, err := Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journals")
	plan := round1Plan(t, w, config.SubEntry{})
	const key = "./app/file.tmpl"
	faultHook = func(phase string) error {
		if phase != "publish-public-before" {
			return nil
		}
		store, err := state.Open(context.Background(), filepath.Join(publicRoot, ".tidydots.db"))
		if err != nil {
			return err
		}
		defer store.Close()
		return store.SaveRender(context.Background(), key, []byte("newly-added"), "hash", "linux", "host")
	}
	err = w.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	if err == nil {
		t.Fatal("move committed despite newly added selected render history")
	}
	if _, err := os.Stat(filepath.Join(publicRoot, "app", "file.tmpl")); err != nil {
		t.Fatalf("source template was lost after state conflict: %v", err)
	}
	if _, err := os.Stat(filepath.Join(privateRoot, "app", "file.tmpl")); !os.IsNotExist(err) {
		t.Fatalf("destination source remained after precommit state conflict: %v", err)
	}
}

func TestRound1SaveEntryRejectsInvalidConfigBeforeWrite(t *testing.T) {
	w, publicRoot, _ := round1Fixture(t, config.SubEntry{})
	path := filepath.Join(publicRoot, "tidydots.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	entry := w.Repositories[0].Config.Applications[0].Entries[0]
	entry.Method = "not-a-method"
	err = w.SaveEntry(context.Background(), EntryRef{Repository: Public, App: "app", Name: "entry"}, entry, false)
	if err == nil {
		t.Fatal("SaveEntry accepted invalid deployment method")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("invalid config was published before validation\nbefore: %s\nafter: %s", before, after)
	}
}

func TestRound1SaveApplicationDryRunValidatesAndPreservesEntries(t *testing.T) {
	w, publicRoot, privateRoot := round1Fixture(t, config.SubEntry{})
	app := w.Repositories[0].Config.Applications[0]
	app.Name = "renamed"
	app.Entries = nil // form payload must not be able to replace scoped entries
	app.Description = "edited"
	if err := w.SaveApplication(context.Background(), Public, "app", app, true); err != nil {
		t.Fatal(err)
	}
	if len(w.Repositories[0].Config.Applications[0].Entries) != 1 || w.Repositories[0].Config.Applications[0].Name != "app" {
		t.Fatal("dry-run changed loaded application data")
	}
	for _, root := range []string{publicRoot, privateRoot} {
		if _, err := os.Stat(filepath.Join(root, "tidydots.yaml")); err != nil {
			t.Fatal(err)
		}
	}
	bad := app
	bad.Name = ""
	if err := w.SaveApplication(context.Background(), Public, "app", bad, true); err == nil {
		t.Fatal("dry-run accepted invalid candidate configuration")
	}
}

func TestRound1SaveApplicationPublishesPairedRenameAtomically(t *testing.T) {
	publicRoot, privateRoot := t.TempDir(), t.TempDir()
	publicApp := config.Application{Name: "app", Description: "public-old", When: `{{ eq .OS "linux" }}`, Entries: []config.SubEntry{{Name: "pub-entry"}}}
	privateApp := config.Application{Name: "app", Description: "private-old", When: `{{ eq .OS "linux" }}`, Entries: []config.SubEntry{{Name: "priv-entry"}}}
	saveRepository(t, publicRoot, &config.Config{Version: 3, Applications: []config.Application{publicApp}})
	saveRepository(t, privateRoot, &config.Config{Version: 3, Applications: []config.Application{privateApp}})
	w, err := Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journals")
	updated := publicApp
	updated.Name = "app"
	updated.Description = "public-new"
	updated.When = `{{ eq .OS "linux" }}`
	updated.Entries = []config.SubEntry{{Name: "must-not-replace-existing-entry"}}
	faultHook = func(phase string) error {
		if phase == "publish-private-after" {
			return errSimulatedCrash
		}
		return nil
	}
	err = w.SaveApplication(context.Background(), Public, "app", updated, false)
	faultHook = nil
	var txErr *TransactionError
	if !errors.As(err, &txErr) {
		t.Fatalf("SaveApplication error = %v, want recoverable transaction error", err)
	}
	if err := Recover(context.Background(), w.JournalDir, txErr.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{publicRoot, privateRoot} {
		cfg, err := config.Load(filepath.Join(root, "tidydots.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Applications[0].Name != "app" {
			t.Fatalf("paired application rename was partially committed in %s: %q", root, cfg.Applications[0].Name)
		}
	}
}

func TestRound1SaveApplicationScopesMetadataAndPreservesEntries(t *testing.T) {
	publicRoot, privateRoot := t.TempDir(), t.TempDir()
	publicApp := config.Application{Name: "app", Description: "public", When: `{{ eq .OS "linux" }}`, Entries: []config.SubEntry{{Name: "public-entry"}}}
	privateApp := config.Application{Name: "app", Description: "private", When: `{{ eq .OS "linux" }}`, Entries: []config.SubEntry{{Name: "private-entry"}}}
	saveRepository(t, publicRoot, &config.Config{Version: 3, Applications: []config.Application{publicApp}})
	saveRepository(t, privateRoot, &config.Config{Version: 3, Applications: []config.Application{privateApp}})
	w, err := Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journals")
	updated := publicApp
	updated.Name = "app"
	updated.Description = "public updated"
	updated.When = `{{ eq .OS "windows" }}`
	updated.Entries = nil
	if err := w.SaveApplication(context.Background(), Public, "app", updated, false); err != nil {
		t.Fatalf("scoped application condition edit was rejected: %v", err)
	}
	if w.Repositories[0].Config.Applications[0].When == w.Repositories[1].Config.Applications[0].When {
		t.Fatal("scoped application save collapsed the independent conditions")
	}
	updated.Name = "renamed"
	if err := w.SaveApplication(context.Background(), Public, "app", updated, false); err != nil {
		t.Fatal(err)
	}
	pub, err := config.Load(filepath.Join(publicRoot, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	priv, err := config.Load(filepath.Join(privateRoot, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if pub.Applications[0].Name != "renamed" || pub.Applications[0].Description != "public updated" || len(pub.Applications[0].Entries) != 1 || pub.Applications[0].Entries[0].Name != "public-entry" {
		t.Fatalf("public save did not preserve scoped data: %#v", pub.Applications[0])
	}
	if priv.Applications[0].Name != "renamed" || priv.Applications[0].Description != "private" || len(priv.Applications[0].Entries) != 1 || priv.Applications[0].Entries[0].Name != "private-entry" {
		t.Fatalf("private scope was overwritten by public metadata: %#v", priv.Applications[0])
	}
	if pub.Applications[0].When != `{{ eq .OS "windows" }}` || priv.Applications[0].When != `{{ eq .OS "linux" }}` {
		t.Fatalf("scoped conditions were not preserved through rename: public=%q private=%q", pub.Applications[0].When, priv.Applications[0].When)
	}
}

func TestRound1PlanMoveAllowsRenameAndDisclosesConditions(t *testing.T) {
	target := t.TempDir()
	w, publicRoot, _ := round1Fixture(t, config.SubEntry{
		Name: "entry", Backup: "./app", Files: []string{"file"}, Targets: map[string]string{"linux": target},
		When: `{{ eq .OS "linux" }}`,
	})
	public := cloneConfig(w.Repositories[0].Config)
	public.Applications[0].When = `{{ eq .OS "linux" }}`
	if err := config.Save(public, filepath.Join(publicRoot, "tidydots.yaml")); err != nil {
		t.Fatal(err)
	}
	w, err := Load(publicRoot, w.Repositories[1].Root)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journals")
	updated := w.Repositories[0].Config.Applications[0].Entries[0]
	updated.Name = "renamed"
	plan := round1Plan(t, w, updated)
	if plan.SourceRef.Name != "entry" || plan.DestinationRef.Name != "renamed" || plan.SourceRef.Repository != Public || plan.DestinationRef.Repository != Private {
		t.Fatalf("move plan refs = %v -> %v", plan.SourceRef, plan.DestinationRef)
	}
	joined := strings.Join(plan.Summary, "\n")
	if !strings.Contains(joined, "renamed") || !strings.Contains(joined, "OS") {
		t.Fatalf("summary omits resulting identity or condition metadata: %q", joined)
	}
	if err := w.ApplyMove(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := w.Lookup(EntryRef{Repository: Private, App: "app", Name: "renamed"}); err != nil {
		t.Fatalf("renamed entry missing under new owner: %v", err)
	}
}

func TestRound1CommittedCleanupPreservesSourceIfDestinationMissing(t *testing.T) {
	w, publicRoot, privateRoot := round1Fixture(t, config.SubEntry{})
	plan := round1Plan(t, w, config.SubEntry{})
	faultHook = func(phase string) error {
		if phase == "commit-marker-after" {
			return errSimulatedCrash
		}
		return nil
	}
	err := w.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	var transactionErr *TransactionError
	if !errors.As(err, &transactionErr) {
		t.Fatalf("ApplyMove error = %v, want transaction error", err)
	}
	if err := os.Remove(filepath.Join(privateRoot, "app", "file")); err != nil {
		t.Fatal(err)
	}
	if err := Recover(context.Background(), w.JournalDir, transactionErr.ID, false); err == nil {
		t.Fatal("committed cleanup succeeded after destination data disappeared")
	}
	if _, err := os.Stat(filepath.Join(publicRoot, "app", "file")); err != nil {
		t.Fatalf("last good source was deleted: %v", err)
	}
}

func TestRound1CommitMarkerSyncAmbiguityDoesNotRollback(t *testing.T) {
	w, publicRoot, privateRoot := round1Fixture(t, config.SubEntry{})
	plan := round1Plan(t, w, config.SubEntry{})
	faultHook = func(phase string) error {
		if phase == "commit-marker-directory-sync-before" {
			return errors.New("simulated directory flush failure")
		}
		return nil
	}
	err := w.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	var txErr *TransactionError
	if !errors.As(err, &txErr) || (txErr.Outcome != OutcomeCommittedCleanupPending && txErr.Outcome != OutcomeCommitIndeterminate) {
		t.Fatalf("ApplyMove error = %v, want commit-uncertain transaction error", err)
	}
	if _, err := os.Stat(filepath.Join(publicRoot, "app", "file")); err != nil {
		t.Fatalf("possibly committed marker triggered rollback/source removal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(privateRoot, "app", "file")); err != nil {
		t.Fatalf("possibly committed destination was rolled back: %v", err)
	}
	if err := Recover(context.Background(), w.JournalDir, txErr.ID, false); err != nil {
		t.Fatalf("recovery of visible commit marker: %v", err)
	}
}

func TestRound1MutationGuardChecksPendingAfterLocks(t *testing.T) {
	root := t.TempDir()
	journalDir := filepath.Join(t.TempDir(), "journals")
	if err := os.MkdirAll(journalDir, 0700); err != nil {
		t.Fatal(err)
	}
	mutationGuardLockedHook = func() {
		mutationGuardLockedHook = nil
		j := moveJournal{Version: 1, ID: "interleaved", Phase: "prepared", PublicRoot: root, PublicConfig: filepath.Join(root, "tidydots.yaml")}
		if err := writeJournal(journalDir, j); err != nil {
			t.Fatalf("write interleaved pending journal: %v", err)
		}
	}
	defer func() { mutationGuardLockedHook = nil }()
	if _, err := MutationGuard(context.Background(), journalDir, root); err == nil {
		t.Fatal("MutationGuard authorized mutation after a journal appeared while acquiring locks")
	}
}

func TestRound1RecoverRereadsJournalUnderLocks(t *testing.T) {
	w, publicRoot, _ := round1Fixture(t, config.SubEntry{})
	plan := round1Plan(t, w, config.SubEntry{})
	faultHook = func(phase string) error {
		if phase == "publish-private-after" {
			return errSimulatedCrash
		}
		return nil
	}
	err := w.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	var txErr *TransactionError
	if !errors.As(err, &txErr) {
		t.Fatalf("ApplyMove error = %v", err)
	}
	recoveryLockedHook = func() {
		recoveryLockedHook = nil
		path := filepath.Join(w.JournalDir, txErr.ID+".json")
		journal, err := readRecoveryJournal(path, txErr.ID)
		if err != nil {
			t.Fatalf("read test journal: %v", err)
		}
		journal.Committed = true
		journal.Phase = transactionCommittedPhase
		if err := writeJournal(w.JournalDir, journal); err != nil {
			t.Fatalf("publish simulated concurrent commit: %v", err)
		}
	}
	defer func() { recoveryLockedHook = nil }()
	if err := Recover(context.Background(), w.JournalDir, txErr.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(publicRoot, "app", "file")); !os.IsNotExist(err) {
		t.Fatalf("recovery acted on stale uncommitted journal, source stat error = %v", err)
	}
}

func TestRound1LoadForInspectionAllowsValidPendingManifestsButKeepsMutationsGuarded(t *testing.T) {
	publicRoot, privateRoot := t.TempDir(), t.TempDir()
	entry := config.SubEntry{Name: "entry", Backup: "./app", Files: []string{"file"}, Targets: map[string]string{"linux": t.TempDir()}}
	saveRepository(t, publicRoot, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{entry}}}})
	saveRepository(t, privateRoot, &config.Config{Version: 3})
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir, err := journalDirectory("")
	if err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	pubRepo, err := loadRepository(Public, publicRoot)
	if err != nil {
		t.Fatal(err)
	}
	privRepo, err := loadRepository(Private, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	pending := moveJournal{
		Kind: transactionDocuments, Version: 1, ID: "inspection-pending", Phase: "prepared",
		PublicRoot: pubRepo.Root, PrivateRoot: privRepo.Root,
		PublicConfig: pubRepo.ConfigPath, PrivateConfig: privRepo.ConfigPath,
	}
	if err := writeJournal(dir, pending); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(publicRoot, privateRoot); err == nil {
		t.Fatal("mutation-oriented Load did not report the pending transaction")
	}
	w, err := LoadForInspection(publicRoot, privateRoot)
	if err != nil {
		t.Fatalf("inspection load rejected valid pending manifests: %v", err)
	}
	if len(w.Repositories) != 2 {
		t.Fatalf("inspection workspace repository count = %d", len(w.Repositories))
	}
	if err := w.SaveEntry(context.Background(), EntryRef{Repository: Public, App: "app", Name: "entry"}, entry, false); err == nil {
		t.Fatal("inspection workspace bypassed authoritative mutation guard")
	}
}

func TestRound1TemplateMoveLeavesUnattributedOrdinaryAliasDeploymentUntouched(t *testing.T) {
	publicRoot, privateRoot, targetDir, otherTarget := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	backup := filepath.Join(publicRoot, "app")
	if err := os.MkdirAll(backup, 0700); err != nil {
		t.Fatal(err)
	}
	ordinary := filepath.Join(backup, "f")
	if err := os.WriteFile(ordinary, []byte("ordinary owner"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "f.tmpl"), []byte("template"), 0600); err != nil {
		t.Fatal(err)
	}
	liveLink := filepath.Join(targetDir, "f")
	if err := os.Symlink(ordinary, liveLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	saveRepository(t, publicRoot, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{
		{Name: "template", Backup: "./app", Files: []string{"f.tmpl"}, Targets: map[string]string{"linux": targetDir}},
		{Name: "ordinary", Backup: "./app", Files: []string{"f"}, Targets: map[string]string{"linux": otherTarget}},
	}}}})
	saveRepository(t, privateRoot, &config.Config{Version: 3})
	w, err := Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journals")
	plan, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: Public, App: "app", Name: "template"}, Destination: Private}, &platform.Platform{OS: platform.OSLinux})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.data.sourceFiles) != 1 || plan.data.sourceFiles[0].rel != "f.tmpl" {
		t.Fatalf("template snapshot unexpectedly adopted ordinary alias: %#v", plan.data.sourceFiles)
	}
	if len(plan.data.targets) != 0 {
		t.Fatalf("unattributed ordinary alias was scheduled for retargeting: %#v", plan.data.targets)
	}
	if err := w.ApplyMove(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(liveLink)
	if err != nil {
		t.Fatal(err)
	}
	if got != ordinary {
		t.Fatalf("ordinary live deployment was retargeted to %q, want unchanged %q", got, ordinary)
	}
	if _, err := os.Stat(filepath.Join(privateRoot, "app", "f")); !os.IsNotExist(err) {
		t.Fatalf("private target contains an unowned template alias: %v", err)
	}
}
