package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/packages"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

func TestResolveCLIWorkspaceHonorsRegistrationAndOverrides(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	publicDir, registeredPrivate, explicitPrivate := makeValidRepo(t), makeValidRepo(t), makeValidRepo(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicDir, PrivateConfigDir: registeredPrivate}); err != nil {
		t.Fatal(err)
	}

	t.Run("registered pair", func(t *testing.T) {
		cmd := newRootCommand()
		configDir, privateDir, publicOnly = "", "", false
		w, _, routed, err := resolveCLIWorkspace(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if !routed || len(w.Repositories) != 2 || w.Repositories[1].Role != workspace.Private {
			t.Fatalf("workspace route = %t, repositories = %+v", routed, w)
		}
	})

	t.Run("dir isolates registered private", func(t *testing.T) {
		cmd := newRootCommand()
		configDir, privateDir, publicOnly = publicDir, "", false
		w, _, routed, err := resolveCLIWorkspace(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if routed || w != nil {
			t.Fatalf("--dir without pairing should retain normal single-repo path: routed=%t workspace=%+v", routed, w)
		}
	})

	t.Run("explicit private dir pairs with dir", func(t *testing.T) {
		cmd := newRootCommand()
		configDir, privateDir, publicOnly = publicDir, explicitPrivate, false
		w, _, routed, err := resolveCLIWorkspace(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if !routed || len(w.Repositories) != 2 || w.Repositories[1].Root != explicitPrivate {
			t.Fatalf("explicit workspace = routed %t, repositories %+v", routed, w)
		}
	})

	t.Run("public-only ignores missing registered private", func(t *testing.T) {
		if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicDir, PrivateConfigDir: t.TempDir() + "/missing"}); err != nil {
			t.Fatal(err)
		}
		cmd := newRootCommand()
		configDir, privateDir, publicOnly = "", "", true
		w, _, routed, err := resolveCLIWorkspace(cmd)
		if err != nil {
			t.Fatal(err)
		}
		if !routed || len(w.Repositories) != 1 {
			t.Fatalf("public-only workspace = routed %t, repositories %+v", routed, w)
		}
	})

	t.Run("enabled missing private gives recovery guidance", func(t *testing.T) {
		if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicDir, PrivateConfigDir: t.TempDir() + "/missing"}); err != nil {
			t.Fatal(err)
		}
		cmd := newRootCommand()
		configDir, privateDir, publicOnly = "", "", false
		_, _, _, err := resolveCLIWorkspace(cmd)
		if err == nil || !strings.Contains(err.Error(), "--public-only") {
			t.Fatalf("error = %v, want public-only recovery guidance", err)
		}
	})
}

func TestPrivateSelectionFlagsRouteListAndPublicOnlyIgnoresRegistration(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	publicRoot, privateRoot := makeValidRepo(t), makeValidRepo(t)
	cmd := newRootCommand()
	cmd.SetArgs([]string{"--dir", publicRoot, "--private-dir", privateRoot, "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("explicit private-dir list should route through workspace: %v", err)
	}

	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicRoot, PrivateConfigDir: filepath.Join(t.TempDir(), "missing-private")}); err != nil {
		t.Fatal(err)
	}
	resetCLIFlags(t)
	cmd = newRootCommand()
	cmd.SetArgs([]string{"--public-only", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("public-only list should ignore invalid private registration: %v", err)
	}
}

func TestPendingTransactionBlocksMutationBeforeWorkspaceLoadAndCannotBeBypassed(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	publicRoot, privateRoot, alternateRoot := makeValidRepo(t), makeValidRepo(t), makeValidRepo(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicRoot, PrivateConfigDir: privateRoot}); err != nil {
		t.Fatal(err)
	}
	writePendingCLIJournal(t, "pending-owner", publicRoot, privateRoot)
	if err := os.WriteFile(filepath.Join(privateRoot, "tidydots.yaml"), []byte("not: [valid"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"--public-only", "restore"},
		{"--dir", alternateRoot, "restore"},
	} {
		resetCLIFlags(t)
		cmd := newRootCommand()
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "pending-owner") || !strings.Contains(err.Error(), "recover") {
			t.Fatalf("args %v error = %v, want pending transaction recovery guidance", args, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(publicRoot, ".tidydots.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("writable state opened before pending transaction block: %v", err)
	}
}

func TestPendingTransactionAllowsValidWorkspaceInspectionAndDryRun(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	publicRoot, privateRoot := makeValidRepo(t), makeValidRepo(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicRoot, PrivateConfigDir: privateRoot}); err != nil {
		t.Fatal(err)
	}
	writePendingCLIJournal(t, "pending-readable", publicRoot, privateRoot)

	commands := [][]string{
		{"status", "--json"},
		{"list"},
		{"list-packages"},
		{"restore", "-n"},
	}
	for _, args := range commands {
		resetCLIFlags(t)
		cmd := newRootCommand()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Errorf("inspection command %v blocked by recoverable journal: %v", args, err)
		}
	}
}

func TestExplicitDirPendingInspectionWorksWithoutUsableAppRegistration(t *testing.T) {
	for _, metadataState := range []string{"absent", "malformed"} {
		t.Run(metadataState, func(t *testing.T) {
			useIsolatedHome(t)
			resetCLIFlags(t)
			publicRoot, otherRoot := makeValidRepo(t), makeValidRepo(t)
			writePendingCLIJournal(t, "pending-explicit-dir", publicRoot, otherRoot)
			if metadataState == "malformed" {
				if err := os.MkdirAll(filepath.Dir(config.AppConfigPath()), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(config.AppConfigPath(), []byte("config_dir: [not valid\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}

			for _, args := range [][]string{
				{"--dir", publicRoot, "list"},
				{"--dir", publicRoot, "status", "--json"},
				{"--dir", publicRoot, "restore", "-n"},
			} {
				resetCLIFlags(t)
				cmd := newRootCommand()
				cmd.SetArgs(args)
				if err := cmd.Execute(); err != nil {
					t.Errorf("read-only explicit-dir command %v with %s metadata failed: %v", args, metadataState, err)
				}
			}

			resetCLIFlags(t)
			cmd := newRootCommand()
			cmd.SetArgs([]string{"--dir", publicRoot, "restore"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "pending-explicit-dir") {
				t.Errorf("applied restore error = %v, want pending journal block", err)
			}
			if metadataState == "malformed" {
				_, metadataErr := config.LoadAppConfigMetadata()
				resetCLIFlags(t)
				cmd = newRootCommand()
				cmd.SetArgs([]string{"list"})
				err = cmd.Execute()
				if err == nil || metadataErr == nil || !strings.Contains(err.Error(), metadataErr.Error()) {
					t.Errorf("list without --dir error = %v, want app-config parse error %v when registration is required", err, metadataErr)
				}
			}
		})
	}
}

func TestPublicOnlyAndDirIgnoreUnreadableRegisteredPrivateButKeepPendingProtection(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	publicRoot, alternateRoot := makeValidRepo(t), makeValidRepo(t)
	privateFile := filepath.Join(t.TempDir(), "private-is-not-a-directory")
	if err := os.WriteFile(privateFile, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicRoot, PrivateConfigDir: privateFile}); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"--public-only", "list"}, {"--dir", alternateRoot, "list"}} {
		resetCLIFlags(t)
		cmd := newRootCommand()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Errorf("excluded non-directory private root blocked %v: %v", args, err)
		}
	}

	writePendingCLIJournal(t, "pending-offline-private", publicRoot, privateFile)
	for _, args := range [][]string{{"--public-only", "restore"}, {"--dir", alternateRoot, "restore"}} {
		resetCLIFlags(t)
		cmd := newRootCommand()
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "pending-offline-private") {
			t.Errorf("writer %v error = %v, want pending journal identity block", args, err)
		}
	}
}

func TestPublicOnlyIgnoresUnreadableRegisteredPrivatePath(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	publicRoot, alternateRoot := makeValidRepo(t), makeValidRepo(t)
	privateParent := t.TempDir()
	privateRoot := filepath.Join(privateParent, "private")
	if err := os.Mkdir(privateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	writeConfigFixture(t, privateRoot)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicRoot, PrivateConfigDir: privateRoot}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(privateParent, 0700); err != nil {
			t.Errorf("restore private parent permissions: %v", err)
		}
	})
	if err := os.Chmod(privateParent, 0000); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(privateRoot); err == nil {
		_ = os.Chmod(privateParent, 0700)
		t.Skip("current user can still access mode-000 parent")
	}

	for _, args := range [][]string{{"--public-only", "list"}, {"--dir", alternateRoot, "list"}} {
		resetCLIFlags(t)
		cmd := newRootCommand()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Errorf("excluded unreadable private root blocked %v: %v", args, err)
		}
	}
}

func TestRecoverDryRunWorksWithInvalidWorkspaceConfig(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	publicRoot, privateRoot := t.TempDir(), makeValidRepo(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicRoot, PrivateConfigDir: privateRoot}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(publicRoot, "tidydots.yaml"), []byte("invalid: ["), 0600); err != nil {
		t.Fatal(err)
	}
	writePendingCLIJournal(t, "recover-invalid-config", publicRoot, privateRoot)
	journalPath := cliJournalPath(t, "recover-invalid-config")
	before, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	cmd.SetArgs([]string{"recover", "recover-invalid-config", "-n"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("recover dry-run must not load repository YAML or registration pair: %v", err)
	}
	after, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("recovery dry-run changed the pending journal")
	}
}

func TestMutationGuardBlocksOverlappingRootLock(t *testing.T) {
	useIsolatedHome(t)
	root := t.TempDir()
	release, err := workspace.AcquireMutationLocks(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := workspace.MutationGuard(context.Background(), "", root); err == nil || !strings.Contains(err.Error(), "being modified") {
		t.Fatalf("mutation guard error = %v, want root lock conflict", err)
	}
}

func TestWorkspaceRestoreAcquiresMutationGuardBeforeWritableState(t *testing.T) {
	useIsolatedHome(t)
	publicRoot, privateRoot := t.TempDir(), makeValidRepo(t)
	targetRoot := filepath.Join(t.TempDir(), "target")
	writeConfigFixture(t, publicRoot, config.Application{Name: "shell", Entries: []config.SubEntry{fileEntry("public", "public.conf", targetRoot)}})
	writeFile(t, filepath.Join(publicRoot, "shell", "public.conf"), "public\n")
	w, err := workspace.Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	release, err := workspace.AcquireMutationLocks(context.Background(), publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	err = runWorkspaceRestore(w, platform.Detect(), nil)
	if err == nil || !strings.Contains(err.Error(), "being modified") {
		t.Fatalf("restore error = %v, want held-root lock conflict", err)
	}
	if _, err := os.Lstat(filepath.Join(publicRoot, ".tidydots.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("writable state opened before mutation guard: %v", err)
	}
}

func cliJournalPath(t *testing.T, id string) string {
	t.Helper()
	return filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "tidydots", "transactions", id+".json")
}

func writePendingCLIJournal(t *testing.T, id string, roots ...string) {
	t.Helper()
	if len(roots) != 2 {
		t.Fatalf("journal fixture requires public and private roots, got %d", len(roots))
	}
	path := cliJournalPath(t, id)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	journal, err := json.Marshal(map[string]any{
		"version": 1, "id": id, "phase": "prepared",
		"public_root": roots[0], "private_root": roots[1],
		"public_config":  filepath.Join(roots[0], "tidydots.yaml"),
		"private_config": filepath.Join(roots[1], "tidydots.yaml"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, journal, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceRestoreDeploysEachEntryFromItsOwner(t *testing.T) {
	useIsolatedHome(t)
	publicRoot, privateRoot := t.TempDir(), t.TempDir()
	targetRoot := filepath.Join(t.TempDir(), "target")
	writeConfigFixture(t, publicRoot, config.Application{Name: "shell", Entries: []config.SubEntry{fileEntry("public", "public.conf", targetRoot)}})
	writeConfigFixture(t, privateRoot, config.Application{Name: "shell", Entries: []config.SubEntry{fileEntry("private", "private.conf", targetRoot)}})
	writeFile(t, filepath.Join(publicRoot, "shell", "public.conf"), "public value\n")
	writeFile(t, filepath.Join(privateRoot, "shell", "private.conf"), "private value\n")
	w, err := workspace.Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	plat := platform.Detect()
	if err := runWorkspaceRestore(w, plat, nil); err != nil {
		t.Fatalf("workspace restore: %v", err)
	}
	for name, want := range map[string]string{"public.conf": "public value\n", "private.conf": "private value\n"} {
		got, err := os.ReadFile(filepath.Join(targetRoot, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestWorkspaceBackupWritesSelectedPrivateEntryOnlyToPrivateRoot(t *testing.T) {
	useIsolatedHome(t)
	publicRoot, privateRoot := t.TempDir(), t.TempDir()
	targetRoot := t.TempDir()
	writeConfigFixture(t, publicRoot, config.Application{Name: "shell", Entries: []config.SubEntry{fileEntry("public", "public.conf", targetRoot)}})
	writeConfigFixture(t, privateRoot, config.Application{Name: "shell", Entries: []config.SubEntry{fileEntry("private", "private.conf", targetRoot)}})
	writeFile(t, filepath.Join(targetRoot, "private.conf"), "private-local-edit\n")
	w, err := workspace.Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := runWorkspaceBackup(w, platform.Detect(), []string{"shell", "private"}); err != nil {
		t.Fatalf("workspace backup: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(privateRoot, "shell", "private.conf"))
	if err != nil || string(got) != "private-local-edit\n" {
		t.Fatalf("wrong backup owner content %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(publicRoot, "shell", "private.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private backup leaked to public root: %v", err)
	}
}

func TestWorkspaceRestoreRejectsLateTargetCollisionBeforeEffects(t *testing.T) {
	publicRoot, privateRoot := t.TempDir(), t.TempDir()
	targetRoot := filepath.Join(t.TempDir(), "collision")
	writeConfigFixture(t, publicRoot, config.Application{Name: "shell", Entries: []config.SubEntry{fileEntry("public", "same.conf", targetRoot)}})
	writeConfigFixture(t, privateRoot, config.Application{Name: "shell", Entries: []config.SubEntry{fileEntry("private", "same.conf", targetRoot)}})
	writeFile(t, filepath.Join(publicRoot, "shell", "same.conf"), "public\n")
	writeFile(t, filepath.Join(privateRoot, "shell", "same.conf"), "private\n")
	w, err := workspace.Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := runWorkspaceRestore(w, platform.Detect(), nil); err == nil || !strings.Contains(err.Error(), "target conflict") {
		t.Fatalf("restore error = %v, want target conflict", err)
	}
	if _, err := os.Lstat(filepath.Join(targetRoot, "same.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target was changed before collision error: %v", err)
	}
	for _, root := range []string{publicRoot, privateRoot} {
		if _, err := os.Lstat(filepath.Join(root, ".tidydots.db")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("state was written before collision validation in %s: %v", root, err)
		}
	}
}

func TestSelectedPublicFolderBackupRejectsInactivePrivateNestedTarget(t *testing.T) {
	useIsolatedHome(t)
	publicRoot, privateRoot := t.TempDir(), t.TempDir()
	publicTarget := filepath.Join(t.TempDir(), "public-target")
	privateSource := filepath.Join(privateRoot, "private-source", "secret.conf")
	writeFile(t, privateSource, "private secret\n")
	if err := os.MkdirAll(publicTarget, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(privateSource, filepath.Join(publicTarget, "secret.conf")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeConfigFixture(t, publicRoot, config.Application{Name: "public-app", Entries: []config.SubEntry{{
		Name:    "public-folder",
		Backup:  "./public-backup",
		Targets: map[string]string{"linux": publicTarget, "windows": publicTarget},
	}}})
	writeConfigFixture(t, privateRoot, config.Application{
		Name:    "private-app",
		When:    `{{ eq .OS "windows" }}`,
		Entries: []config.SubEntry{fileEntry("private-secret", "secret.conf", publicTarget)},
	})
	w, err := workspace.Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	err = runWorkspaceBackup(w, platform.Detect().WithOS(platform.OSLinux), []string{"public-app"})
	if err == nil || !strings.Contains(err.Error(), "target conflict") {
		t.Fatalf("selected folder backup error = %v, want private-target conflict", err)
	}
	if _, err := os.Lstat(filepath.Join(publicRoot, "public-backup", "secret.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private symlink content was copied into public backup: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(publicRoot, ".tidydots.db")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("writable state initialized before conflict preflight: %v", err)
	}
}

func TestSelectedRestoreIgnoresUnrelatedCrossOwnerTargetConflict(t *testing.T) {
	useIsolatedHome(t)
	publicRoot, privateRoot := t.TempDir(), makeValidRepo(t)
	selectedTarget := filepath.Join(t.TempDir(), "selected-target")
	conflictTarget := filepath.Join(t.TempDir(), "unrelated-target")
	writeConfigFixture(t, publicRoot,
		config.Application{Name: "selected", Entries: []config.SubEntry{fileEntry("safe", "safe.conf", selectedTarget)}},
		config.Application{Name: "public-other", Entries: []config.SubEntry{fileEntry("public", "shared.conf", conflictTarget)}},
	)
	writeConfigFixture(t, privateRoot,
		config.Application{Name: "private-other", Entries: []config.SubEntry{fileEntry("private", "shared.conf", conflictTarget)}},
	)
	writeFile(t, filepath.Join(publicRoot, "shell", "safe.conf"), "safe\n")
	w, err := workspace.Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := runWorkspaceRestore(w, platform.Detect(), []string{"selected"}); err != nil {
		t.Fatalf("unrelated cross-owner conflict blocked selected restore: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(selectedTarget, "safe.conf"))
	if err != nil || string(got) != "safe\n" {
		t.Fatalf("selected target content = %q, %v", got, err)
	}
}

func TestWorkspaceSetupCheckRunsFromPrivateRepositoryRoot(t *testing.T) {
	useIsolatedHome(t)
	publicRoot, privateRoot := t.TempDir(), t.TempDir()
	check := "test \"$PWD\" = " + privateRoot
	writeConfigFixture(t,
		publicRoot,
		config.Application{Name: "setup-app", Entries: []config.SubEntry{{Name: "public", Run: map[string]string{"linux": "true"}, Check: map[string]string{"linux": "true"}}}},
	)
	writeConfigFixture(t,
		privateRoot,
		config.Application{Name: "setup-app", Entries: []config.SubEntry{{Name: "private-setup", Run: map[string]string{"linux": "false"}, Check: map[string]string{"linux": check}}}},
	)
	w, err := workspace.Load(publicRoot, privateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := runWorkspaceRestore(w, platform.Detect().WithOS(platform.OSLinux), []string{"setup-app", "private-setup"}); err != nil {
		t.Fatalf("private setup check failed from non-owner directory: %v", err)
	}
}

func TestWorkspacePackageConfigurationRetainsOwningRepositoryPriority(t *testing.T) {
	repo := &workspace.Repository{
		Role: workspace.Private,
		Root: t.TempDir(),
		Config: &config.Config{
			Version:         3,
			DefaultManager:  "apt",
			ManagerPriority: []string{"apt", "brew"},
			Applications: []config.Application{{
				Name: "editor",
				Package: &config.EntryPackage{Managers: map[string]config.ManagerValue{
					"apt": {PackageName: "editor-package"},
				}},
			}},
		},
	}
	pkgConfig, list := packageConfigForRepository(repo, platform.Detect())
	if pkgConfig.DefaultManager != "apt" || len(pkgConfig.ManagerPriority) != 2 || pkgConfig.ManagerPriority[0] != "apt" {
		t.Fatalf("package configuration lost owner priority/default: %+v", pkgConfig)
	}
	if len(list) != 1 || list[0].Name != "editor" || list[0].Managers[packages.Apt].PackageName != "editor-package" {
		t.Fatalf("package conversion lost private owner's app: %+v", list)
	}
}

func TestWorkspaceStatusDryRunIsOwnerTaggedAndDoesNotCreateState(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	publicRoot, privateRoot := t.TempDir(), t.TempDir()
	targetRoot := filepath.Join(t.TempDir(), "target")
	writeConfigFixture(t, publicRoot, config.Application{Name: "shell", Entries: []config.SubEntry{fileEntry("public", "public.conf", targetRoot)}})
	writeConfigFixture(t, privateRoot, config.Application{Name: "shell", Entries: []config.SubEntry{fileEntry("private", "private.conf", targetRoot)}})
	writeFile(t, filepath.Join(publicRoot, "shell", "public.conf"), "public\n")
	writeFile(t, filepath.Join(privateRoot, "shell", "private.conf"), "private\n")
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicRoot, PrivateConfigDir: privateRoot}); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	cmd := newRootCommand()
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"--dry-run", "status", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("workspace status dry-run: %v", err)
	}
	for _, owner := range []string{`"repository": "public"`, `"repository": "private"`} {
		if !strings.Contains(output.String(), owner) {
			t.Errorf("status JSON missing owner %q:\n%s", owner, output.String())
		}
	}
	for _, root := range []string{publicRoot, privateRoot} {
		if _, err := os.Lstat(filepath.Join(root, ".tidydots.db")); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("status created state database in %s: %v", root, err)
		}
	}
}

func fileEntry(name, file, targetRoot string) config.SubEntry {
	return config.SubEntry{
		Name:    name,
		Backup:  "./shell",
		Files:   []string{file},
		Targets: map[string]string{"linux": targetRoot, "windows": targetRoot},
	}
}

func writeConfigFixture(t *testing.T, root string, apps ...config.Application) {
	t.Helper()
	if err := config.Save(&config.Config{Version: 3, Applications: apps}, filepath.Join(root, "tidydots.yaml")); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
