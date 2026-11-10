package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/packages"
	"github.com/AntoineGS/tidydots/internal/tui"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

func TestRunInit(t *testing.T) {
	useIsolatedHome(t)

	tests := []struct {
		setupPath   func(t *testing.T) string
		name        string
		errContains string
		wantErr     bool
	}{
		{
			name: "valid directory",
			setupPath: func(t *testing.T) string {
				return t.TempDir()
			},
			wantErr: false,
		},
		{
			name: "non-existent directory",
			setupPath: func(t *testing.T) string {
				tmpDir := t.TempDir()
				return filepath.Join(tmpDir, "non-existent")
			},
			wantErr:     true,
			errContains: "directory does not exist",
		},
		{
			name: "file instead of directory",
			setupPath: func(t *testing.T) string {
				tmpDir := t.TempDir()
				filePath := filepath.Join(tmpDir, "file.txt")
				if err := os.WriteFile(filePath, []byte("test"), 0600); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return filePath
			},
			wantErr:     true,
			errContains: "not a directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setupPath(t)
			args := []string{path}

			err := runInit(nil, args)

			if tt.wantErr {
				if err == nil {
					t.Errorf("runInit() expected error containing %q, got nil", tt.errContains)
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("runInit() error = %v, want error containing %q", err, tt.errContains)
				}
			} else if err != nil {
				t.Errorf("runInit() unexpected error = %v", err)
			}
		})
	}
}

func TestRunInitPreservesHostnamesWhenPreviousRepoIsUnavailable(t *testing.T) {
	useIsolatedHome(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: filepath.Join(t.TempDir(), "gone"), Hostnames: []string{"desktop", "laptop"}}); err != nil {
		t.Fatal(err)
	}
	newRepo := t.TempDir()
	if err := runInit(nil, []string{newRepo}); err != nil {
		t.Fatalf("runInit() error = %v", err)
	}
	got, err := config.LoadAppConfigMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Hostnames, []string{"desktop", "laptop"}) {
		t.Fatalf("hostnames = %v, want preserved metadata", got.Hostnames)
	}
}

func TestInitPublicRepoWithIsolatedHomeSiblingInsideWorkingDirectory(t *testing.T) {
	resetCLIFlags(t)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workingRoot := t.TempDir()
	if err := os.Chdir(workingRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})

	home := filepath.Join(workingRoot, "fake-home")
	publicRoot := filepath.Join(workingRoot, "public")
	for _, dir := range []string{home, publicRoot} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	cmd := newRootCommand()
	cmd.SetArgs([]string{"init", publicRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init public repository with isolated sibling HOME: %v", err)
	}
	got, err := config.LoadAppConfigMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigDir != publicRoot {
		t.Fatalf("registered public root = %q, want %q", got.ConfigDir, publicRoot)
	}
}

func TestGetConfigDir(t *testing.T) {
	useIsolatedHome(t)
	// Save original value
	originalConfigDir := configDir
	defer func() {
		configDir = originalConfigDir
	}()

	// Remove config file to test "app config not found" case
	appConfigPath := config.AppConfigPath()
	if err := os.Remove(appConfigPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		flagValue  string
		errMessage string
		wantAbs    bool
		wantErr    bool
	}{
		{
			name:      "flag override with relative path",
			flagValue: ".",
			wantAbs:   true,
			wantErr:   false,
		},
		{
			name:      "flag override with absolute path",
			flagValue: t.TempDir(),
			wantAbs:   true,
			wantErr:   false,
		},
		{
			name:      "empty flag falls back to app config",
			flagValue: "",
			wantErr:   true, // Will fail because app config was removed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configDir = tt.flagValue

			result, err := getConfigDir()

			if tt.wantErr {
				if err == nil {
					t.Errorf("getConfigDir() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("getConfigDir() unexpected error = %v", err)
				return
			}

			if tt.wantAbs && !filepath.IsAbs(result) {
				t.Errorf("getConfigDir() = %v, want absolute path", result)
			}

			// When flag is set, result should be the absolute version of the flag
			if tt.flagValue != "" {
				expectedAbs, _ := filepath.Abs(tt.flagValue)
				if result != expectedAbs {
					t.Errorf("getConfigDir() = %v, want %v", result, expectedAbs)
				}
			}
		})
	}
}

func TestInitRegistersPrivateRepositoryAndRemovesIt(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)

	publicDir := makeValidGitRepo(t)
	privateDir := makeValidGitRepo(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicDir, Hostnames: []string{"desktop"}}); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand()
	cmd.SetArgs([]string{"init", "--private", privateDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --private: %v", err)
	}
	got, err := config.LoadAppConfigMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if got.PrivateConfigDir != privateDir {
		t.Fatalf("private_config_dir = %q, want %q", got.PrivateConfigDir, privateDir)
	}
	if got.ConfigDir != publicDir || !reflect.DeepEqual(got.Hostnames, []string{"desktop"}) {
		t.Fatalf("registration changed existing local settings: %+v", got)
	}
	for _, root := range []string{publicDir, privateDir} {
		assertTidydotsGitExcludes(t, root)
	}

	cmd = newRootCommand()
	cmd.SetArgs([]string{"init", "--remove-private"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --remove-private: %v", err)
	}
	got, err = config.LoadAppConfigMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if got.PrivateConfigDir != "" || got.ConfigDir != publicDir || !reflect.DeepEqual(got.Hostnames, []string{"desktop"}) {
		t.Fatalf("remove-private changed unexpected settings: %+v", got)
	}
}

func TestInitPrivateDryRunDoesNotPersist(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	publicDir := makeValidGitRepo(t)
	privateDir := makeValidGitRepo(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicDir}); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{publicDir, privateDir} {
		if err := workspace.RegisterGitExcludes(root); err != nil {
			t.Fatalf("prepare excludes in %s: %v", root, err)
		}
	}

	publicExcludePath := filepath.Join(publicDir, ".git", "info", "exclude")
	privateExcludePath := filepath.Join(privateDir, ".git", "info", "exclude")
	publicBefore, err := os.ReadFile(publicExcludePath)
	if err != nil {
		t.Fatal(err)
	}
	privateBefore, err := os.ReadFile(privateExcludePath)
	if err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand()
	cmd.SetArgs([]string{"--dry-run", "init", "--private", privateDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run init --private: %v", err)
	}
	got, err := config.LoadAppConfigMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if got.PrivateConfigDir != "" {
		t.Fatalf("dry-run registered private repository %q", got.PrivateConfigDir)
	}
	publicAfter, err := os.ReadFile(publicExcludePath)
	if err != nil {
		t.Fatal(err)
	}
	privateAfter, err := os.ReadFile(privateExcludePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(publicAfter, publicBefore) || !bytes.Equal(privateAfter, privateBefore) {
		t.Fatalf("dry-run modified Git excludes: public %q -> %q, private %q -> %q", publicBefore, publicAfter, privateBefore, privateAfter)
	}
}

func TestInitPrivateRegistrationSupportsNonGitConfigDirectories(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	publicDir, privateDir := makeValidRepo(t), makeValidRepo(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicDir}); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	cmd.SetArgs([]string{"init", "--private", privateDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("register non-Git config directories: %v", err)
	}
	got, err := config.LoadAppConfigMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if got.PrivateConfigDir != privateDir {
		t.Fatalf("registered private root = %q, want %q", got.PrivateConfigDir, privateDir)
	}
	for _, root := range []string{publicDir, privateDir} {
		if _, err := os.Lstat(filepath.Join(root, ".git")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("registration created Git metadata in non-Git root %s: %v", root, err)
		}
	}
}

func TestInitPrivateDryRunRejectsMissingEffectiveGitExcludesWithoutWriting(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	publicDir, privateDir := makeValidGitRepo(t), makeValidGitRepo(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: publicDir}); err != nil {
		t.Fatal(err)
	}
	publicExcludePath := filepath.Join(publicDir, ".git", "info", "exclude")
	privateExcludePath := filepath.Join(privateDir, ".git", "info", "exclude")
	for _, path := range []string{publicExcludePath, privateExcludePath} {
		if err := os.WriteFile(path, []byte("# unchanged\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	cmd := newRootCommand()
	cmd.SetArgs([]string{"--dry-run", "init", "--private", privateDir})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "does not effectively ignore") {
		t.Fatalf("dry-run error = %v, want missing effective ignore guidance", err)
	}
	for _, path := range []string{publicExcludePath, privateExcludePath} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "# unchanged\n" {
			t.Fatalf("dry-run modified %s: %q", path, got)
		}
	}
}

func TestInitPrimaryDryRunDoesNotPersist(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	initial := makeValidRepo(t)
	proposed := makeValidRepo(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: initial, PrivateConfigDir: makeValidRepo(t)}); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand()
	cmd.SetArgs([]string{"init", proposed, "-n"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run init primary: %v", err)
	}
	got, err := config.LoadAppConfigMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigDir != initial || got.PrivateConfigDir == "" {
		t.Fatalf("dry-run changed local configuration: %+v", got)
	}
}

func TestInitPrimarySwitchClearsPrivateUnlessSameCanonicalPath(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	oldPrimary, newPrimary, privateDir := makeValidRepo(t), makeValidRepo(t), makeValidRepo(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: oldPrimary, PrivateConfigDir: privateDir, Hostnames: []string{"desktop"}}); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	cmd.SetArgs([]string{"init", newPrimary})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("switch primary: %v", err)
	}
	probeRelease, err := workspace.AcquireMutationLocks(context.Background(), oldPrimary, newPrimary, privateDir)
	if err != nil {
		t.Fatalf("registration left workspace roots locked after return: %v", err)
	}
	probeRelease()
	got, err := config.LoadAppConfigMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if got.PrivateConfigDir != "" || got.ConfigDir != newPrimary || !reflect.DeepEqual(got.Hostnames, []string{"desktop"}) {
		t.Fatalf("switch primary did not clear only private association: %+v", got)
	}

	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: oldPrimary, PrivateConfigDir: privateDir}); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "primary-alias")
	if err := os.Symlink(oldPrimary, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	cmd = newRootCommand()
	cmd.SetArgs([]string{"init", alias})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init same canonical primary: %v", err)
	}
	got, err = config.LoadAppConfigMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if got.PrivateConfigDir != privateDir {
		t.Fatalf("same canonical primary lost private association: %q", got.PrivateConfigDir)
	}
}

func TestAppliedRegistrationChangesRejectHeldWorkspaceLocks(t *testing.T) {
	tests := []struct {
		name  string
		args  func(public, private, nextPublic string) []string
		roots func(public, private, nextPublic string) []string
		setup func(t *testing.T, public, private string)
	}{
		{
			name:  "primary switch",
			args:  func(_, _, next string) []string { return []string{"init", next} },
			roots: func(public, private, next string) []string { return []string{public, private, next} },
			setup: func(t *testing.T, public, private string) {
				t.Helper()
				if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: public, PrivateConfigDir: private}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:  "private pairing",
			args:  func(_, private, _ string) []string { return []string{"init", "--private", private} },
			roots: func(public, private, _ string) []string { return []string{public, private} },
			setup: func(t *testing.T, public, _ string) {
				t.Helper()
				if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: public}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:  "private re-pairing",
			args:  func(_, _, next string) []string { return []string{"init", "--private", next} },
			roots: func(public, private, next string) []string { return []string{public, private, next} },
			setup: func(t *testing.T, public, private string) {
				t.Helper()
				if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: public, PrivateConfigDir: private}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:  "private removal",
			args:  func(_, _, _ string) []string { return []string{"init", "--remove-private"} },
			roots: func(public, private, _ string) []string { return []string{public, private} },
			setup: func(t *testing.T, public, private string) {
				t.Helper()
				if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: public, PrivateConfigDir: private}); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useIsolatedHome(t)
			resetCLIFlags(t)
			public, private, next := makeValidGitRepo(t), makeValidGitRepo(t), makeValidGitRepo(t)
			tt.setup(t, public, private)
			before, err := os.ReadFile(config.AppConfigPath())
			if err != nil {
				t.Fatal(err)
			}
			excludes := []string{
				filepath.Join(public, ".git", "info", "exclude"),
				filepath.Join(private, ".git", "info", "exclude"),
				filepath.Join(next, ".git", "info", "exclude"),
			}
			excludeBefore := make([][]byte, len(excludes))
			for i, path := range excludes {
				excludeBefore[i], err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			release, err := workspace.AcquireMutationLocks(context.Background(), tt.roots(public, private, next)...)
			if err != nil {
				t.Fatal(err)
			}
			defer release()

			cmd := newRootCommand()
			cmd.SetArgs(tt.args(public, private, next))
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "locked") && !strings.Contains(err.Error(), "being modified") {
				t.Fatalf("registration error = %v, want held-root lock error", err)
			}
			after, err := os.ReadFile(config.AppConfigPath())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("registration changed app config under held lock: %q -> %q", before, after)
			}
			for i, path := range excludes {
				after, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(after, excludeBefore[i]) {
					t.Errorf("registration changed Git excludes under held lock for %s: %q -> %q", path, excludeBefore[i], after)
				}
			}
		})
	}
}

func TestInitPrivateDryRunDoesNotWriteGitExcludesWhenRulesMissing(t *testing.T) {
	useIsolatedHome(t)
	resetCLIFlags(t)
	public, private := makeValidGitRepo(t), makeValidGitRepo(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: public}); err != nil {
		t.Fatal(err)
	}
	paths := []string{filepath.Join(public, ".git", "info", "exclude"), filepath.Join(private, ".git", "info", "exclude")}
	before := make([][]byte, len(paths))
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		before[i] = data
	}
	cmd := newRootCommand()
	cmd.SetArgs([]string{"init", "--private", private, "-n"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "does not effectively ignore") {
		t.Fatalf("dry-run registration error = %v, want missing ignore guidance", err)
	}
	for i, path := range paths {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, before[i]) {
			t.Fatalf("dry-run changed Git exclude %s: %q -> %q", path, before[i], got)
		}
	}
}

func TestPreviewRejectsHeldWorkspaceLock(t *testing.T) {
	useIsolatedHome(t)
	public, private, template := previewFixture(t)
	release, err := workspace.AcquireMutationLocks(context.Background(), private)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := previewHelperCommand(ctx, public, private, template, false)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("preview did not reject held workspace lock before starting watcher: %s", output)
	}
	if err == nil || !strings.Contains(string(output), "workspace root is being modified") && !strings.Contains(string(output), "locked") {
		t.Fatalf("preview result = %v, output %s; want lock rejection", err, output)
	}
	if _, err := os.Lstat(template + ".rendered"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preview wrote rendered output despite held lock: %v", err)
	}
}

func TestPreviewDryRunRendersPlanWithoutWritingOrStartingWatcher(t *testing.T) {
	for _, test := range []struct {
		name       string
		existing   bool
		oldContent string
	}{
		{name: "absent output"},
		{name: "preserve existing output", existing: true, oldContent: "user-edited output\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			useIsolatedHome(t)
			public, private, template := previewFixture(t)
			rendered := template + ".rendered"
			if test.existing {
				if err := os.WriteFile(rendered, []byte(test.oldContent), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := previewHelperCommand(ctx, public, private, template, true)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("preview dry-run: %v: %s", err, output)
			}
			if !strings.Contains(string(output), "would render") || !strings.Contains(string(output), "watcher not started") {
				t.Fatalf("dry-run did not report a planned render: %s", output)
			}
			got, err := os.ReadFile(rendered)
			if test.existing {
				if err != nil || string(got) != test.oldContent {
					t.Fatalf("dry-run changed existing output: %q, %v", got, err)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("dry-run created rendered output %q: %v", got, err)
			}
		})
	}
}

func TestPreviewHoldsWorkspaceLockForWatcherSession(t *testing.T) {
	useIsolatedHome(t)
	public, private, template := previewFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := previewHelperCommand(ctx, public, private, template, false)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("preview did not complete initial render: %v; stderr %s", err, stderr.String())
	}
	if !strings.Contains(line, "source_map") {
		t.Fatalf("initial preview output = %q, want source map", line)
	}
	if _, err := workspace.MutationGuard(context.Background(), "", private); err == nil {
		t.Fatal("another mutation acquired the private root lock while preview watcher was active")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("preview watcher shutdown: %v; stderr %s", err, stderr.String())
	}
}

func TestPreviewCommandHelper(t *testing.T) {
	if os.Getenv("TIDYDOTS_PREVIEW_HELPER") != "1" {
		return
	}
	public, private, template := os.Getenv("TIDYDOTS_PREVIEW_PUBLIC"), os.Getenv("TIDYDOTS_PREVIEW_PRIVATE"), os.Getenv("TIDYDOTS_PREVIEW_TEMPLATE")
	args := []string{"--dir", public, "--private-dir", private}
	if os.Getenv("TIDYDOTS_PREVIEW_DRY_RUN") == "1" {
		args = append(args, "--dry-run")
	}
	args = append(args, "preview", template)
	cmd := newRootCommand()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func previewHelperCommand(ctx context.Context, public, private, template string, dryRun bool) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPreviewCommandHelper$")
	env := append(os.Environ(),
		"TIDYDOTS_PREVIEW_HELPER=1",
		"TIDYDOTS_PREVIEW_PUBLIC="+public,
		"TIDYDOTS_PREVIEW_PRIVATE="+private,
		"TIDYDOTS_PREVIEW_TEMPLATE="+template,
	)
	if dryRun {
		env = append(env, "TIDYDOTS_PREVIEW_DRY_RUN=1")
	}
	cmd.Env = env
	return cmd
}

func previewFixture(t *testing.T) (public, private, template string) {
	t.Helper()
	resetCLIFlags(t)
	public, private = makeValidRepo(t), makeValidRepo(t)
	template = filepath.Join(private, "sample.tmpl")
	if err := os.WriteFile(template, []byte("rendered preview\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return public, private, template
}

func TestInitPrivateRejectsInvalidRepositoriesAndArgumentCombinations(t *testing.T) {
	useIsolatedHome(t)
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing mode"},
		{name: "private plus positional", args: []string{"--private", t.TempDir(), t.TempDir()}},
		{name: "remove plus positional", args: []string{"--remove-private", t.TempDir()}},
		{name: "both private modes", args: []string{"--private", t.TempDir(), "--remove-private"}},
		{name: "false remove flag", args: []string{"--remove-private=false"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetCLIFlags(t)
			cmd := newRootCommand()
			cmd.SetArgs(append([]string{"init"}, tt.args...))
			if err := cmd.Execute(); err == nil {
				t.Fatal("expected argument validation error")
			}
		})
	}

	resetCLIFlags(t)
	if err := config.SaveAppConfig(&config.AppConfig{ConfigDir: makeValidRepo(t)}); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCommand()
	cmd.SetArgs([]string{"init", "--private", t.TempDir()})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected invalid private config to be rejected")
	}
}

func TestPrivateSelectionFlagsAreMutuallyExclusive(t *testing.T) {
	resetCLIFlags(t)
	cmd := newRootCommand()
	cmd.SetArgs([]string{"--private-dir", t.TempDir(), "--public-only", "list"})
	if err := cmd.Execute(); err == nil || !contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error = %v, want mutually exclusive validation", err)
	}

	resetCLIFlags(t)
	cmd = newRootCommand()
	cmd.SetArgs([]string{"--private-dir=", "list"})
	if err := cmd.Execute(); err == nil || !contains(err.Error(), "requires a path") {
		t.Fatalf("error = %v, want empty path validation", err)
	}
}

func TestWriteHumanStatusShowsWorkspaceOwners(t *testing.T) {
	var output bytes.Buffer
	report := tui.StatusReport{Applications: []tui.StatusApplication{{
		Name:    "shell",
		Package: &tui.StatusPackage{Repository: "private", Name: "shell-package", Method: "apt"},
		Entries: []tui.StatusEntry{{Repository: "private", Name: "private-config", State: "Ready"}},
	}}}
	if err := writeHumanStatus(&output, report); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"shell-package (apt) [private]", "private-config [private]: Ready"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("human status output missing %q:\n%s", want, output.String())
		}
	}
}

func useIsolatedHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
}

func resetCLIFlags(t *testing.T) {
	t.Helper()
	configDir, osOverride, dryRun, verbose, interactive, actions = "", "", false, false, false, false
	statusActions, statusJSON, noMerge, forceDelete, forceRender = false, false, false, false, false
	privateDir, publicOnly, initPrivateDir, removePrivate = "", false, "", false
	t.Cleanup(func() {
		configDir, osOverride, dryRun, verbose, interactive, actions = "", "", false, false, false, false
		statusActions, statusJSON, noMerge, forceDelete, forceRender = false, false, false, false, false
		privateDir, publicOnly, initPrivateDir, removePrivate = "", false, "", false
	})
}

func makeValidRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := config.Save(&config.Config{Version: 3}, filepath.Join(dir, "tidydots.yaml")); err != nil {
		t.Fatalf("save repository config: %v", err)
	}
	return dir
}

func makeValidGitRepo(t *testing.T) string {
	t.Helper()
	dir := makeValidRepo(t)
	if output, err := exec.CommandContext(context.Background(), "git", "-C", dir, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("initialize Git test repository: %v: %s", err, output)
	}
	return dir
}

func assertTidydotsGitExcludes(t *testing.T, root string) {
	t.Helper()
	for _, path := range []string{
		".tidydots-txn/probe",
		".tidydots-txn-config-probe",
		".tidydots.db",
		"sample.tmpl.rendered",
		"sample.tmpl.conflict",
	} {
		cmd := exec.CommandContext(context.Background(), "git", "-C", root, "check-ignore", "--no-index", "-q", path)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("Git does not ignore %q in %s: %v: %s", path, root, err, output)
		}
	}
}

func TestConvertToPackageManagers(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []packages.PackageManager
	}{
		{
			name:  "empty slice",
			input: []string{},
			want:  []packages.PackageManager{},
		},
		{
			name:  "single manager",
			input: []string{"pacman"},
			want:  []packages.PackageManager{packages.PackageManager("pacman")},
		},
		{
			name:  "multiple managers",
			input: []string{"yay", "paru", "pacman"},
			want: []packages.PackageManager{
				packages.PackageManager("yay"),
				packages.PackageManager("paru"),
				packages.PackageManager("pacman"),
			},
		},
		{
			name:  "nil input",
			input: nil,
			want:  []packages.PackageManager{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToPackageManagers(tt.input)

			if len(got) != len(tt.want) {
				t.Errorf("convertToPackageManagers() returned %d items, want %d", len(got), len(tt.want))
				return
			}

			for i, pm := range got {
				if pm != tt.want[i] {
					t.Errorf("convertToPackageManagers()[%d] = %v, want %v", i, pm, tt.want[i])
				}
			}
		})
	}
}

// contains checks if substr is in s
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && searchSubstring(s, substr)))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
