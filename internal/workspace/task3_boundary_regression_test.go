package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/state"
)

func boundaryPlan(t *testing.T, w *Workspace) *MovePlan {
	t.Helper()
	plan, err := w.PlanMove(context.Background(), MoveRequest{
		Ref: EntryRef{Repository: Public, App: "app", Name: "entry"}, Destination: Private,
	}, &platform.Platform{OS: platform.OSLinux})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func boundaryCommitCrash(t *testing.T, w *Workspace, plan *MovePlan) string {
	t.Helper()
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
		t.Fatalf("expected committed crash, got %v", err)
	}
	return transactionErr.ID
}

func TestBoundaryRecoveryRejectsDestinationBackupRootSymlink(t *testing.T) {
	w, public, private := round1Fixture(t, config.SubEntry{})
	id := boundaryCommitCrash(t, w, boundaryPlan(t, w))
	backup := filepath.Join(private, "app")
	outside := filepath.Join(t.TempDir(), "saved")
	if err := os.Rename(backup, outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, backup); err != nil {
		t.Fatal(err)
	}
	if err := Recover(context.Background(), w.JournalDir, id, false); err == nil {
		t.Fatal("recovery accepted a symlinked committed backup root")
	}
	if _, err := os.Stat(filepath.Join(public, "app", "file")); err != nil {
		t.Fatalf("source was removed despite replaced destination root: %v", err)
	}
}

func TestBoundaryRecoveryRejectsNewAttributableTemplateArtifact(t *testing.T) {
	w, public, _ := round1Fixture(t, config.SubEntry{})
	cfg := cloneConfig(w.Repositories[0].Config)
	cfg.Applications[0].Entries[0].Files = []string{"file.tmpl"}
	saveRepository(t, public, cfg)
	backup := filepath.Join(public, "app")
	if err := os.Remove(filepath.Join(backup, "file")); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"file.tmpl": "template", "file.tmpl.rendered": "rendered"} {
		if err := os.WriteFile(filepath.Join(backup, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("file.tmpl.rendered", filepath.Join(backup, "file")); err != nil {
		t.Fatal(err)
	}
	w, err := Load(public, w.Repositories[1].Root)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journal")
	id := boundaryCommitCrash(t, w, boundaryPlan(t, w))
	artifact := filepath.Join(backup, "file.tmpl.conflict")
	if err := os.WriteFile(artifact, []byte("new private recovery data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Recover(context.Background(), w.JournalDir, id, false); err == nil {
		t.Fatal("recovery ignored a newly attributable conflict artifact")
	}
	if _, err := os.Stat(filepath.Join(public, "app", "file.tmpl")); err != nil {
		t.Fatalf("source was removed despite new artifact: %v", err)
	}
}

func TestBoundaryApplyRejectsArtifactAddedAfterPlanning(t *testing.T) {
	w, public, private := round1Fixture(t, config.SubEntry{})
	cfg := cloneConfig(w.Repositories[0].Config)
	cfg.Applications[0].Entries[0].Files = []string{"file.tmpl"}
	saveRepository(t, public, cfg)
	backup := filepath.Join(public, "app")
	if err := os.Remove(filepath.Join(backup, "file")); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"file.tmpl": "template", "file.tmpl.rendered": "rendered"} {
		if err := os.WriteFile(filepath.Join(backup, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("file.tmpl.rendered", filepath.Join(backup, "file")); err != nil {
		t.Fatal(err)
	}
	w, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journal")
	plan := boundaryPlan(t, w)
	if err := os.WriteFile(filepath.Join(backup, "file.tmpl.conflict"), []byte("new recovery data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.ApplyMove(context.Background(), plan, false); err == nil {
		t.Fatal("apply accepted an attributable artifact added after planning")
	}
	if _, err := os.Stat(filepath.Join(backup, "file.tmpl")); err != nil {
		t.Fatalf("source was modified after late artifact conflict: %v", err)
	}
}

func TestBoundaryDurablySyncsCreatedAncestorChain(t *testing.T) {
	w, public, private := round1Fixture(t, config.SubEntry{})
	cfg := cloneConfig(w.Repositories[0].Config)
	cfg.Applications[0].Entries[0].Backup = "./deep/inner/app"
	saveRepository(t, public, cfg)
	source := filepath.Join(public, "deep", "inner", "app")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "file"), []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	w, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "nested", "journal")
	plan := boundaryPlan(t, w)
	var synced []string
	syncDirectoryObserver = func(path string) { synced = append(synced, filepath.Clean(path)) }
	defer func() { syncDirectoryObserver = nil }()
	if err := w.ApplyMove(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(private, "deep"), private, filepath.Dir(w.JournalDir)} {
		if !containsPath(synced, path) {
			t.Errorf("created directory chain did not sync %q; calls: %v", path, synced)
		}
	}
}

func containsPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	for _, path := range paths {
		if filepath.Clean(path) == want {
			return true
		}
	}
	return false
}

func TestBoundaryRecoveryRejectsNewAttributableRenderHistory(t *testing.T) {
	w, public, private := round1Fixture(t, config.SubEntry{})
	cfg := cloneConfig(w.Repositories[0].Config)
	cfg.Applications[0].Entries[0].Files = []string{"file.tmpl"}
	saveRepository(t, public, cfg)
	if err := os.WriteFile(filepath.Join(public, "app", "file.tmpl"), []byte("template"), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(context.Background(), filepath.Join(public, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRender(context.Background(), "./app/file.tmpl", []byte("base"), "hash", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	w, err = Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journal")
	id := boundaryCommitCrash(t, w, boundaryPlan(t, w))
	store, err = state.Open(context.Background(), filepath.Join(public, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRender(context.Background(), "./app/file.tmpl", []byte("late baseline"), "new", "linux", "other-host"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Recover(context.Background(), w.JournalDir, id, false); err == nil {
		t.Fatal("recovery ignored a newly appended source render row")
	}
	if _, err := os.Stat(filepath.Join(public, "app", "file.tmpl")); err != nil {
		t.Fatalf("source was removed despite new history: %v", err)
	}
}

func TestBoundaryApplyRechecksEffectiveArtifactIgnore(t *testing.T) {
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
	for name, data := range map[string]string{"f.tmpl": "template", "f.tmpl.rendered": "edited"} {
		if err := os.WriteFile(filepath.Join(backup, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("f.tmpl.rendered", filepath.Join(backup, "f")); err != nil {
		t.Fatal(err)
	}
	saveRepository(t, public, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "entry", Backup: "./app", Files: []string{"f.tmpl"}, Targets: map[string]string{"linux": live}}}}}})
	saveRepository(t, private, &config.Config{Version: 3})
	w, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journal")
	plan := boundaryPlan(t, w)
	if err := os.WriteFile(filepath.Join(private, ".gitignore"), []byte("!app/f.tmpl.rendered\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := w.ApplyMove(context.Background(), plan, false); err == nil {
		t.Fatal("apply accepted a generated artifact that is no longer ignored")
	}
}

func TestBoundaryScopedApplicationConditionsRemainIndependent(t *testing.T) {
	public, private := t.TempDir(), t.TempDir()
	saveRepository(t, public, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", When: `{{ eq .OS "linux" }}`, Entries: []config.SubEntry{{Name: "public"}}}}})
	privApp := config.Application{Name: "app", When: `{{ eq .OS "windows" }}`, Entries: []config.SubEntry{{Name: "private"}}}
	saveRepository(t, private, &config.Config{Version: 3, Applications: []config.Application{privApp}})
	w, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journal")
	privApp.Description = "scope-local metadata"
	if err := w.SaveApplication(context.Background(), Private, "app", privApp, false); err != nil {
		t.Fatalf("independent scoped condition was rejected: %v", err)
	}
	if w.Repositories[0].Config.Applications[0].When == w.Repositories[1].Config.Applications[0].When {
		t.Fatal("scoped save collapsed the independent app conditions")
	}
}

func TestBoundaryLoadedConfigDigestMatchesParsedSnapshot(t *testing.T) {
	root := t.TempDir()
	contents := []byte("version: 3\napplications:\n  - name: app\n    entries:\n      - name: entry\n")
	if err := os.WriteFile(filepath.Join(root, "tidydots.yaml"), contents, 0600); err != nil {
		t.Fatal(err)
	}
	repo, err := loadRepository(Public, root)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.Parse(contents)
	if err != nil {
		t.Fatal(err)
	}
	if repo.ConfigDigest != digest(contents) || repo.Config.Applications[0].Name != parsed.Applications[0].Name {
		t.Fatal("repository fingerprint and parsed model do not come from the same byte snapshot")
	}
}

func TestBoundaryMoveSummaryUsesResultingSetupMetadata(t *testing.T) {
	public, private := t.TempDir(), t.TempDir()
	entry := config.SubEntry{Name: "entry", When: `{{ eq .OS "linux" }}`, Check: map[string]string{"linux": "old-check"}, Run: map[string]string{"linux": "old-run"}}
	saveRepository(t, public, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{entry}}}})
	saveRepository(t, private, &config.Config{Version: 3})
	w, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	entry.When = `{{ eq .OS "windows" }}`
	entry.Check = map[string]string{"linux": "new-check"}
	entry.Run = map[string]string{"linux": "new-run"}
	plan, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: Public, App: "app", Name: "entry"}, Destination: Private, Updated: entry}, &platform.Platform{OS: platform.OSLinux})
	if err != nil {
		t.Fatal(err)
	}
	summary := strings.Join(plan.Summary, "\n")
	if !strings.Contains(summary, "windows") || !strings.Contains(summary, "new-check") || !strings.Contains(summary, "new-run") {
		t.Fatalf("summary does not describe the resulting setup definition: %s", summary)
	}
}

func TestBoundaryReviewNestedWholeFolderCleanupProgress(t *testing.T) {
	w, public, private := round1Fixture(t, config.SubEntry{})
	cfg := cloneConfig(w.Repositories[0].Config)
	cfg.Applications[0].Entries[0].Files = nil
	saveRepository(t, public, cfg)
	nested := filepath.Join(public, "app", "nested")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "payload"), []byte("nested data"), 0600); err != nil {
		t.Fatal(err)
	}
	fresh, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	fresh.JournalDir = w.JournalDir
	plan := boundaryPlan(t, fresh)
	if err := fresh.ApplyMove(context.Background(), plan, false); err != nil {
		var txErr *TransactionError
		if errors.As(err, &txErr) {
			t.Fatalf("nested cleanup failed (%v), replay: %v", err, Recover(context.Background(), fresh.JournalDir, txErr.ID, false))
		}
		t.Fatal(err)
	}
}

func TestBoundaryReviewWholeFolderReplayAfterRootRemoval(t *testing.T) {
	w, public, private := round1Fixture(t, config.SubEntry{})
	cfg := cloneConfig(w.Repositories[0].Config)
	cfg.Applications[0].Entries[0].Files = nil
	saveRepository(t, public, cfg)
	fresh, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	fresh.JournalDir = w.JournalDir
	plan := boundaryPlan(t, fresh)
	faultHook = func(phase string) error {
		if phase == "cleanup-source-after:." {
			return errSimulatedCrash
		}
		return nil
	}
	err = fresh.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	var txErr *TransactionError
	if !errors.As(err, &txErr) {
		t.Fatalf("expected source-root removal interruption, got %v", err)
	}
	if err := Recover(context.Background(), fresh.JournalDir, txErr.ID, false); err != nil {
		t.Fatalf("recovery could not reconcile source-root removal: %v", err)
	}
}

func TestBoundaryReviewNestedDirectoryRemovalReplay(t *testing.T) {
	w, public, private := round1Fixture(t, config.SubEntry{})
	cfg := cloneConfig(w.Repositories[0].Config)
	cfg.Applications[0].Entries[0].Files = nil
	saveRepository(t, public, cfg)
	nested := filepath.Join(public, "app", "nested")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "payload"), []byte("nested data"), 0600); err != nil {
		t.Fatal(err)
	}
	fresh, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	fresh.JournalDir = w.JournalDir
	plan := boundaryPlan(t, fresh)
	faultHook = func(phase string) error {
		if phase == "cleanup-source-after:nested" {
			return errSimulatedCrash
		}
		return nil
	}
	err = fresh.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	var txErr *TransactionError
	if !errors.As(err, &txErr) {
		t.Fatalf("expected nested-directory removal interruption, got %v", err)
	}
	if err := Recover(context.Background(), fresh.JournalDir, txErr.ID, false); err != nil {
		t.Fatalf("recovery could not reconcile nested-directory removal: %v", err)
	}
}

func TestBoundaryReviewExplicitDirectoryCleanup(t *testing.T) {
	w, public, private := round1Fixture(t, config.SubEntry{})
	cfg := cloneConfig(w.Repositories[0].Config)
	cfg.Applications[0].Entries[0].Files = []string{"nested"}
	saveRepository(t, public, cfg)
	nested := filepath.Join(public, "app", "nested")
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "payload"), []byte("nested data"), 0600); err != nil {
		t.Fatal(err)
	}
	fresh, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	fresh.JournalDir = w.JournalDir
	if err := fresh.ApplyMove(context.Background(), boundaryPlan(t, fresh), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatalf("selected source directory remains after move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(public, "app", "file")); err != nil {
		t.Fatalf("unselected sibling was removed: %v", err)
	}
}

func TestBoundaryReviewOutstandingIntentAcceptsExactBeforeImage(t *testing.T) {
	w, _, _ := round1Fixture(t, config.SubEntry{})
	plan := boundaryPlan(t, w)
	faultHook = func(phase string) error {
		if phase == "cleanup-source-before:file" {
			return errSimulatedCrash
		}
		return nil
	}
	err := w.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	var txErr *TransactionError
	if !errors.As(err, &txErr) {
		t.Fatalf("expected interruption after source-removal intent, got %v", err)
	}
	if err := Recover(context.Background(), w.JournalDir, txErr.ID, false); err != nil {
		t.Fatalf("recovery did not resume removal of the exact before-image: %v", err)
	}
}

func TestBoundaryReviewOutstandingIntentRejectsChangedBeforeImage(t *testing.T) {
	w, public, _ := round1Fixture(t, config.SubEntry{})
	plan := boundaryPlan(t, w)
	faultHook = func(phase string) error {
		if phase == "cleanup-source-before:file" {
			return errSimulatedCrash
		}
		return nil
	}
	err := w.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	var txErr *TransactionError
	if !errors.As(err, &txErr) {
		t.Fatalf("expected interruption after source-removal intent, got %v", err)
	}
	path := filepath.Join(public, "app", "file")
	if err := os.WriteFile(path, []byte("changed external data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Recover(context.Background(), w.JournalDir, txErr.ID, false); err == nil {
		t.Fatal("recovery accepted changed contents under an outstanding removal intent")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "changed external data" {
		t.Fatalf("recovery altered the changed source: %q, %v", got, err)
	}
}

func TestBoundaryReviewRecreatedCleanedArtifactIsConflict(t *testing.T) {
	w, public, private := round1Fixture(t, config.SubEntry{})
	cfg := cloneConfig(w.Repositories[0].Config)
	cfg.Applications[0].Entries[0].Files = []string{"file.tmpl"}
	saveRepository(t, public, cfg)
	backup := filepath.Join(public, "app")
	if err := os.Remove(filepath.Join(backup, "file")); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"file.tmpl": "template", "file.tmpl.rendered": "render", "file.tmpl.conflict": "old recovery"} {
		if err := os.WriteFile(filepath.Join(backup, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("file.tmpl.rendered", filepath.Join(backup, "file")); err != nil {
		t.Fatal(err)
	}
	fresh, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	fresh.JournalDir = w.JournalDir
	plan := boundaryPlan(t, fresh)
	faultHook = func(phase string) error {
		if phase == "cleanup-source-after:file.tmpl.rendered" {
			return errSimulatedCrash
		}
		return nil
	}
	err = fresh.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	var txErr *TransactionError
	if !errors.As(err, &txErr) {
		t.Fatalf("expected artifact-cleanup interruption, got %v", err)
	}
	conflict := filepath.Join(backup, "file.tmpl.conflict")
	if err := os.WriteFile(conflict, []byte("new external private recovery"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Recover(context.Background(), fresh.JournalDir, txErr.ID, false); err == nil {
		t.Fatal("recovery ignored a recreated completed artifact removal")
	}
	if got, err := os.ReadFile(conflict); err != nil || string(got) != "new external private recovery" {
		t.Fatalf("recovery altered the recreated artifact: %q, %v", got, err)
	}
}

func TestBoundaryReviewRecreatedCleanedOrdinaryFileIsConflict(t *testing.T) {
	w, public, private := round1Fixture(t, config.SubEntry{})
	plan := boundaryPlan(t, w)
	faultHook = func(phase string) error {
		if phase == "cleanup-source-after:file" {
			return errSimulatedCrash
		}
		return nil
	}
	err := w.ApplyMove(context.Background(), plan, false)
	faultHook = nil
	var txErr *TransactionError
	if !errors.As(err, &txErr) {
		t.Fatalf("expected ordinary-file cleanup interruption, got %v", err)
	}
	recreated := filepath.Join(public, "app", "file")
	if err := os.WriteFile(recreated, []byte("external replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Recover(context.Background(), w.JournalDir, txErr.ID, false); err == nil {
		t.Fatal("recovery ignored a recreated completed ordinary-file removal")
	}
	if got, err := os.ReadFile(recreated); err != nil || string(got) != "external replacement" {
		t.Fatalf("recovery altered the recreated file: %q, %v", got, err)
	}
	_ = private
}

func TestBoundaryReviewDefaultJournalAncestorsDurableBeforeMoveCommit(t *testing.T) {
	w, _, _ := round1Fixture(t, config.SubEntry{})
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	w.JournalDir = ""
	plan := boundaryPlan(t, w)
	var synced []string
	syncDirectoryObserver = func(path string) { synced = append(synced, filepath.Clean(path)) }
	defer func() { syncDirectoryObserver = nil }()
	var missing []string
	faultHook = func(phase string) error {
		if phase == "commit-marker-directory-sync-before" {
			for _, required := range []string{xdg, filepath.Join(xdg, "tidydots")} {
				if !containsPath(synced, required) {
					missing = append(missing, required)
				}
			}
		}
		return nil
	}
	defer func() { faultHook = nil }()
	if err := w.ApplyMove(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Fatalf("default journal ancestors were not synced before commit: %v (calls %v)", missing, synced)
	}
}

func TestBoundaryReviewDefaultJournalAncestorsDurableBeforeDocumentCommit(t *testing.T) {
	public, private := t.TempDir(), t.TempDir()
	saveRepository(t, public, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "entry"}}}}})
	saveRepository(t, private, &config.Config{Version: 3})
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	w, err := Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	updated := w.Repositories[0].Config.Applications[0]
	updated.Description = "changed"
	var synced []string
	syncDirectoryObserver = func(path string) { synced = append(synced, filepath.Clean(path)) }
	defer func() { syncDirectoryObserver = nil }()
	var missing []string
	faultHook = func(phase string) error {
		if phase == "commit-marker-directory-sync-before" {
			for _, required := range []string{xdg, filepath.Join(xdg, "tidydots")} {
				if !containsPath(synced, required) {
					missing = append(missing, required)
				}
			}
		}
		return nil
	}
	defer func() { faultHook = nil }()
	if err := w.SaveApplication(context.Background(), Public, "app", updated, false); err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Fatalf("document transaction did not sync default journal ancestors before commit: %v", missing)
	}
}
