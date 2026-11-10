package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
)

func clearGitObjectOverrides(t *testing.T) {
	t.Helper()
	keys := []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS"}
	prior := make(map[string]string)
	set := make(map[string]bool)
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			prior[key], set[key] = value, true
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Cleanup(func() {
		for key := range set {
			if err := os.Setenv(key, prior[key]); err != nil {
				t.Errorf("restore %s: %v", key, err)
			}
		}
	})
}

func TestWorkspaceRejectsGitObjectAlternates(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	clearGitObjectOverrides(t)
	public, private := t.TempDir(), t.TempDir()
	for _, root := range []string{public, private} {
		cmd := exec.CommandContext(context.Background(), git, "init", "-q", root)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init: %s: %v", output, err)
		}
		saveRepository(t, root, &config.Config{Version: 3})
	}
	alt := filepath.Join(private, ".git", "objects", "info", "alternates")
	if err := os.WriteFile(alt, []byte(filepath.Join(public, ".git", "objects")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(public, private); err == nil {
		t.Fatal("Load accepted an object alternate that prevents proving storage independence")
	}
}

func TestGitInspectionRejectsExistingRepositoryWhenOverridesPreventProof(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	clearGitObjectOverrides(t)
	root := t.TempDir()
	if output, err := exec.CommandContext(context.Background(), git, "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	t.Setenv("GIT_OBJECT_DIRECTORY", filepath.Join(root, ".git", "objects"))
	if _, err := shareGitObjectStore(root, t.TempDir()); err == nil {
		t.Fatal("Git environment override was treated as proof of independent storage")
	}
}

func TestGitDirOverrideWithoutPhysicalMetadataFailsClosed(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	clearGitObjectOverrides(t)
	store, public, private := t.TempDir(), t.TempDir(), t.TempDir()
	if output, err := exec.CommandContext(context.Background(), git, "init", "-q", store).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	for _, root := range []string{public, private} {
		saveRepository(t, root, &config.Config{Version: 3})
	}
	t.Setenv("GIT_DIR", filepath.Join(store, ".git"))
	var common []string
	for _, root := range []string{public, private} {
		output, err := exec.CommandContext(context.Background(), git, "-C", root, "rev-parse", "--git-common-dir").Output()
		if err != nil {
			t.Fatal(err)
		}
		common = append(common, strings.TrimSpace(string(output)))
	}
	if common[0] != common[1] {
		t.Fatal("fixture has independent effective Git common directories")
	}
	if _, err := Load(public, private); err == nil {
		t.Fatal("accepted roots sharing effective Git storage through GIT_DIR without physical .git metadata")
	}
}

func TestGitStorageOverridesFailClosedWithoutPhysicalMetadata(t *testing.T) {
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CONFIG", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS"} {
		t.Run(key, func(t *testing.T) {
			clearGitObjectOverrides(t)
			t.Setenv(key, "synthetic-override")
			shared, err := shareGitObjectStore(t.TempDir(), t.TempDir())
			if err == nil || shared {
				t.Fatalf("override was not rejected: shared=%v, err=%v", shared, err)
			}
		})
	}
}

func TestGitInspectionKeepsNonGitRootsUsableWithoutGitExecutable(t *testing.T) {
	path := t.TempDir()
	t.Setenv("PATH", path)
	shared, err := shareGitObjectStore(t.TempDir(), t.TempDir())
	if err != nil || shared {
		t.Fatalf("non-Git roots without git: shared=%v, err=%v", shared, err)
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := shareGitObjectStore(root, t.TempDir()); err == nil {
		t.Fatal("existing Git metadata without Git executable was treated as independent")
	}
}
