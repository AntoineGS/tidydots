package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/state"
)

func TestMoveEntryRetainsDeployment(t *testing.T) {
	publicDir, privateDir, liveDir := t.TempDir(), t.TempDir(), t.TempDir()
	source := filepath.Join(publicDir, "shell", "env")
	if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("same content\n"), 0750); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(liveDir, "env")
	if err := os.Symlink(source, target); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "shell", Entries: []config.SubEntry{{Name: "env", Backup: "./shell", Files: []string{"env"}, Targets: map[string]string{"linux": liveDir}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: Public, App: "shell", Name: "env"}, Destination: Private}, &platform.Platform{OS: platform.OSLinux})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ApplyMove(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(privateDir, "shell", "env"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "same content\n" {
		t.Fatalf("source content = %q", got)
	}
	if _, err := os.Lstat(source); !os.IsNotExist(err) {
		t.Fatalf("old source still exists: %v", err)
	}
	link, err := os.Readlink(target)
	if err != nil {
		t.Fatal(err)
	}
	resolved := link
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(target), resolved)
	}
	if filepath.Clean(resolved) != filepath.Join(privateDir, "shell", "env") {
		t.Fatalf("link = %q (resolved %q)", link, resolved)
	}
	loaded, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := loaded.Lookup(EntryRef{Repository: Private, App: "shell", Name: "env"}); err != nil {
		t.Fatal(err)
	}
	if len(loaded.Repositories[0].Config.Applications) != 0 {
		t.Fatal("source application was not removed")
	}
}

func TestMoveRecoversAcrossDurableBoundaries(t *testing.T) {
	phases := []string{"stage-source-before", "stage-source-after", "stage-source-recorded", "publish-source-before", "publish-source-after", "publish-source-recorded", "transfer-state-before", "transfer-state-after", "transfer-state-recorded", "retarget-links-before", "retarget-links-after", "retarget-links-recorded", "publish-public-before", "publish-public-after", "publish-public-recorded", "publish-private-before", "publish-private-after", "publish-private-recorded", "commit-marker-after", "cleanup-source-after:env"}
	for _, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			publicDir, privateDir, liveDir := t.TempDir(), t.TempDir(), t.TempDir()
			source := filepath.Join(publicDir, "shell", "env")
			if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(source, []byte("data\n"), 0750); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(liveDir, "env")
			if err := os.Symlink(source, target); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "shell", Entries: []config.SubEntry{{Name: "env", Backup: "./shell", Files: []string{"env"}, Targets: map[string]string{"linux": liveDir}}}}}})
			saveRepository(t, privateDir, &config.Config{Version: 3})
			w, err := Load(publicDir, privateDir)
			if err != nil {
				t.Fatal(err)
			}
			w.JournalDir = filepath.Join(t.TempDir(), "journal")
			plan, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: Public, App: "shell", Name: "env"}, Destination: Private}, &platform.Platform{OS: platform.OSLinux})
			if err != nil {
				t.Fatal(err)
			}
			faultHook = func(got string) error {
				if got == phase {
					return errSimulatedCrash
				}
				return nil
			}
			err = w.ApplyMove(context.Background(), plan, false)
			faultHook = nil
			if err == nil {
				t.Fatal("fault was not injected")
			}
			var transactionErr *TransactionError
			if !errors.As(err, &transactionErr) {
				t.Fatalf("error type = %T, want TransactionError", err)
			}
			if err := Recover(context.Background(), w.JournalDir, transactionErr.ID, false); err != nil {
				t.Fatal(err)
			}
			if err := Recover(context.Background(), w.JournalDir, transactionErr.ID, false); err == nil {
				t.Fatal("second recovery unexpectedly found an already completed journal")
			}
			pub, err := config.Load(filepath.Join(publicDir, "tidydots.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			priv, err := config.Load(filepath.Join(privateDir, "tidydots.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			owners := 0
			if len(pub.Applications) > 0 {
				owners += len(pub.Applications[0].Entries)
			}
			if len(priv.Applications) > 0 {
				owners += len(priv.Applications[0].Entries)
			}
			if owners != 1 {
				t.Fatalf("entry owners = %d", owners)
			}
			link, err := os.Readlink(target)
			if err != nil {
				t.Fatal(err)
			}
			resolved := link
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(target), resolved)
			}
			got, err := os.ReadFile(resolved)
			if err != nil || string(got) != "data\n" {
				t.Fatalf("deployment target %s: %q %v", resolved, got, err)
			}
			if transactionErr.Outcome == OutcomeCommittedCleanupPending {
				if len(priv.Applications) == 0 {
					t.Fatal("committed move did not retain destination owner")
				}
			} else if len(pub.Applications) == 0 {
				t.Fatal("rolled-back move did not retain source owner")
			}
			if _, err := os.Stat(filepath.Join(w.JournalDir, transactionErr.ID+".json")); !os.IsNotExist(err) {
				t.Fatalf("journal still exists: %v", err)
			}
		})
	}
}

func TestMoveCopiesAllHostTemplateHistory(t *testing.T) {
	publicDir, privateDir, liveDir := t.TempDir(), t.TempDir(), t.TempDir()
	source := filepath.Join(publicDir, "shell", "prompt.tmpl")
	if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("hello {{ .User }}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source+".rendered", []byte("edited rendered output"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("prompt.tmpl.rendered", filepath.Join(filepath.Dir(source), "prompt")); err != nil {
		t.Fatal(err)
	}
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "shell", Entries: []config.SubEntry{{Name: "prompt", Backup: "./shell", Files: []string{"prompt.tmpl"}, Targets: map[string]string{"linux": filepath.Join(liveDir, "prompt")}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3})
	store, err := state.Open(context.Background(), filepath.Join(publicDir, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRender(context.Background(), "./shell/prompt.tmpl", []byte("linux desktop"), "hash-1", "linux", "desktop"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRender(context.Background(), "./shell/prompt.tmpl", []byte("windows laptop"), "hash-2", "windows", "laptop"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journal")
	plan, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: Public, App: "shell", Name: "prompt"}, Destination: Private}, &platform.Platform{OS: platform.OSLinux})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ApplyMove(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	dest, err := state.OpenReadOnly(context.Background(), filepath.Join(privateDir, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dest.Close()
	records, err := dest.ExportRenders(context.Background(), []string{"./shell/prompt.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("destination render records = %d", len(records))
	}
	for _, r := range records {
		if string(r.PureRender) == "" {
			t.Fatal("empty render record")
		}
	}
	src, err := state.OpenReadOnly(context.Background(), filepath.Join(publicDir, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	remaining, err := src.ExportRenders(context.Background(), []string{"./shell/prompt.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("source retained %d moved records", len(remaining))
	}
	if got, err := os.ReadFile(filepath.Join(privateDir, "shell", "prompt.tmpl.rendered")); err != nil || string(got) != "edited rendered output" {
		t.Fatalf("edited render = %q, %v", got, err)
	}
}

func TestMoveFolderPreservesEmptyDirectoriesModesAndEditedRender(t *testing.T) {
	publicDir, privateDir, liveDir := t.TempDir(), t.TempDir(), t.TempDir()
	backup := filepath.Join(publicDir, "terminal")
	if err := os.MkdirAll(filepath.Join(backup, "empty"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "bin"), []byte("#!/bin/sh\n"), 0750); err != nil {
		t.Fatal(err)
	}
	template := filepath.Join(backup, "prompt.tmpl")
	if err := os.WriteFile(template, []byte("new template"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(template+".rendered", []byte("user edited render"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("prompt.tmpl.rendered", filepath.Join(backup, "prompt")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(liveDir, "terminal")
	if err := os.Symlink(backup, target); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "terminal", Entries: []config.SubEntry{{Name: "all", Backup: "./terminal", Targets: map[string]string{"linux": target}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journals")
	plan, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: Public, App: "terminal", Name: "all"}, Destination: Private}, &platform.Platform{OS: platform.OSLinux})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ApplyMove(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(privateDir, "terminal")
	for _, path := range []string{filepath.Join(dest, "empty"), filepath.Join(dest, "prompt.tmpl.rendered"), filepath.Join(dest, "prompt")} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("moved artifact %s: %v", path, err)
		}
	}
	info, err := os.Stat(filepath.Join(dest, "bin"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0750 {
		t.Fatalf("executable mode = %o", info.Mode().Perm())
	}
	if got, err := os.ReadFile(filepath.Join(dest, "prompt.tmpl.rendered")); err != nil || string(got) != "user edited render" {
		t.Fatalf("render = %q, %v", got, err)
	}
	link, err := os.Readlink(target)
	if err != nil {
		t.Fatal(err)
	}
	resolved := link
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(target), resolved)
	}
	if filepath.Clean(resolved) != dest {
		t.Fatalf("folder deployment resolves to %s, want %s", resolved, dest)
	}
	if _, err := os.Lstat(backup); !os.IsNotExist(err) {
		t.Fatalf("old source remains: %v", err)
	}
}

func TestMoveCopyKeepsLiveTargetAndExternalRecoveryBackup(t *testing.T) {
	publicDir, privateDir, liveDir := t.TempDir(), t.TempDir(), t.TempDir()
	backup := filepath.Join(publicDir, "app")
	if err := os.MkdirAll(backup, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(backup, "settings.tmpl")
	if err := os.WriteFile(source, []byte("{{ .OS }}"), 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(liveDir, "settings")
	if err := os.WriteFile(target, []byte("live edits"), 0640); err != nil {
		t.Fatal(err)
	}
	recovery := target + ".tidydots.bak"
	if err := os.WriteFile(recovery, []byte("external backup"), 0600); err != nil {
		t.Fatal(err)
	}
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "settings", Backup: "./app", Files: []string{"settings.tmpl"}, Method: config.MethodCopy, Targets: map[string]string{"linux": liveDir}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3})
	store, err := state.Open(context.Background(), filepath.Join(publicDir, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	key := "copy:\"./app/settings.tmpl\":\"" + target + "\""
	if err := store.SaveRender(context.Background(), key, []byte("old base"), "hash", "linux", "host-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journals")
	plan, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: Public, App: "app", Name: "settings"}, Destination: Private}, &platform.Platform{OS: platform.OSLinux})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ApplyMove(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{target: "live edits", recovery: "external backup"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v", path, got, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(privateDir, "app", "settings.tmpl")); err != nil || string(got) != "{{ .OS }}" {
		t.Fatalf("source = %q, %v", got, err)
	}
	dest, err := state.OpenReadOnly(context.Background(), filepath.Join(privateDir, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer dest.Close()
	records, err := dest.ExportRenders(context.Background(), []string{key})
	if err != nil || len(records) != 1 {
		t.Fatalf("copy render history = %d, %v", len(records), err)
	}
}

func TestMoveSetupDefinitionDoesNotRunCommands(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "setup", Entries: []config.SubEntry{{Name: "marker", Check: map[string]string{"linux": "false"}, Run: map[string]string{"linux": "touch /tmp/should-not-run"}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journals")
	plan, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: Public, App: "setup", Name: "marker"}, Destination: Private}, &platform.Platform{OS: platform.OSLinux})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ApplyMove(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("/tmp/should-not-run"); !os.IsNotExist(err) {
		t.Fatalf("setup command ran during move: %v", err)
	}
	if len(w.Repositories[1].Config.Applications) != 1 {
		t.Fatal("setup entry was not moved")
	}
}

func TestMoveCopiesAttributedCopyTemplateConflictWithoutAlias(t *testing.T) {
	publicDir, privateDir, targetDir := t.TempDir(), t.TempDir(), t.TempDir()
	sourceRoot := filepath.Join(publicDir, "app")
	if err := os.MkdirAll(sourceRoot, 0700); err != nil {
		t.Fatal(err)
	}
	templatePath := filepath.Join(sourceRoot, "f.tmpl")
	conflictPath := templatePath + ".conflict"
	for path, contents := range map[string]string{templatePath: "{{ .OS }}", conflictPath: "user recovery marker"} {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	entry := config.SubEntry{Name: "template", Backup: "./app", Files: []string{"f.tmpl"}, Method: config.MethodCopy, Targets: map[string]string{"linux": targetDir}}
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{entry}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3})
	copyKey := fmt.Sprintf("copy:%q:%q", "./app/f.tmpl", "/home/user/config/f")
	store := openStateStore(t, filepath.Join(publicDir, ".tidydots.db"))
	if err := store.SaveRender(context.Background(), copyKey, []byte("render history"), "hash", "linux", "history-host"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journals")
	plan, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: Public, App: "app", Name: "template"}, Destination: Private}, &platform.Platform{OS: platform.OSLinux})
	if err != nil {
		t.Fatal(err)
	}
	if !snapshotContainsPath(plan.data.sourceFiles, sourceRoot, conflictPath) {
		t.Fatalf("attributed conflict artifact missing from planned source snapshot: %#v", plan.data.sourceFiles)
	}
	if err := w.ApplyMove(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	movedConflict := filepath.Join(privateDir, "app", "f.tmpl.conflict")
	if got, err := os.ReadFile(movedConflict); err != nil || string(got) != "user recovery marker" {
		t.Fatalf("moved conflict = %q, %v", got, err)
	}
	if _, err := os.Lstat(conflictPath); !os.IsNotExist(err) {
		t.Fatalf("source conflict artifact remains after committed move: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(privateDir, "app", "f")); !os.IsNotExist(err) {
		t.Fatalf("copy template move fabricated a repository alias: %v", err)
	}
	destination, err := state.OpenReadOnly(context.Background(), filepath.Join(privateDir, ".tidydots.db"))
	if err != nil {
		t.Fatal(err)
	}
	if destination == nil {
		t.Fatal("attributed copy history was not imported")
	}
	defer destination.Close()
	if records, err := destination.ExportRenders(context.Background(), []string{copyKey}); err != nil || len(records) != 1 {
		t.Fatalf("imported copy history = %d records, %v", len(records), err)
	}
}

func TestMoveNestedSymlinkTemplateArtifactsBothDirections(t *testing.T) {
	for _, direction := range []struct {
		name       string
		sourceRole Role
		destRole   Role
	}{
		{name: "public-to-private", sourceRole: Public, destRole: Private},
		{name: "private-to-public", sourceRole: Private, destRole: Public},
	} {
		t.Run(direction.name, func(t *testing.T) {
			publicDir, privateDir, liveDir := t.TempDir(), t.TempDir(), t.TempDir()
			sourceRoot, destinationRoot := publicDir, privateDir
			if direction.sourceRole == Private {
				sourceRoot, destinationRoot = privateDir, publicDir
			}
			backup := filepath.Join(sourceRoot, "app", "nested")
			if err := os.MkdirAll(backup, 0700); err != nil {
				t.Fatal(err)
			}
			for path, contents := range map[string]string{
				filepath.Join(backup, "f.tmpl"):          "{{ .User }}",
				filepath.Join(backup, "f.tmpl.rendered"): "edited rendered content",
				filepath.Join(backup, "f.tmpl.conflict"): "saved conflict recovery",
			} {
				if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			alias := filepath.Join(backup, "f")
			if err := os.Symlink("f.tmpl.rendered", alias); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			targetDir := filepath.Join(liveDir, "nested")
			if err := os.MkdirAll(targetDir, 0700); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(targetDir, "f")
			if err := os.Symlink(alias, target); err != nil {
				t.Fatal(err)
			}
			entry := config.SubEntry{Name: "template", Backup: "./app", Files: []string{"nested/f.tmpl"}, Targets: map[string]string{"linux": liveDir}}
			sourceConfig := &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{entry}}}}
			if direction.sourceRole == Public {
				saveRepository(t, publicDir, sourceConfig)
				saveRepository(t, privateDir, &config.Config{Version: 3})
			} else {
				saveRepository(t, publicDir, &config.Config{Version: 3})
				saveRepository(t, privateDir, sourceConfig)
			}
			w, err := Load(publicDir, privateDir)
			if err != nil {
				t.Fatal(err)
			}
			w.JournalDir = filepath.Join(t.TempDir(), "journals")
			plan, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: direction.sourceRole, App: "app", Name: "template"}, Destination: direction.destRole}, &platform.Platform{OS: platform.OSLinux})
			if err != nil {
				t.Fatal(err)
			}
			for _, rel := range []string{"nested/f.tmpl", "nested/f", "nested/f.tmpl.rendered", "nested/f.tmpl.conflict"} {
				if !snapshotContainsPath(plan.data.sourceFiles, filepath.Join(sourceRoot, "app"), filepath.Join(sourceRoot, "app", rel)) {
					t.Errorf("plan omitted nested artifact %q: %#v", rel, plan.data.sourceFiles)
				}
			}
			if err := w.ApplyMove(context.Background(), plan, false); err != nil {
				t.Fatal(err)
			}
			for rel, want := range map[string]string{
				"nested/f.tmpl":          "{{ .User }}",
				"nested/f.tmpl.rendered": "edited rendered content",
				"nested/f.tmpl.conflict": "saved conflict recovery",
			} {
				got, err := os.ReadFile(filepath.Join(destinationRoot, "app", rel))
				if err != nil || string(got) != want {
					t.Errorf("destination %s = %q, %v; want %q", rel, got, err, want)
				}
			}
			if _, err := os.Lstat(alias); !os.IsNotExist(err) {
				t.Errorf("source alias remains after move: %v", err)
			}
			link, err := os.Readlink(target)
			if err != nil {
				t.Fatal(err)
			}
			resolved := link
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(target), resolved)
			}
			if filepath.Clean(resolved) != filepath.Join(destinationRoot, "app", "nested", "f") {
				t.Fatalf("live deployment points to %q, want destination alias", resolved)
			}
			got, err := os.ReadFile(target)
			if err != nil || string(got) != "edited rendered content" {
				t.Fatalf("live target reads %q, %v", got, err)
			}
		})
	}
}

func TestMoveRejectsUnattributedGeneratedArtifact(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	backup := filepath.Join(publicDir, "app")
	if err := os.MkdirAll(backup, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "f.tmpl"), []byte("{{ .OS }}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backup, "f.tmpl.conflict"), []byte("possibly private recovery"), 0600); err != nil {
		t.Fatal(err)
	}
	entry := config.SubEntry{Name: "template", Backup: "./app", Files: []string{"f.tmpl"}, Method: config.MethodCopy, Targets: map[string]string{"linux": t.TempDir()}}
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{entry}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.PlanMove(context.Background(), MoveRequest{Ref: EntryRef{Repository: Public, App: "app", Name: "template"}, Destination: Private}, &platform.Platform{OS: platform.OSLinux}); err == nil {
		t.Fatal("move planned while an existing generated artifact had no attribution proof")
	}
	if got, err := os.ReadFile(filepath.Join(backup, "f.tmpl.conflict")); err != nil || string(got) != "possibly private recovery" {
		t.Fatalf("preflight changed unattributed artifact: %q, %v", got, err)
	}
}
