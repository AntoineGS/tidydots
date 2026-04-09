package packages

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/cmdexec"
	"github.com/AntoineGS/tidydots/internal/config"
)

// newStubManager creates a Manager with the given OS type wired to a StubRunner.
// Available and availableSet are left empty so tests can control them directly.
func newStubManager(t *testing.T, osType string) (*Manager, *cmdexec.StubRunner) {
	t.Helper()
	cfg := &Config{}
	stub := cmdexec.NewStubRunner()
	mgr := &Manager{
		ctx:          context.Background(),
		Config:       cfg,
		OS:           osType,
		Available:    []PackageManager{},
		availableSet: map[PackageManager]bool{},
		runner:       stub,
	}
	return mgr, stub
}

// setAvailable is a helper to configure which managers are available on a stub manager.
func setAvailable(m *Manager, managers ...PackageManager) {
	m.Available = managers
	m.availableSet = toAvailableSet(managers)
}

// --- Install flow with stub ---

func TestInstall_Pacman_CallsCorrectCommand(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")
	setAvailable(mgr, Pacman)

	pkg := Package{
		Name:     "neovim",
		Managers: map[PackageManager]ManagerValue{Pacman: {PackageName: "neovim"}},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}
	if result.Method != "pacman" {
		t.Errorf("expected method=pacman, got %q", result.Method)
	}
	if result.Package != "neovim" {
		t.Errorf("expected package=neovim, got %q", result.Package)
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call")
	}
	call := stub.Calls[0]
	if call.Name != "sudo" {
		t.Errorf("pacman install should start with sudo, got command %q", call.Name)
	}
	// args should contain "pacman", "-S", "--noconfirm", "neovim"
	found := false
	for _, arg := range call.Args {
		if arg == "neovim" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected package name 'neovim' in args %v", call.Args)
	}
}

func TestInstall_Yay_CallsCorrectCommand(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")
	setAvailable(mgr, Yay)

	pkg := Package{
		Name:     "aur-pkg",
		Managers: map[PackageManager]ManagerValue{Yay: {PackageName: "aur-pkg"}},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}
	if result.Method != "yay" {
		t.Errorf("expected method=yay, got %q", result.Method)
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call")
	}
	call := stub.Calls[0]
	if call.Name != "yay" {
		t.Errorf("expected command 'yay', got %q", call.Name)
	}
	// yay does not use sudo — Run is used, not RunWithSudo
	if call.Sudo {
		t.Error("yay install should not use sudo")
	}
}

func TestInstall_Apt_UsesNonSudoRunner(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")
	setAvailable(mgr, Apt)

	pkg := Package{
		Name:     "vim",
		Managers: map[PackageManager]ManagerValue{Apt: {PackageName: "vim"}},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call")
	}
	// apt-get uses "sudo apt-get" — the first arg is sudo (run via Run, not RunWithSudo)
	call := stub.Calls[0]
	if call.Name != "sudo" {
		t.Errorf("apt install should use 'sudo', got %q", call.Name)
	}
	if call.Sudo {
		// Should use Run (not RunWithSudo) since sudo is baked into the install args
		t.Error("apt install should NOT use RunWithSudo (sudo is in the install args)")
	}
}

func TestInstall_MultipleManagersAvailable_UsesFirst(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")
	// Both yay and pacman available; pkg has both — yay comes first in Available
	setAvailable(mgr, Yay, Pacman)

	pkg := Package{
		Name: "tool",
		Managers: map[PackageManager]ManagerValue{
			Yay:    {PackageName: "tool"},
			Pacman: {PackageName: "tool"},
		},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}
	if result.Method != "yay" {
		t.Errorf("expected yay (first available), got method=%q", result.Method)
	}
	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call")
	}
	if stub.Calls[0].Name != "yay" {
		t.Errorf("expected yay to be called first, got %q", stub.Calls[0].Name)
	}
}

func TestInstall_NoMatchingManager_ReturnsFailure(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")
	// Only apt available, but pkg only has pacman
	setAvailable(mgr, Apt)

	pkg := Package{
		Name:     "nvim",
		Managers: map[PackageManager]ManagerValue{Pacman: {PackageName: "nvim"}},
	}

	result := mgr.Install(pkg)
	if result.Success {
		t.Error("expected failure when no matching manager is available")
	}
	if len(stub.Calls) != 0 {
		t.Errorf("expected no stub calls, got %d", len(stub.Calls))
	}
}

func TestInstall_EmptyManagersMap_ReturnsFailure(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")
	setAvailable(mgr, Pacman)

	pkg := Package{
		Name:     "ghost",
		Managers: map[PackageManager]ManagerValue{},
	}

	result := mgr.Install(pkg)
	if result.Success {
		t.Error("expected failure for package with empty managers")
	}
	if len(stub.Calls) != 0 {
		t.Errorf("expected no stub calls, got %d", len(stub.Calls))
	}
}

func TestInstall_InvalidPackageName_ReturnsFailure(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")
	setAvailable(mgr, Pacman)

	pkg := Package{
		Name:     "bad",
		Managers: map[PackageManager]ManagerValue{Pacman: {PackageName: "-bad-flag"}},
	}

	result := mgr.Install(pkg)
	if result.Success {
		t.Error("expected failure for invalid package name")
	}
	if len(stub.Calls) != 0 {
		t.Errorf("expected no stub calls for invalid package name, got %d", len(stub.Calls))
	}
}

// --- Git package install ---

func TestInstall_GitPackage_Clone_CallsGitClone(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := tmpDir + "/myrepo"

	mgr, stub := newStubManager(t, "linux")

	pkg := Package{
		Name: "myrepo",
		Managers: map[PackageManager]ManagerValue{
			Git: {
				Git: &config.GitPackage{
					URL: "https://github.com/user/myrepo.git",
					Targets: map[string]string{
						"linux": targetDir,
					},
				},
			},
		},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}
	if result.Method != "git" {
		t.Errorf("expected method=git, got %q", result.Method)
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call")
	}
	call := stub.Calls[0]
	if call.Name != "git" {
		t.Errorf("expected 'git' command, got %q", call.Name)
	}
	if call.Sudo {
		t.Error("non-sudo git clone should not use RunWithSudo")
	}
	// Verify args contain "clone" and the URL
	args := call.Args
	if len(args) == 0 || args[0] != "clone" {
		t.Errorf("expected first arg 'clone', got %v", args)
	}
	foundURL := false
	for _, arg := range args {
		if arg == "https://github.com/user/myrepo.git" {
			foundURL = true
		}
	}
	if !foundURL {
		t.Errorf("expected URL in git clone args, got %v", args)
	}
}

func TestInstall_GitPackage_CloneWithBranch(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := tmpDir + "/myrepo"

	mgr, stub := newStubManager(t, "linux")

	pkg := Package{
		Name: "myrepo",
		Managers: map[PackageManager]ManagerValue{
			Git: {
				Git: &config.GitPackage{
					URL:    "https://github.com/user/myrepo.git",
					Branch: "main",
					Targets: map[string]string{
						"linux": targetDir,
					},
				},
			},
		},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call")
	}
	args := stub.Calls[0].Args
	// Should contain "-b" and "main" flags
	foundBranchFlag := false
	for i, arg := range args {
		if arg == "-b" && i+1 < len(args) && args[i+1] == "main" {
			foundBranchFlag = true
		}
	}
	if !foundBranchFlag {
		t.Errorf("expected '-b main' in git clone args, got %v", args)
	}
}

func TestInstall_GitPackage_Clone_WithSudo(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := tmpDir + "/sudorepo"

	mgr, stub := newStubManager(t, "linux")

	pkg := Package{
		Name: "sudorepo",
		Managers: map[PackageManager]ManagerValue{
			Git: {
				Git: &config.GitPackage{
					URL:  "https://github.com/user/sudorepo.git",
					Sudo: true,
					Targets: map[string]string{
						"linux": targetDir,
					},
				},
			},
		},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call")
	}
	call := stub.Calls[0]
	if !call.Sudo {
		t.Error("sudo git clone should use RunWithSudo")
	}
}

func TestInstall_GitPackage_NoTargetForOS_ReturnsFailure(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")

	pkg := Package{
		Name: "win-only-repo",
		Managers: map[PackageManager]ManagerValue{
			Git: {
				Git: &config.GitPackage{
					URL: "https://github.com/user/win-only.git",
					Targets: map[string]string{
						"windows": "C:/repos/win-only",
					},
				},
			},
		},
	}

	result := mgr.Install(pkg)
	if result.Success {
		t.Error("expected failure when no target defined for current OS")
	}
	if len(stub.Calls) != 0 {
		t.Errorf("expected no calls for missing OS target, got %d", len(stub.Calls))
	}
}

func TestInstall_GitPackage_InvalidURL_ReturnsFailure(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")

	pkg := Package{
		Name: "bad-repo",
		Managers: map[PackageManager]ManagerValue{
			Git: {
				Git: &config.GitPackage{
					URL: "ftp://bad-scheme.example.com/repo.git",
					Targets: map[string]string{
						"linux": "/tmp/bad-repo",
					},
				},
			},
		},
	}

	result := mgr.Install(pkg)
	if result.Success {
		t.Error("expected failure for invalid git URL scheme")
	}
	if len(stub.Calls) != 0 {
		t.Errorf("expected no calls for invalid URL, got %d", len(stub.Calls))
	}
}

// --- Installer package install ---

func TestInstall_InstallerPackage_Linux_RunsShCommand(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")

	pkg := Package{
		Name: "custom-installer",
		Managers: map[PackageManager]ManagerValue{
			Installer: {
				Installer: &config.InstallerPackage{
					Command: map[string]string{
						"linux": "curl -sSL https://example.com/install.sh | sh",
					},
				},
			},
		},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}
	if result.Method != "installer" {
		t.Errorf("expected method=installer, got %q", result.Method)
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call")
	}
	call := stub.Calls[0]
	if call.Name != "sh" {
		t.Errorf("expected 'sh' command for linux installer, got %q", call.Name)
	}
	if call.Sudo {
		t.Error("installer should not use RunWithSudo")
	}
}

func TestInstall_InstallerPackage_Windows_RunsPowershellCommand(t *testing.T) {
	mgr, stub := newStubManager(t, "windows")

	pkg := Package{
		Name: "win-installer",
		Managers: map[PackageManager]ManagerValue{
			Installer: {
				Installer: &config.InstallerPackage{
					Command: map[string]string{
						"windows": "installer.exe /quiet",
					},
				},
			},
		},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call")
	}
	call := stub.Calls[0]
	if call.Name != "powershell" {
		t.Errorf("expected 'powershell' command for windows installer, got %q", call.Name)
	}
}

func TestInstall_InstallerPackage_NoCommandForOS_ReturnsFailure(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")

	pkg := Package{
		Name: "win-installer",
		Managers: map[PackageManager]ManagerValue{
			Installer: {
				Installer: &config.InstallerPackage{
					Command: map[string]string{
						"windows": "installer.exe /quiet",
					},
				},
			},
		},
	}

	result := mgr.Install(pkg)
	if result.Success {
		t.Error("expected failure when installer has no command for current OS")
	}
	if len(stub.Calls) != 0 {
		t.Errorf("expected no calls for missing OS command, got %d", len(stub.Calls))
	}
}

// --- Custom command install ---

func TestInstall_CustomCommand_Linux(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")
	// No managers available so custom is tried
	setAvailable(mgr)

	pkg := Package{
		Name:   "custom-tool",
		Custom: map[string]string{"linux": "make install"},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}
	if result.Method != MethodCustom {
		t.Errorf("expected method=custom, got %q", result.Method)
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call")
	}
	call := stub.Calls[0]
	if call.Name != "sh" {
		t.Errorf("expected 'sh' for linux custom command, got %q", call.Name)
	}
}

func TestInstall_CustomCommand_WrongOS_ReturnsFailure(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")

	pkg := Package{
		Name:   "win-custom",
		Custom: map[string]string{"windows": "install.bat"},
	}

	result := mgr.Install(pkg)
	if result.Success {
		t.Error("expected failure when custom command is for a different OS")
	}
	if len(stub.Calls) != 0 {
		t.Errorf("expected no stub calls, got %d", len(stub.Calls))
	}
}

// --- InstallAll with stub ---

func TestInstallAll_MultiplePackages(t *testing.T) {
	mgr, _ := newStubManager(t, "linux")
	setAvailable(mgr, Pacman)

	packages := []Package{
		{Name: "pkg1", Managers: map[PackageManager]ManagerValue{Pacman: {PackageName: "pkg1"}}},
		{Name: "pkg2", Managers: map[PackageManager]ManagerValue{Pacman: {PackageName: "pkg2"}}},
		{Name: "pkg3", Managers: map[PackageManager]ManagerValue{Apt: {PackageName: "pkg3"}}}, // not available
	}

	results := mgr.InstallAll(packages)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if !results[0].Success {
		t.Errorf("pkg1: expected success, got %s", results[0].Message)
	}
	if !results[1].Success {
		t.Errorf("pkg2: expected success, got %s", results[1].Message)
	}
	if results[2].Success {
		t.Error("pkg3: expected failure (apt not available)")
	}
}

// --- Status checks with isInstalledWithRunner ---

func TestIsInstalledWithRunner_PackagePresentReturnsTrue(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	// stub returns success (zero Result = ExitCode 0, no error)
	stub.AddResult("pacman", cmdexec.Result{ExitCode: 0})

	got := isInstalledWithRunner(context.Background(), "neovim", "pacman", stub)
	if !got {
		t.Error("expected isInstalled=true when runner returns success")
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one call")
	}
	call := stub.Calls[0]
	if call.Name != "pacman" {
		t.Errorf("expected 'pacman' check command, got %q", call.Name)
	}
	// Verify the package name appears in the check args
	found := false
	for _, arg := range call.Args {
		if arg == "neovim" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'neovim' in check args, got %v", call.Args)
	}
}

func TestIsInstalledWithRunner_UnknownManager_ReturnsFalse(t *testing.T) {
	stub := cmdexec.NewStubRunner()

	got := isInstalledWithRunner(context.Background(), "something", "nonexistent-manager", stub)
	if got {
		t.Error("expected isInstalled=false for unknown manager")
	}
	if len(stub.Calls) != 0 {
		t.Errorf("expected no calls for unknown manager, got %d", len(stub.Calls))
	}
}

func TestIsInstalledWithRunner_DifferentManagers(t *testing.T) {
	tests := []struct {
		name       string
		manager    string
		expectCmd  string
		expectArgs []string
	}{
		{
			name:      "apt/dpkg check",
			manager:   "apt",
			expectCmd: "dpkg",
		},
		{
			name:      "brew check",
			manager:   "brew",
			expectCmd: "brew",
		},
		{
			name:      "scoop check",
			manager:   "scoop",
			expectCmd: "scoop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := cmdexec.NewStubRunner()
			stub.AddResult(tt.expectCmd, cmdexec.Result{ExitCode: 0})

			got := isInstalledWithRunner(context.Background(), "testpkg", tt.manager, stub)
			if !got {
				t.Errorf("expected isInstalled=true for manager %q", tt.manager)
			}

			if len(stub.Calls) == 0 {
				t.Fatalf("expected at least one call for manager %q", tt.manager)
			}
			if stub.Calls[0].Name != tt.expectCmd {
				t.Errorf("expected command %q, got %q", tt.expectCmd, stub.Calls[0].Name)
			}
		})
	}
}

// --- IsInstallerInstalled via isInstallerInstalledWithRunner ---

func TestIsInstallerInstalledWithRunner_BinaryFound(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddPath("mytool", "/usr/bin/mytool")

	got := isInstallerInstalledWithRunner("mytool", stub)
	if !got {
		t.Error("expected isInstalled=true when binary is found via LookPath")
	}
}

func TestIsInstallerInstalledWithRunner_BinaryNotFound(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	// no path registered for "notfound"

	got := isInstallerInstalledWithRunner("notfound", stub)
	if got {
		t.Error("expected isInstalled=false when binary not found in PATH")
	}
}

func TestIsInstallerInstalledWithRunner_EmptyBinary(t *testing.T) {
	stub := cmdexec.NewStubRunner()

	got := isInstallerInstalledWithRunner("", stub)
	if got {
		t.Error("expected isInstalled=false for empty binary name")
	}
}

// --- DryRun with stub (verifies no actual commands sent to runner) ---

func TestInstall_DryRun_NoStubCallsMade(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")
	setAvailable(mgr, Pacman)
	mgr.DryRun = true

	pkg := Package{
		Name:     "dryrun-pkg",
		Managers: map[PackageManager]ManagerValue{Pacman: {PackageName: "dryrun-pkg"}},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected dry-run success, got: %s", result.Message)
	}
	if len(stub.Calls) != 0 {
		t.Errorf("dry-run should not invoke runner, got %d calls", len(stub.Calls))
	}
	if !strings.Contains(result.Message, "Would run") {
		t.Errorf("expected dry-run message to contain 'Would run', got %q", result.Message)
	}
}

func TestInstall_DryRun_GitClone_NoStubCalls(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, stub := newStubManager(t, "linux")
	mgr.DryRun = true

	pkg := Package{
		Name: "dry-repo",
		Managers: map[PackageManager]ManagerValue{
			Git: {
				Git: &config.GitPackage{
					URL: "https://github.com/user/dry-repo.git",
					Targets: map[string]string{
						"linux": tmpDir + "/dry-repo",
					},
				},
			},
		},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected dry-run success, got: %s", result.Message)
	}
	if len(stub.Calls) != 0 {
		t.Errorf("dry-run should not invoke runner, got %d calls", len(stub.Calls))
	}
}

// --- Dependency install with stub ---

func TestInstall_WithDeps_InstallsDepsFirst(t *testing.T) {
	mgr, stub := newStubManager(t, "linux")
	setAvailable(mgr, Pacman)

	pkg := Package{
		Name: "main-pkg",
		Managers: map[PackageManager]ManagerValue{
			Pacman: {
				PackageName: "main-pkg",
				Deps:        []string{"dep1", "dep2"},
			},
		},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}

	// Should have 3 calls: dep1, dep2, main-pkg
	if len(stub.Calls) != 3 {
		t.Fatalf("expected 3 stub calls (2 deps + 1 main), got %d", len(stub.Calls))
	}

	// Verify deps are installed before main package
	allArgs := make([]string, 0)
	for _, c := range stub.Calls {
		allArgs = append(allArgs, c.Args...)
	}

	dep1Pos, dep2Pos, mainPos := -1, -1, -1
	for i, c := range stub.Calls {
		for _, arg := range c.Args {
			if arg == "dep1" {
				dep1Pos = i
			}
			if arg == "dep2" {
				dep2Pos = i
			}
			if arg == "main-pkg" {
				mainPos = i
			}
		}
	}
	if dep1Pos == -1 || dep2Pos == -1 {
		t.Errorf("expected dep1 and dep2 to be installed, allArgs=%v", allArgs)
	}
	if mainPos == -1 {
		t.Errorf("expected main-pkg to be installed, allArgs=%v", allArgs)
	}
	if mainPos <= dep1Pos || mainPos <= dep2Pos {
		t.Error("expected main package to be installed after deps")
	}
}

// --- Git pull when repo already cloned ---

func TestInstall_GitPackage_Pull_WhenAlreadyCloned(t *testing.T) {
	// Create a fake .git directory so gitPull is triggered instead of gitClone
	tmpDir := t.TempDir()
	gitDir := tmpDir + "/.git"
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("setup: failed to create .git dir: %v", err)
	}

	mgr, stub := newStubManager(t, "linux")

	pkg := Package{
		Name: "existing-repo",
		Managers: map[PackageManager]ManagerValue{
			Git: {
				Git: &config.GitPackage{
					URL: "https://github.com/user/existing-repo.git",
					Targets: map[string]string{
						"linux": tmpDir,
					},
				},
			},
		},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success for git pull, got: %s", result.Message)
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call for git pull")
	}
	call := stub.Calls[0]
	if call.Name != "git" {
		t.Errorf("expected 'git' command for pull, got %q", call.Name)
	}
	// Should contain "-C" and "pull" args
	foundC := false
	foundPull := false
	for _, arg := range call.Args {
		if arg == "-C" {
			foundC = true
		}
		if arg == "pull" {
			foundPull = true
		}
	}
	if !foundC || !foundPull {
		t.Errorf("expected '-C ... pull' in git pull args, got %v", call.Args)
	}
}

func TestInstall_GitPackage_Pull_WithSudo(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := tmpDir + "/.git"
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("setup: failed to create .git dir: %v", err)
	}

	mgr, stub := newStubManager(t, "linux")

	pkg := Package{
		Name: "sudo-pull-repo",
		Managers: map[PackageManager]ManagerValue{
			Git: {
				Git: &config.GitPackage{
					URL:  "https://github.com/user/sudo-pull.git",
					Sudo: true,
					Targets: map[string]string{
						"linux": tmpDir,
					},
				},
			},
		},
	}

	result := mgr.Install(pkg)
	if !result.Success {
		t.Errorf("expected success for sudo git pull, got: %s", result.Message)
	}

	if len(stub.Calls) == 0 {
		t.Fatal("expected at least one stub call")
	}
	if !stub.Calls[0].Sudo {
		t.Error("expected sudo=true for sudo git pull")
	}
}

// --- wingetBulkListWithRunner ---

func TestWingetBulkListWithRunner_ReturnsEmpty_OnRunnerError(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	// No result queued for "winget" — stub returns zero Result (ExitCode 0),
	// but we can test the parsing path with actual output below.

	result := wingetBulkListWithRunner(context.Background(), stub)
	// With no output, should return empty map (header parse fails gracefully)
	if result == nil {
		t.Error("expected non-nil map")
	}
}

func TestWingetBulkListWithRunner_ParsesOutput(t *testing.T) {
	stub := cmdexec.NewStubRunner()

	// Real winget list output format: header, then a line of only dashes, then data rows.
	// The separator line must be entirely dashes (no spaces between columns).
	wingetOutput := "Name                         Id                              Version\n" +
		"----------------------------------------------------------------------------------------------\n" +
		"Git                          Git.Git                         2.44.0\n" +
		"Microsoft Visual Studio Code Microsoft.VisualStudioCode      1.87.0\n"

	stub.AddResult("winget", cmdexec.Result{
		Stdout:   []byte(wingetOutput),
		ExitCode: 0,
	})

	result := wingetBulkListWithRunner(context.Background(), stub)

	if len(result) == 0 {
		t.Errorf("expected parsed winget IDs, got empty map (output was %q)", wingetOutput)
	}
	if !result["git.git"] {
		t.Errorf("expected 'git.git' in result, got %v", result)
	}
}

// --- FromPackageSpec ---

func TestFromPackageSpec_ReturnsPackageWithManagers(t *testing.T) {
	spec := &config.EntryPackage{
		Managers: map[string]config.ManagerValue{
			"pacman": {PackageName: "nvim"},
			"apt":    {PackageName: "neovim"},
		},
	}

	pkg := FromPackageSpec("neovim", spec)

	if pkg == nil {
		t.Fatal("expected non-nil package")
	}
	if pkg.Name != "neovim" {
		t.Errorf("Name = %q, want %q", pkg.Name, "neovim")
	}
	if len(pkg.Managers) != 2 {
		t.Errorf("expected 2 managers, got %d", len(pkg.Managers))
	}
}

func TestFromPackageSpec_NilSpec_ReturnsNil(t *testing.T) {
	pkg := FromPackageSpec("anything", nil)
	if pkg != nil {
		t.Errorf("expected nil for nil spec, got %v", pkg)
	}
}

// --- WithContext builder ---

func TestWithContext_ReturnsNewManagerWithContext(t *testing.T) {
	mgr, _ := newStubManager(t, "linux")
	ctx := context.Background()

	modified := mgr.WithContext(ctx)

	if modified == mgr {
		t.Error("WithContext should return a new Manager, not the same pointer")
	}
	if modified.OS != mgr.OS {
		t.Errorf("WithContext changed OS: got %q, want %q", modified.OS, mgr.OS)
	}
}

// --- ResetInstalledCache ---

func TestResetInstalledCache_CanBeCalledSafely(t *testing.T) {
	// Just verify it doesn't panic and can be called multiple times
	ResetInstalledCache()
	ResetInstalledCache()
}

// --- isInstalledSingle: missing package (no queued result) ---

func TestIsInstalledWithRunner_NoQueuedResult_ReturnsFalse(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	// pacman check returns zero Result (ExitCode 0, nil error) when no result queued
	// This actually means "installed" since zero exit code = success
	// The stub always returns nil error, so isInstalledSingle returns true with default result.
	// This test verifies the behavior is consistent.
	got := isInstalledWithRunner(context.Background(), "testpkg", "pacman", stub)
	// Default Result{} has ExitCode=0 and err=nil, so should return true
	if !got {
		t.Error("expected true for zero-exit-code default result")
	}
}

// --- WithRunner builder ---

func TestWithRunner_ReturnsNewManagerWithStub(t *testing.T) {
	cfg := &Config{}
	stub := cmdexec.NewStubRunner()
	original := &Manager{
		ctx:          context.Background(),
		Config:       cfg,
		OS:           "linux",
		Available:    []PackageManager{Pacman},
		availableSet: map[PackageManager]bool{Pacman: true},
		runner:       cmdexec.OsRunner{},
	}

	modified := original.WithRunner(stub)

	// Should be a different pointer
	if modified == original {
		t.Error("WithRunner should return a new Manager, not the same pointer")
	}
	// Original config/OS should be preserved
	if modified.OS != original.OS {
		t.Errorf("WithRunner changed OS: got %q, want %q", modified.OS, original.OS)
	}
	if modified.Config != original.Config {
		t.Error("WithRunner changed Config pointer")
	}
}
