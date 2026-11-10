package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/state"
)

func TestVerifyPathParentsRejectsSymlinkedAncestor(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(root, "alias")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := verifyPathParents(root, filepath.Join(root, "alias", "new", "file")); err == nil {
		t.Fatal("accepted a path whose existing ancestor is a symlink")
	}
}

func TestVerifyPathParentsAllowsMissingParentsInsideCanonicalRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := verifyPathParents(root, filepath.Join(root, "new", "nested", "file")); err != nil {
		t.Fatalf("missing in-root parents should be allowed: %v", err)
	}
	if err := verifyPathParents(root, filepath.Join(filepath.Dir(root), "outside")); err == nil {
		t.Fatal("accepted a path outside the repository")
	}
}

func TestVerifyProtectedDirectoryRejectsSymlinkAncestryAndRepoOverlap(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	stateParent := t.TempDir()
	alias := filepath.Join(stateParent, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := verifyProtectedDirectory(filepath.Join(alias, "state")); err == nil {
		t.Fatal("accepted protected state with symlinked ancestry")
	}
	if _, err := verifyProtectedDirectory(filepath.Join(root, "state"), root); err == nil {
		t.Fatal("accepted protected state overlapping a repository")
	}
	resolved, err := verifyProtectedDirectory(filepath.Join(stateParent, "state"), root)
	if err != nil {
		t.Fatalf("safe outside protected path rejected: %v", err)
	}
	if !filepath.IsAbs(resolved) {
		t.Fatalf("protected path is not canonical absolute path: %q", resolved)
	}
}

func TestValidateMovedSymlinksResolvesExpandedComponents(t *testing.T) {
	files := []fileSnapshot{
		{rel: ".", dir: true},
		{rel: "deep", dir: true},
		{rel: "deep/b", link: ".."},
		{rel: "a", link: "deep/b/../outside"},
		{rel: "outside", data: []byte("not a valid referent")},
	}
	if err := validateMovedSymlinks(files); err == nil {
		t.Fatal("accepted link that escapes after resolving a nested symlink and ..")
	}
}

func TestCheckExpandedSourceOwnershipBlocksOrdinaryTemplateAlias(t *testing.T) {
	t.Parallel()
	publicRoot, privateRoot := t.TempDir(), t.TempDir()
	backup := filepath.Join(publicRoot, "app")
	if err := os.Mkdir(backup, 0700); err != nil {
		t.Fatal(err)
	}
	publicConfig := &config.Config{Applications: []config.Application{{
		Name: "app",
		Entries: []config.SubEntry{
			{Name: "template", Backup: "./app", Files: []string{"f.tmpl"}},
			{Name: "literal", Backup: "./app", Files: []string{"f"}},
		},
	}}}
	w := &Workspace{Repositories: []*Repository{
		{Role: Public, Root: publicRoot, Config: publicConfig},
		{Role: Private, Root: privateRoot, Config: &config.Config{}},
	}}
	files := []fileSnapshot{{rel: "f.tmpl"}, {rel: "f"}}
	err := checkExpandedSourceOwnership(w, EntryRef{Repository: Public, App: "app", Name: "template"}, backup, files)
	if err == nil || !strings.Contains(err.Error(), "literal") {
		t.Fatalf("expected expanded alias collision with literal owner; got %v", err)
	}
}

func TestSnapshotOwnedDoesNotGuessOrdinaryTemplateAlias(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f.tmpl"), []byte("template"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "f"), []byte("independent literal"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := snapshotOwned(root, []string{"f.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.rel == "f" {
			t.Fatal("ordinary alias-named file was guessed as a template artifact")
		}
	}
}

func TestSnapshotOwnedIncludesNestedRecognizedTemplateArtifacts(t *testing.T) {
	root := t.TempDir()
	writeSourceTestFile(t, filepath.Join(root, "nested", "f.tmpl"), "template")
	writeSourceTestFile(t, filepath.Join(root, "nested", "f.tmpl.rendered"), "edited render")
	if err := os.Symlink("f.tmpl.rendered", filepath.Join(root, "nested", "f")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	files, err := snapshotOwned(root, []string{"nested/f.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"nested/f", "nested/f.tmpl.rendered"} {
		if !hasSnapshotPath(files, path) {
			t.Errorf("nested generated artifact %q omitted: %#v", path, snapshotNames(files))
		}
	}
}

func TestExpandAttributedArtifactsCopiesConflictWithoutAlias(t *testing.T) {
	root := t.TempDir()
	writeSourceTestFile(t, filepath.Join(root, "nested", "f.tmpl"), "copy template")
	for _, suffix := range []string{".rendered", ".conflict", ".rendered.bak"} {
		writeSourceTestFile(t, filepath.Join(root, "nested", "f.tmpl"+suffix), "history-attributed artifact")
	}
	base := []fileSnapshot{{rel: "nested/f.tmpl", mode: 0600, data: []byte("copy template")}}
	records := []state.RenderRecord{{TemplatePath: `copy:"./app/nested/f.tmpl":"/home/user/config"`}}
	files, err := expandAttributedTemplateArtifacts(root, base, "app", config.MethodCopy, records, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"nested/f.tmpl.rendered", "nested/f.tmpl.conflict", "nested/f.tmpl.rendered.bak"} {
		if !hasSnapshotPath(files, path) {
			t.Errorf("history-attributed copy artifact %q omitted without an alias: %#v", path, snapshotNames(files))
		}
	}
	if hasSnapshotPath(files, "nested/f") {
		t.Fatalf("copy template fabricated a repository alias: %#v", snapshotNames(files))
	}
}

func TestExpandAttributedArtifactsMovesHistoryBackedSymlinkArtifactsWithoutAlias(t *testing.T) {
	root := t.TempDir()
	writeSourceTestFile(t, filepath.Join(root, "nested", "f.tmpl"), "symlink template")
	for _, suffix := range []string{".rendered", ".conflict", ".rendered.bak"} {
		writeSourceTestFile(t, filepath.Join(root, "nested", "f.tmpl"+suffix), "generated")
	}
	base := []fileSnapshot{{rel: "nested/f.tmpl", mode: 0600, data: []byte("symlink template")}}
	records := []state.RenderRecord{{TemplatePath: "./app/nested/f.tmpl"}}
	files, err := expandAttributedTemplateArtifacts(root, base, "app", config.MethodSymlink, records, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"nested/f.tmpl.rendered", "nested/f.tmpl.conflict", "nested/f.tmpl.rendered.bak"} {
		if !hasSnapshotPath(files, path) {
			t.Errorf("history-attributed artifact %q omitted: %#v", path, snapshotNames(files))
		}
	}
	if hasSnapshotPath(files, "nested/f") {
		t.Fatal("missing alias was fabricated in the snapshot")
	}
}

func TestExpandAttributedArtifactsDoesNotGuessUnownedSuffixes(t *testing.T) {
	root := t.TempDir()
	writeSourceTestFile(t, filepath.Join(root, "f.tmpl"), "template")
	writeSourceTestFile(t, filepath.Join(root, "f.tmpl.conflict"), "ordinary suffix-named file")
	base := []fileSnapshot{{rel: "f.tmpl", mode: 0600, data: []byte("template")}}
	if _, err := expandAttributedTemplateArtifacts(root, base, "app", config.MethodCopy, nil, nil); err == nil || !strings.Contains(err.Error(), "cannot prove ownership") {
		t.Fatalf("ambiguous suffix-named file error = %v, want attribution rejection", err)
	}
}

func TestExpandAttributedArtifactsUsesRepositoryBackupPrefix(t *testing.T) {
	root := t.TempDir()
	writeSourceTestFile(t, filepath.Join(root, "f.tmpl"), "template")
	writeSourceTestFile(t, filepath.Join(root, "f.tmpl.conflict"), "history-backed conflict")
	base := []fileSnapshot{{rel: "f.tmpl", mode: 0600, data: []byte("template")}}
	records := []state.RenderRecord{{TemplatePath: `copy:"./configurations/f.tmpl":"/home/user/config"`}}
	files, err := expandAttributedTemplateArtifacts(root, base, "configurations", config.MethodCopy, records, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSnapshotPath(files, "f.tmpl.conflict") {
		t.Fatalf("backup-prefixed render history was not attributed: %#v", snapshotNames(files))
	}
}

func TestExpandAttributedArtifactsResolvesLegacyStateRemap(t *testing.T) {
	root := t.TempDir()
	writeSourceTestFile(t, filepath.Join(root, "nested", "f.tmpl"), "template")
	writeSourceTestFile(t, filepath.Join(root, "nested", "f.tmpl.conflict"), "conflict")
	base := []fileSnapshot{{rel: "nested/f.tmpl", mode: 0600, data: []byte("template")}}
	legacy := "f.tmpl"
	stateKeys := []string{encodeStateRemap(legacy, "./app/nested/f.tmpl")}
	records := []state.RenderRecord{{TemplatePath: legacy}}
	files, err := expandAttributedTemplateArtifacts(root, base, "app", config.MethodSymlink, records, stateKeys)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSnapshotPath(files, "nested/f.tmpl.conflict") {
		t.Fatalf("legacy-attributed conflict omitted: %#v", snapshotNames(files))
	}
}

func writeSourceTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func hasSnapshotPath(files []fileSnapshot, path string) bool {
	for _, file := range files {
		if filepath.Clean(file.rel) == filepath.Clean(path) {
			return true
		}
	}
	return false
}

func snapshotNames(files []fileSnapshot) []string {
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = file.rel
	}
	return names
}

func TestValidateMovedSymlinksRejectsCyclesAndAcceptsContainedTargets(t *testing.T) {
	cycle := []fileSnapshot{{rel: ".", dir: true}, {rel: "a", link: "b"}, {rel: "b", link: "a"}}
	if err := validateMovedSymlinks(cycle); err == nil {
		t.Fatal("accepted a source symlink cycle")
	}
	contained := []fileSnapshot{{rel: ".", dir: true}, {rel: "dir", dir: true}, {rel: "dir/file", data: []byte("safe")}, {rel: "link", link: "dir/file"}}
	if err := validateMovedSymlinks(contained); err != nil {
		t.Fatalf("rejected a symlink resolving within the moved set: %v", err)
	}
}

func TestSnapshotOwnedAcceptsExplicitSymlinkThroughImplicitParent(t *testing.T) {
	root := t.TempDir()
	writeSourceTestFile(t, filepath.Join(root, "nested", "file"), "payload")
	if err := os.Symlink("nested/file", filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	files, err := snapshotOwned(root, []string{"nested/file", "link"})
	if err != nil {
		t.Fatalf("safe explicit source link rejected: %v", err)
	}
	if hasSnapshotPath(files, "nested") {
		t.Fatal("implicit structural parent was added to the transferred source set")
	}
	if !hasSnapshotPath(files, "nested/file") || !hasSnapshotPath(files, "link") {
		t.Fatalf("selected source nodes missing: %#v", snapshotNames(files))
	}
}

func TestCopySnapshotRejectsSymlinkedDestinationParents(t *testing.T) {
	stage, external := t.TempDir(), t.TempDir()
	if err := os.Symlink(external, filepath.Join(stage, "alias")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	err := copySnapshot(stage, []fileSnapshot{{rel: filepath.Join("alias", "escaped"), data: []byte("private")}})
	if err == nil {
		t.Fatal("copied a snapshot through a symlinked destination parent")
	}
	if _, err := os.Lstat(filepath.Join(external, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("snapshot wrote outside stage: %v", err)
	}
}

func TestCheckGeneratedIgnoresUsesBackupRootRelativePath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.invalid")
	runGit(t, root, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/f.tmpl.rendered\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := checkGeneratedIgnoresAt(context.Background(), root, "app", []fileSnapshot{{rel: "f.tmpl.rendered"}})
	if err == nil {
		t.Fatal("accepted root ignore for a generated artifact actually under app/")
	}
}

func TestRegisterGitExcludesWorksForLinkedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	base := t.TempDir()
	main := filepath.Join(base, "main")
	linked := filepath.Join(base, "linked")
	private := filepath.Join(base, "private")
	if err := os.Mkdir(main, 0700); err != nil {
		t.Fatal(err)
	}
	runGit(t, main, "init", "-q")
	runGit(t, main, "config", "user.email", "test@example.invalid")
	runGit(t, main, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(main, "tracked"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	runGit(t, main, "add", "tracked")
	runGit(t, main, "commit", "-qm", "initial")
	runGit(t, main, "worktree", "add", "-q", "-b", "linked-branch", linked)
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	runGit(t, private, "init", "-q")
	shared, err := shareGitObjectStore(linked, private)
	if err != nil || shared {
		t.Fatalf("linked public worktree should pair with independent private checkout: shared=%v err=%v", shared, err)
	}
	if err := RegisterGitExcludes(linked); err != nil {
		t.Fatalf("register excludes in linked worktree: %v", err)
	}
	for _, path := range []string{".tidydots-txn/marker", "app/f.tmpl.rendered"} {
		cmd := exec.CommandContext(context.Background(), "git", "check-ignore", "-q", "--no-index", path)
		cmd.Dir = linked
		if err := cmd.Run(); err != nil {
			t.Errorf("linked worktree does not effectively ignore %q: %v", path, err)
		}
	}
}

func TestRegisterGitExcludesRejectsSymlinkExcludeFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	info := filepath.Join(root, ".git", "info")
	path := filepath.Join(info, "exclude")
	external := filepath.Join(t.TempDir(), "external-exclude")
	if err := os.WriteFile(external, []byte("# external\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Symlink(external, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := RegisterGitExcludes(root); err == nil {
		t.Fatal("followed a symlinked Git exclude file")
	}
	contents, err := os.ReadFile(external)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "# external\n" {
		t.Fatalf("external exclude file was modified: %q", contents)
	}
}

func TestNonGitWorkspaceWorksWithoutGitExecutable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	if err := RegisterGitExcludes(root); err != nil {
		t.Fatalf("register excludes for NonGit root: %v", err)
	}
	if err := checkGeneratedIgnoresAt(context.Background(), root, "app", []fileSnapshot{{rel: "f.tmpl.conflict"}}); err != nil {
		t.Fatalf("check generated artifacts for NonGit root: %v", err)
	}
	if err := checkGitIgnore(context.Background(), root, transactionIgnoreDir); err != nil {
		t.Fatalf("check transaction ignore for NonGit root: %v", err)
	}
}

func TestGitWorkspaceRequiresGitExecutableForIgnoreChecks(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	t.Setenv("PATH", t.TempDir())
	if err := RegisterGitExcludes(root); err == nil {
		t.Fatal("registered excludes for a Git root without Git available to verify them")
	}
	if err := checkGeneratedIgnoresAt(context.Background(), root, "app", []fileSnapshot{{rel: "f.tmpl.conflict"}}); err == nil {
		t.Fatal("accepted generated-ignore check for Git root without Git")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, output)
	}
}
