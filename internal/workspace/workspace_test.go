package workspace

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

func saveRepository(t *testing.T, dir string, cfg *config.Config) {
	t.Helper()
	if err := config.Save(cfg, filepath.Join(dir, "tidydots.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationsRetainOwners(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	for _, item := range []struct{ dir, entry string }{{publicDir, "defaults"}, {privateDir, "hosts"}} {
		cfg := &config.Config{Version: 3, Applications: []config.Application{{Name: "ssh", Entries: []config.SubEntry{{Name: item.entry, Backup: "./ssh", Targets: map[string]string{"linux": "/unused"}, Files: []string{item.entry}}}}}}
		saveRepository(t, item.dir, cfg)
	}
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	apps, err := w.Applications()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || len(apps[0].Entries) != 2 {
		t.Fatalf("unexpected merged apps: %#v", apps)
	}
	if apps[0].Entries[0].Ref.Repository != Public || apps[0].Entries[1].Ref.Repository != Private {
		t.Fatal("entry ownership lost")
	}
	if len(w.Repositories) != 2 || len(w.Repositories[0].Config.Applications[0].Entries) != 1 || len(w.Repositories[1].Config.Applications[0].Entries) != 1 {
		t.Fatal("workspace merge mutated source configs")
	}
}

func TestWorkspaceValidateFreshRejectsStaleOrUnfingerprintedRepositories(t *testing.T) {
	tests := []struct {
		name      string
		staleRole Role
		noDigest  bool
	}{
		{name: "fresh"},
		{name: "stale public", staleRole: Public},
		{name: "stale private", staleRole: Private},
		{name: "missing public fingerprint", staleRole: Public, noDigest: true},
		{name: "missing private fingerprint", staleRole: Private, noDigest: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publicDir, privateDir := t.TempDir(), t.TempDir()
			saveRepository(t, publicDir, &config.Config{Version: 3})
			saveRepository(t, privateDir, &config.Config{Version: 3})
			w, err := Load(publicDir, privateDir)
			if err != nil {
				t.Fatal(err)
			}
			if test.staleRole != "" && !test.noDigest {
				root := publicDir
				if test.staleRole == Private {
					root = privateDir
				}
				saveRepository(t, root, &config.Config{Version: 3, Applications: []config.Application{{Name: "changed externally"}}})
			}
			if test.noDigest {
				repo, err := w.Repository(test.staleRole)
				if err != nil {
					t.Fatal(err)
				}
				repo.ConfigDigest = ""
			}
			err = w.ValidateFresh()
			if test.name == "fresh" {
				if err != nil {
					t.Fatalf("fresh workspace rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateFresh accepted %s repository", test.name)
			}
		})
	}
}

func TestApplicationsMergeOrderAndPackageOwner(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	pubPkg := &config.EntryPackage{Managers: map[string]config.ManagerValue{"apt": {PackageName: "openssh-client"}}}
	privOnly := &config.EntryPackage{Managers: map[string]config.ManagerValue{"apt": {PackageName: "private-tool"}}}
	saveRepository(t, publicDir, &config.Config{Version: 3, DefaultManager: "apt", ManagerPriority: []string{"apt", "brew"}, Applications: []config.Application{
		{Name: "shared", Description: "public description", When: `{{ eq .OS "linux" }}`, Package: pubPkg, Entries: []config.SubEntry{{Name: "public-entry"}}},
		{Name: "public-only"},
	}})
	saveRepository(t, privateDir, &config.Config{Version: 3, DefaultManager: "brew", ManagerPriority: []string{"brew"}, Applications: []config.Application{
		{Name: "shared", Description: "private description", Entries: []config.SubEntry{{Name: "private-entry", When: `{{ eq .Distro "arch" }}`}}},
		{Name: "private-only", Package: privOnly},
	}})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if w.Repositories[0].Config.DefaultManager != "apt" || w.Repositories[1].Config.DefaultManager != "brew" {
		t.Fatalf("repository-local manager defaults were merged: %#v", w.Repositories)
	}
	apps, err := w.Applications()
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{apps[0].Name, apps[1].Name, apps[2].Name}; strings.Join(got, ",") != "shared,public-only,private-only" {
		t.Fatalf("application order = %v", got)
	}
	shared := apps[0]
	if shared.Description != "public description" || shared.Entries[0].AppWhen == "" || shared.Entries[1].AppWhen != "" {
		t.Fatalf("contributions did not retain source metadata: %#v", shared)
	}
	if shared.PackageOwner != Public || shared.Package == nil {
		t.Fatalf("package owner = %q, want public", shared.PackageOwner)
	}
	if packageName, ok := shared.Package.GetManagerString("apt"); !ok || packageName != "openssh-client" {
		t.Fatalf("merged package = %#v, want public package", shared.Package)
	}
	if apps[2].PackageOwner != Private || apps[2].Package == nil {
		t.Fatalf("private package owner not retained: %#v", apps[2])
	}
}

func TestLoadRejectsDuplicateEntryNamesIncludingInactive(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", When: `{{ eq .OS "windows" }}`, Entries: []config.SubEntry{{Name: "same"}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "same", When: `{{ eq .OS "never" }}`}}}}})
	_, err := Load(publicDir, privateDir)
	if err == nil || !strings.Contains(err.Error(), "same") || !strings.Contains(err.Error(), "public") || !strings.Contains(err.Error(), "private") {
		t.Fatalf("Load error = %v, want duplicate with both owners", err)
	}
}

func TestLoadRejectsDuplicatePackagesEvenWhenConditionsDiffer(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	packageDef := &config.EntryPackage{Managers: map[string]config.ManagerValue{"apt": {PackageName: "pkg"}}}
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", When: `{{ eq .OS "linux" }}`, Package: packageDef}}})
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", When: `{{ eq .OS "windows" }}`, Package: packageDef}}})
	_, err := Load(publicDir, privateDir)
	if err == nil || !strings.Contains(err.Error(), "package") {
		t.Fatalf("Load error = %v, want duplicate package error", err)
	}
}

func TestLoadRejectsInvalidRoots(t *testing.T) {
	root := t.TempDir()
	publicDir := filepath.Join(root, "public")
	privateDir := filepath.Join(publicDir, "private")
	if err := os.MkdirAll(privateDir, 0750); err != nil {
		t.Fatal(err)
	}
	saveRepository(t, publicDir, &config.Config{Version: 3})
	saveRepository(t, privateDir, &config.Config{Version: 3})
	if err := os.Symlink(filepath.Join(root, "public"), filepath.Join(root, "public-link")); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, private string
	}{
		{name: "same root", private: publicDir},
		{name: "nested", private: privateDir},
		{name: "symlink alias", private: filepath.Join(root, "public-link")},
		{name: "missing", private: filepath.Join(root, "missing")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Load(publicDir, test.private); err == nil {
				t.Fatal("Load() succeeded for invalid repository roots")
			}
		})
	}
}

func TestLoadRejectsWorktreesSharingGitObjectStore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	main := filepath.Join(root, "main")
	if err := os.MkdirAll(main, 0700); err != nil {
		t.Fatal(err)
	}
	runGit := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	runGit(main, "init")
	saveRepository(t, main, &config.Config{Version: 3})
	runGit(main, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "add", "tidydots.yaml")
	runGit(main, "-c", "user.name=test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial")
	private := filepath.Join(root, "private-worktree")
	runGit(main, "worktree", "add", "--detach", private, "HEAD")
	if _, err := Load(main, private); err == nil {
		t.Fatal("Load accepted public/private roots sharing a Git object store")
	}
}

func TestLoadRejectsMissingEnabledPrivateRepository(t *testing.T) {
	publicDir := t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3})
	if _, err := Load(publicDir, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("Load() succeeded with an enabled private repository missing")
	}
}

type mapRenderer map[string]string

func (r mapRenderer) RenderString(_ string, expression string) (string, error) {
	if expression == "{{public}}" {
		return r["public"], nil
	}
	if expression == "{{private}}" {
		return r["private"], nil
	}
	if expression == "{{entry}}" {
		return r["entry"], nil
	}
	return expression, nil
}

func TestSelectEntriesKeepsOwnerAndConditions(t *testing.T) {
	app := Application{Entries: []Entry{
		{Ref: EntryRef{Repository: Public, App: "app", Name: "public"}, AppWhen: "{{public}}", Value: config.SubEntry{When: "{{entry}}"}},
		{Ref: EntryRef{Repository: Private, App: "app", Name: "private"}, AppWhen: "{{private}}"},
	}}
	selected := SelectEntries(app, "", mapRenderer{"public": "true", "entry": "true", "private": "false"})
	if len(selected) != 1 || selected[0].Ref.Repository != Public || selected[0].Ref.Name != "public" {
		t.Fatalf("selected = %#v, want public contribution only", selected)
	}
	selected = SelectEntries(app, "private", mapRenderer{"private": "true"})
	if len(selected) != 1 || selected[0].Ref.Repository != Private {
		t.Fatalf("named selection = %#v, want private owner", selected)
	}
}

func TestLoadPublicOnlyDoesNotRequirePrivateRepository(t *testing.T) {
	publicDir := t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3})
	w, err := Load(publicDir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Repositories) != 1 || w.Repositories[0].Role != Public {
		t.Fatalf("repositories = %#v, want public only", w.Repositories)
	}
}

func TestOwnerAwareCRUDWritesOnlySelectedScope(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "editor", Description: "portable", When: `{{ eq .OS "linux" }}`, Entries: []config.SubEntry{{Name: "defaults"}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	entry := config.SubEntry{Name: "secret", Backup: "./secret", Files: []string{"secret.conf"}, Targets: map[string]string{"linux": filepath.Join(t.TempDir(), "secret.conf")}}
	beforePrivate, err := os.ReadFile(privateDir + string(os.PathSeparator) + "tidydots.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.AddEntry(context.Background(), Private, "editor", entry, true); err != nil {
		t.Fatal(err)
	}
	afterDryRun, err := os.ReadFile(filepath.Join(privateDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforePrivate, afterDryRun) {
		t.Fatal("dry-run changed private manifest")
	}
	if len(w.Repositories[1].Config.Applications) != 0 {
		t.Fatal("dry-run mutated loaded workspace")
	}
	if err := w.AddEntry(context.Background(), Private, "editor", entry, false); err != nil {
		t.Fatal(err)
	}
	pub, priv := w.Repositories[0].Config, w.Repositories[1].Config
	if len(pub.Applications[0].Entries) != 1 || pub.Applications[0].Entries[0].Name != "defaults" {
		t.Fatalf("public scope changed: %#v", pub.Applications)
	}
	if len(priv.Applications) != 1 || priv.Applications[0].When != `{{ eq .OS "linux" }}` || priv.Applications[0].Description != "" || len(priv.Applications[0].Entries) != 1 {
		t.Fatalf("private scoped app = %#v", priv.Applications)
	}
	updated := entry
	updated.When = `{{ eq .Distro "arch" }}`
	if err := w.SaveEntry(context.Background(), EntryRef{Repository: Private, App: "editor", Name: "secret"}, updated, false); err != nil {
		t.Fatal(err)
	}
	if w.Repositories[1].Config.Applications[0].Entries[0].When != updated.When {
		t.Fatal("private entry edit was not reloaded")
	}
	loaded, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Repositories[0].Config.Applications[0].Entries) != 1 {
		t.Fatal("owner-aware edit modified public entries")
	}
}

func TestOwnerAwareCRUDRejectsStaleConfigAndRenamesBothScopes(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Description: "public"}}})
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Description: "private"}}})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(privateDir, "tidydots.yaml")
	if err := os.WriteFile(path, []byte("version: 3\napplications: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	description := "changed"
	if err := w.EditApplication(context.Background(), Private, "app", &description, nil, nil, false); err == nil {
		t.Fatal("stale scoped save overwrote an external edit")
	}
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Description: "private"}}})
	w, err = Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.RenameApplication(context.Background(), "app", "renamed", false); err != nil {
		t.Fatal(err)
	}
	if w.Repositories[0].Config.Applications[0].Name != "renamed" || w.Repositories[1].Config.Applications[0].Name != "renamed" {
		t.Fatal("rename did not update both owning documents")
	}
	if err := w.DeleteApplication(context.Background(), "renamed", false); err != nil {
		t.Fatal(err)
	}
	if len(w.Repositories[0].Config.Applications) != 0 || len(w.Repositories[1].Config.Applications) != 0 {
		t.Fatal("merged delete left a contribution")
	}
}

func TestLookupUsesStableEntryReference(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "entry", Backup: "./public", Targets: map[string]string{"linux": "/tmp/public"}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "entry-private", Backup: "./private", Targets: map[string]string{"linux": "/tmp/private"}}}}}})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	repo, app, entry, err := w.Lookup(EntryRef{Repository: Private, App: "app", Name: "entry-private"})
	if err != nil {
		t.Fatal(err)
	}
	if repo.Role != Private || app.Name != "app" || entry.Backup != "./private" {
		t.Fatalf("lookup returned repo=%#v app=%#v entry=%#v", repo, app, entry)
	}
}

func TestValidateTargetsAllowsDistinctFilesInSameDirectory(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	for _, item := range []struct{ dir, name, file string }{{publicDir, "public-entry", "public.conf"}, {privateDir, "private-entry", "private.conf"}} {
		saveRepository(t, item.dir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: item.name, Backup: "./source", Files: []string{item.file}, Targets: map[string]string{"linux": "/tmp/shared"}}}}}})
	}
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateTargets(&platform.Platform{OS: platform.OSLinux}); err != nil {
		t.Fatalf("ValidateTargets() rejected disjoint files: %v", err)
	}
}

func TestValidateTargetsRejectsFileAndFolderOverlap(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "folder", Backup: "./source", Targets: map[string]string{"linux": "/tmp/shared"}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "file", Backup: "./source", Files: []string{"nested/file.conf"}, Targets: map[string]string{"linux": "/tmp/shared"}}}}}})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateTargets(&platform.Platform{OS: platform.OSLinux}); err == nil || !strings.Contains(err.Error(), "target") {
		t.Fatalf("ValidateTargets() = %v, want target conflict", err)
	}
}

func TestValidateTargetsRejectsTemplateAliasCollision(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "template", Backup: "./source", Files: []string{"settings.conf.tmpl"}, Targets: map[string]string{"linux": "/tmp/shared"}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "literal", Backup: "./source", Files: []string{"settings.conf"}, Targets: map[string]string{"linux": "/tmp/shared"}}}}}})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateTargets(&platform.Platform{OS: platform.OSLinux}); err == nil {
		t.Fatal("ValidateTargets() missed collision after .tmpl suffix stripping")
	}
}

func TestValidateTargetsResolvesSymlinkAncestors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory symlink creation requires platform privileges on Windows")
	}
	base := t.TempDir()
	realTarget := filepath.Join(base, "real")
	aliasTarget := filepath.Join(base, "alias")
	if err := os.Mkdir(realTarget, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realTarget, aliasTarget); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "public", Backup: "./source", Targets: map[string]string{"linux": filepath.Join(realTarget, "config")}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "private", Backup: "./source", Targets: map[string]string{"linux": filepath.Join(aliasTarget, "config")}}}}}})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateTargets(&platform.Platform{OS: platform.OSLinux}); err == nil {
		t.Fatal("ValidateTargets() missed symlink-ancestor collision")
	}
}

func TestValidateTargetsPreservesFolderSymlinkEntryIdentity(t *testing.T) {
	base := t.TempDir()
	live := filepath.Join(base, "live")
	source := filepath.Join(base, "source")
	if err := os.MkdirAll(live, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(source, 0750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(live, "config")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "folder", Backup: "./source", Targets: map[string]string{"linux": link}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "selected", Backup: "./source", Files: []string{"config"}, Targets: map[string]string{"linux": live}}}}}})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateTargets(&platform.Platform{OS: platform.OSLinux}); err == nil {
		t.Fatal("ValidateTargets() missed a selected file replacing the folder symlink itself")
	}
}

func TestValidateTargetsFolderSymlinkConflictsWithSelectedChildThroughAlias(t *testing.T) {
	base := t.TempDir()
	live := filepath.Join(base, "live")
	source := filepath.Join(base, "source")
	if err := os.MkdirAll(live, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(source, 0750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(live, "config")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "folder", Backup: "./source", Targets: map[string]string{"linux": link}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "child", Backup: "./source", Files: []string{"config/child"}, Targets: map[string]string{"linux": live}}}}}})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateTargets(&platform.Platform{OS: platform.OSLinux}); err == nil {
		t.Fatal("ValidateTargets() missed a selected child accessed through the deployed folder link")
	}
}

func TestValidateTargetsDoesNotTreatFinalFolderLinkAsItsReferent(t *testing.T) {
	base := t.TempDir()
	live := filepath.Join(base, "live")
	source := filepath.Join(base, "source")
	if err := os.MkdirAll(live, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(source, 0750); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(live, "config")
	if err := os.Symlink(source, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "folder-link", Backup: "./source", Targets: map[string]string{"linux": link}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "source-target", Backup: "./source", Targets: map[string]string{"linux": source}}}}}})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateTargets(&platform.Platform{OS: platform.OSLinux}); err != nil {
		t.Fatalf("distinct folder link and referent destinations conflicted: %v", err)
	}
}

func TestValidateTargetsUsesLiteralManagerSelectedFilenames(t *testing.T) {
	const target = "/tmp/tidydots-selected-files"
	tests := []struct {
		name  string
		files []string
	}{
		{name: "template-looking filename", files: []string{"{{ .Hostname }}", "test-host"}},
		{name: "environment-looking filename", files: []string{"$TIDYDOTS_SELECTED_FILE", "resolved-name"}},
		{name: "invalid template syntax is literal", files: []string{"{{ if }}", "other"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publicDir, privateDir := t.TempDir(), t.TempDir()
			saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "public", Backup: "./source", Files: []string{test.files[0]}, Targets: map[string]string{"linux": target}}}}}})
			saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "private", Backup: "./source", Files: []string{test.files[1]}, Targets: map[string]string{"linux": target}}}}}})
			managerPathA := filepath.Clean(filepath.Join(target, test.files[0]))
			managerPathB := filepath.Clean(filepath.Join(target, test.files[1]))
			if managerPathA == managerPathB {
				t.Fatalf("test filenames resolve to the same manager output: %q", managerPathA)
			}
			w, err := Load(publicDir, privateDir)
			if err != nil {
				t.Fatal(err)
			}
			plat := &platform.Platform{OS: platform.OSLinux, Hostname: "test-host", EnvVars: map[string]string{"TIDYDOTS_SELECTED_FILE": "resolved-name"}}
			if err := w.ValidateTargets(plat); err != nil {
				t.Fatalf("ValidateTargets() rejected distinct literal manager destinations %q and %q: %v", managerPathA, managerPathB, err)
			}
		})
	}
}

func TestValidateTargetsRejectsInactiveCrossRepositoryContributions(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", When: `{{ eq .OS "linux" }}`, Entries: []config.SubEntry{{Name: "public", Backup: "./source", Targets: map[string]string{"linux": "/tmp/shared"}}}}}})
	saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", When: `{{ eq .OS "windows" }}`, Entries: []config.SubEntry{{Name: "private", Backup: "./source", Targets: map[string]string{"linux": "/tmp/shared"}}}}}})
	w, err := Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateTargets(&platform.Platform{OS: platform.OSLinux}); err == nil {
		t.Fatal("ValidateTargets() ignored an inactive cross-repository ownership claim")
	}
}

func TestValidateTargetsReservesInactiveCrossRepositoryOwnership(t *testing.T) {
	for _, inactiveScope := range []string{"app.when", "entry.when"} {
		t.Run(inactiveScope, func(t *testing.T) {
			publicDir, privateDir := t.TempDir(), t.TempDir()
			live := filepath.Join(t.TempDir(), "live")
			if err := os.MkdirAll(live, 0700); err != nil {
				t.Fatal(err)
			}
			privateTarget := filepath.Join(live, "private.conf")
			if err := os.WriteFile(privateTarget, []byte("private live bytes\n"), 0600); err != nil {
				t.Fatal(err)
			}
			pub := config.Application{Name: "public-app", Entries: []config.SubEntry{{Name: "whole-folder", Backup: "./public", Targets: map[string]string{"linux": live}}}}
			priv := config.Application{Name: "private-app", Entries: []config.SubEntry{{Name: "private-file", Backup: "./private", Files: []string{"private.conf"}, Targets: map[string]string{"linux": live}}}}
			if inactiveScope == "app.when" {
				priv.When = `{{ eq .OS "windows" }}`
			} else {
				priv.Entries[0].When = `{{ eq .OS "windows" }}`
			}
			saveRepository(t, publicDir, &config.Config{Version: 3, Applications: []config.Application{pub}})
			saveRepository(t, privateDir, &config.Config{Version: 3, Applications: []config.Application{priv}})
			w, err := Load(publicDir, privateDir)
			if err != nil {
				t.Fatal(err)
			}
			err = w.ValidateTargets(&platform.Platform{OS: platform.OSLinux})
			if err == nil || !strings.Contains(err.Error(), "target conflict") {
				t.Fatalf("ValidateTargets() = %v, want cross-owner reservation conflict", err)
			}
			got, err := os.ReadFile(privateTarget)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "private live bytes\n" {
				t.Fatalf("private live file changed: %q", got)
			}
			if _, err := os.Stat(filepath.Join(publicDir, "public")); !os.IsNotExist(err) {
				t.Fatalf("preflight created public source: %v", err)
			}
		})
	}
}

func TestValidateTargetsKeepsInactiveSameOwnerCompositionPermitted(t *testing.T) {
	root := t.TempDir()
	saveRepository(t, root, &config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: []config.SubEntry{{Name: "folder", Backup: "./folder", Targets: map[string]string{"linux": "/tmp/tidydots-same-owner"}}, {Name: "inactive-child", Backup: "./child", Files: []string{"nested/file"}, Targets: map[string]string{"linux": "/tmp/tidydots-same-owner"}, When: `{{ eq .OS "windows" }}`}}}}})
	w, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.ValidateCrossRepositoryTargets(&platform.Platform{OS: platform.OSLinux}); err != nil {
		t.Fatalf("same-owner inactive composition rejected: %v", err)
	}
}
