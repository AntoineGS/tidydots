package platform

import (
	"os"
	"runtime"
	"testing"

	"github.com/AntoineGS/tidydots/internal/cmdexec"
	"github.com/AntoineGS/tidydots/internal/fsys"
)

// --- detectDistroWithFS tests ---

func TestDetectDistroWithFS_Arch(t *testing.T) {
	t.Parallel()

	mem := fsys.NewMemFS()
	if err := mem.MkdirAll("/etc", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	osRelease := "NAME=\"Arch Linux\"\nPRETTY_NAME=\"Arch Linux\"\nID=arch\nBUILD_ID=rolling\n"
	if err := mem.WriteFile("/etc/os-release", []byte(osRelease), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	distro := detectDistroWithFS(mem)
	if distro != "arch" {
		t.Errorf("detectDistroWithFS() = %q, want %q", distro, "arch")
	}
}

func TestDetectDistroWithFS_Ubuntu(t *testing.T) {
	t.Parallel()

	mem := fsys.NewMemFS()
	if err := mem.MkdirAll("/etc", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	osRelease := "NAME=\"Ubuntu\"\nVERSION=\"22.04 LTS\"\nID=ubuntu\nID_LIKE=debian\n"
	if err := mem.WriteFile("/etc/os-release", []byte(osRelease), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	distro := detectDistroWithFS(mem)
	if distro != "ubuntu" {
		t.Errorf("detectDistroWithFS() = %q, want %q", distro, "ubuntu")
	}
}

func TestDetectDistroWithFS_Fedora(t *testing.T) {
	t.Parallel()

	mem := fsys.NewMemFS()
	if err := mem.MkdirAll("/etc", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	osRelease := "NAME=\"Fedora Linux\"\nVERSION=\"38\"\nID=fedora\nVERSION_ID=38\n"
	if err := mem.WriteFile("/etc/os-release", []byte(osRelease), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	distro := detectDistroWithFS(mem)
	if distro != "fedora" {
		t.Errorf("detectDistroWithFS() = %q, want %q", distro, "fedora")
	}
}

func TestDetectDistroWithFS_Debian(t *testing.T) {
	t.Parallel()

	mem := fsys.NewMemFS()
	if err := mem.MkdirAll("/etc", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	osRelease := "NAME=\"Debian GNU/Linux\"\nVERSION=\"12 (bookworm)\"\nID=debian\n"
	if err := mem.WriteFile("/etc/os-release", []byte(osRelease), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	distro := detectDistroWithFS(mem)
	if distro != "debian" {
		t.Errorf("detectDistroWithFS() = %q, want %q", distro, "debian")
	}
}

func TestDetectDistroWithFS_QuotedID(t *testing.T) {
	t.Parallel()

	mem := fsys.NewMemFS()
	if err := mem.MkdirAll("/etc", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Some distros quote their ID value
	osRelease := "NAME=\"Some Distro\"\nID=\"opensuse-leap\"\nVERSION_ID=\"15.5\"\n"
	if err := mem.WriteFile("/etc/os-release", []byte(osRelease), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	distro := detectDistroWithFS(mem)
	if distro != "opensuse-leap" {
		t.Errorf("detectDistroWithFS() = %q, want %q", distro, "opensuse-leap")
	}
}

func TestDetectDistroWithFS_MissingFile(t *testing.T) {
	t.Parallel()

	mem := fsys.NewMemFS()
	// No /etc/os-release written

	distro := detectDistroWithFS(mem)
	if distro != "" {
		t.Errorf("detectDistroWithFS() = %q, want empty string for missing file", distro)
	}
}

func TestDetectDistroWithFS_MalformedContent(t *testing.T) {
	t.Parallel()

	mem := fsys.NewMemFS()
	if err := mem.MkdirAll("/etc", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Malformed content with no ID= line
	osRelease := "THIS IS NOT VALID\nNAME_NO_EQUALS arch\nSOMETHING=else\n"
	if err := mem.WriteFile("/etc/os-release", []byte(osRelease), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Should not panic and should return empty since there is no ID= line
	distro := detectDistroWithFS(mem)
	if distro != "" {
		t.Errorf("detectDistroWithFS() = %q, want empty string for malformed content", distro)
	}
}

func TestDetectDistroWithFS_EmptyFile(t *testing.T) {
	t.Parallel()

	mem := fsys.NewMemFS()
	if err := mem.MkdirAll("/etc", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := mem.WriteFile("/etc/os-release", []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	distro := detectDistroWithFS(mem)
	if distro != "" {
		t.Errorf("detectDistroWithFS() = %q, want empty string for empty file", distro)
	}
}

// --- isCommandAvailableWithRunner tests ---

func TestIsCommandAvailableWithRunner_Found(t *testing.T) {
	t.Parallel()

	stub := cmdexec.NewStubRunner()
	stub.AddPath("git", "/usr/bin/git")

	if !isCommandAvailableWithRunner("git", stub) {
		t.Error("isCommandAvailableWithRunner() = false, want true for registered command")
	}
}

func TestIsCommandAvailableWithRunner_NotFound(t *testing.T) {
	t.Parallel()

	stub := cmdexec.NewStubRunner()
	// "apt" is not registered

	if isCommandAvailableWithRunner("apt", stub) {
		t.Error("isCommandAvailableWithRunner() = true, want false for unregistered command")
	}
}

func TestIsCommandAvailableWithRunner_MultipleCommands(t *testing.T) {
	t.Parallel()

	stub := cmdexec.NewStubRunner()
	stub.AddPath("pacman", "/usr/bin/pacman")
	stub.AddPath("git", "/usr/bin/git")

	if !isCommandAvailableWithRunner("pacman", stub) {
		t.Error("expected pacman to be found")
	}

	if !isCommandAvailableWithRunner("git", stub) {
		t.Error("expected git to be found")
	}

	if isCommandAvailableWithRunner("brew", stub) {
		t.Error("brew should not be found")
	}
}

// --- DetectAvailableManagersWithRunner tests ---

func TestDetectAvailableManagersWithRunner_Some(t *testing.T) {
	ResetAvailableManagersCache()
	detectedOS = "" // allow all managers through the OS filter

	stub := cmdexec.NewStubRunner()
	stub.AddPath("pacman", "/usr/bin/pacman")
	stub.AddPath("git", "/usr/bin/git")
	// apt, yay, brew, etc. not registered — LookPath returns error

	managers := DetectAvailableManagersWithRunner(stub)

	hasPacman := false
	hasGit := false
	hasApt := false

	for _, m := range managers {
		switch m {
		case "pacman":
			hasPacman = true
		case "git":
			hasGit = true
		case "apt":
			hasApt = true
		}
	}

	if !hasPacman {
		t.Error("expected pacman to be detected")
	}

	if !hasGit {
		t.Error("expected git to be detected")
	}

	if hasApt {
		t.Error("apt should NOT be detected")
	}
}

func TestDetectAvailableManagersWithRunner_None(t *testing.T) {
	ResetAvailableManagersCache()
	detectedOS = "" // allow all managers through the OS filter

	stub := cmdexec.NewStubRunner()
	// No paths registered

	managers := DetectAvailableManagersWithRunner(stub)
	if len(managers) != 0 {
		t.Errorf("expected no managers, got %v", managers)
	}
}

func TestDetectAvailableManagersWithRunner_AllKnown(t *testing.T) {
	ResetAvailableManagersCache()
	detectedOS = "" // allow all managers through the OS filter

	stub := cmdexec.NewStubRunner()
	for _, mgr := range KnownPackageManagers {
		stub.AddPath(mgr, "/usr/bin/"+mgr)
	}

	managers := DetectAvailableManagersWithRunner(stub)

	if len(managers) != len(KnownPackageManagers) {
		t.Errorf("expected %d managers, got %d: %v", len(KnownPackageManagers), len(managers), managers)
	}
}

func TestDetectAvailableManagersWithRunner_LinuxOSFiltering(t *testing.T) {
	ResetAvailableManagersCache()
	detectedOS = OSLinux

	stub := cmdexec.NewStubRunner()
	// Register all managers as available
	for _, mgr := range KnownPackageManagers {
		stub.AddPath(mgr, "/usr/bin/"+mgr)
	}

	managers := DetectAvailableManagersWithRunner(stub)

	// Windows-only managers should be excluded
	for _, m := range managers {
		if m == "winget" || m == "scoop" || m == "choco" {
			t.Errorf("manager %q should not appear when OS is Linux", m)
		}
	}

	// Linux managers should be included
	hasPackman := false
	for _, m := range managers {
		if m == "pacman" {
			hasPackman = true
		}
	}

	if !hasPackman {
		t.Error("expected pacman to appear when OS is Linux")
	}
}

// --- Platform struct and With* method tests ---

func TestPlatform_WithOS_ImmutableCopy(t *testing.T) {
	t.Parallel()

	original := &Platform{
		OS:      OSLinux,
		Distro:  "arch",
		EnvVars: map[string]string{"key": "value"},
	}

	modified := original.WithOS(OSWindows)

	if original.OS != OSLinux {
		t.Errorf("original OS was mutated to %q", original.OS)
	}

	if modified.OS != OSWindows {
		t.Errorf("WithOS() returned platform with OS = %q, want %q", modified.OS, OSWindows)
	}

	if modified.Distro != "arch" {
		t.Errorf("WithOS() did not preserve Distro, got %q", modified.Distro)
	}

	// EnvVars should be an independent copy
	modified.EnvVars["new"] = "entry"
	if _, exists := original.EnvVars["new"]; exists {
		t.Error("modifying new platform's EnvVars affected the original")
	}
}

func TestPlatform_WithHostname(t *testing.T) {
	t.Parallel()

	p := &Platform{
		OS:       OSLinux,
		Hostname: "oldhost",
		EnvVars:  map[string]string{},
	}

	newP := p.WithHostname("newhost")

	if p.Hostname != "oldhost" {
		t.Errorf("original Hostname was mutated to %q", p.Hostname)
	}

	if newP.Hostname != "newhost" {
		t.Errorf("WithHostname() = %q, want %q", newP.Hostname, "newhost")
	}
}

func TestPlatform_WithUser(t *testing.T) {
	t.Parallel()

	p := &Platform{
		OS:      OSLinux,
		User:    "alice",
		EnvVars: map[string]string{},
	}

	newP := p.WithUser("bob")

	if p.User != "alice" {
		t.Errorf("original User was mutated to %q", p.User)
	}

	if newP.User != "bob" {
		t.Errorf("WithUser() = %q, want %q", newP.User, "bob")
	}
}

func TestPlatform_WithDistro(t *testing.T) {
	t.Parallel()

	p := &Platform{
		OS:      OSLinux,
		Distro:  "arch",
		EnvVars: map[string]string{},
	}

	newP := p.WithDistro("ubuntu")

	if p.Distro != "arch" {
		t.Errorf("original Distro was mutated to %q", p.Distro)
	}

	if newP.Distro != "ubuntu" {
		t.Errorf("WithDistro() = %q, want %q", newP.Distro, "ubuntu")
	}
}

func TestPlatform_IsArchLinux(t *testing.T) {
	t.Parallel()

	tests := []struct {
		distro string
		want   bool
	}{
		{"arch", true},
		{"ubuntu", false},
		{"fedora", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.distro, func(t *testing.T) {
			t.Parallel()

			p := &Platform{Distro: tt.distro}
			if got := p.IsArchLinux(); got != tt.want {
				t.Errorf("IsArchLinux() = %v, want %v for distro %q", got, tt.want, tt.distro)
			}
		})
	}
}

// --- detectPowerShellProfileWithRunner tests ---

func TestDetectPowerShellProfileWithRunner_Success(t *testing.T) {
	t.Parallel()

	stub := cmdexec.NewStubRunner()
	stub.AddResult("pwsh", cmdexec.Result{
		Stdout: []byte("C:\\Users\\user\\Documents\\PowerShell\\Microsoft.PowerShell_profile.ps1\n"),
	})

	p := &Platform{EnvVars: make(map[string]string)}
	p.detectPowerShellProfileWithRunner(stub)

	if p.EnvVars["PWSH_PROFILE"] == "" {
		t.Error("PWSH_PROFILE was not set")
	}

	if p.EnvVars["PWSH_PROFILE_FILE"] == "" {
		t.Error("PWSH_PROFILE_FILE was not set")
	}

	if p.EnvVars["PWSH_PROFILE_PATH"] == "" {
		t.Error("PWSH_PROFILE_PATH was not set")
	}
}

func TestDetectPowerShellProfileWithRunner_EmptyOutput(t *testing.T) {
	t.Parallel()

	stub := cmdexec.NewStubRunner()
	stub.AddResult("pwsh", cmdexec.Result{
		Stdout: []byte("   \n"),
	})

	p := &Platform{EnvVars: make(map[string]string)}
	p.detectPowerShellProfileWithRunner(stub)

	if len(p.EnvVars) != 0 {
		t.Errorf("expected no EnvVars set for empty output, got %v", p.EnvVars)
	}
}

func TestDetectPowerShellProfileWithRunner_NoResult(t *testing.T) {
	t.Parallel()

	stub := cmdexec.NewStubRunner()
	// No result queued — StubRunner returns zero Result without error

	p := &Platform{EnvVars: make(map[string]string)}
	p.detectPowerShellProfileWithRunner(stub)

	if _, ok := p.EnvVars["PWSH_PROFILE"]; ok {
		t.Error("PWSH_PROFILE should not be set when output is empty")
	}
}

// --- lookPathSkipWindowsDrives tests ---

func TestLookPathSkipWindowsDrives_EmptyPATH(t *testing.T) {
	t.Setenv("PATH", "")

	found := lookPathSkipWindowsDrives("git", []string{"/mnt/c"})
	if found {
		t.Error("expected false when PATH is empty")
	}
}

func TestLookPathSkipWindowsDrives_AllMounted(t *testing.T) {
	// All PATH entries are under Windows drive mounts — should skip all
	t.Setenv("PATH", "/mnt/c/Windows/System32:/mnt/d/bin")

	found := lookPathSkipWindowsDrives("git", []string{"/mnt/c", "/mnt/d"})
	if found {
		t.Error("expected false when all PATH entries are under Windows drive mounts")
	}
}

func TestLookPathSkipWindowsDrives_NotFound(t *testing.T) {
	// PATH has a real non-mounted directory but the command doesn't exist there
	t.Setenv("PATH", "/tmp")

	found := lookPathSkipWindowsDrives("definitely-not-a-real-command-xyz", []string{"/mnt/c"})
	if found {
		t.Error("expected false for nonexistent command")
	}
}

func TestLookPathSkipWindowsDrives_Found(t *testing.T) {
	// lookPathSkipWindowsDrives only runs under WSL (it reads /proc/mounts), and
	// both it and this test assume Unix PATH syntax: ':' as the list separator
	// and '/' as the path separator. On Windows t.TempDir() returns C:\..., which
	// a ':' split would tear in half.
	if runtime.GOOS == OSWindows {
		t.Skip("lookPathSkipWindowsDrives is WSL-only and assumes Unix PATH syntax")
	}

	// Create a temp directory with a fake executable and add it to PATH
	tmpDir := t.TempDir()
	fakeBin := tmpDir + "/fakecmd"

	// Write a real file using os package (lookPathSkipWindowsDrives uses os.Stat)
	f, err := os.Create(fakeBin) //nolint:gosec
	if err != nil {
		t.Fatalf("create fake binary: %v", err)
	}
	f.Close()

	t.Setenv("PATH", tmpDir+":/mnt/c/bin")

	found := lookPathSkipWindowsDrives("fakecmd", []string{"/mnt/c"})
	if !found {
		t.Error("expected true when command exists in non-mounted PATH dir")
	}
}

// --- SetDetectionHints tests ---

func TestSetDetectionHints_Linux(t *testing.T) {
	SetDetectionHints(OSLinux, false)

	if detectedOS != OSLinux {
		t.Errorf("detectedOS = %q, want %q", detectedOS, OSLinux)
	}

	if detectedWSL {
		t.Error("detectedWSL should be false after SetDetectionHints with isWSL=false")
	}
}

func TestSetDetectionHints_Windows(t *testing.T) {
	SetDetectionHints(OSWindows, false)

	if detectedOS != OSWindows {
		t.Errorf("detectedOS = %q, want %q", detectedOS, OSWindows)
	}
}
