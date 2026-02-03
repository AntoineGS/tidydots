package platform

import (
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
)

// Supported operating system identifiers.
const (
	// OSLinux represents Linux operating systems
	OSLinux = "linux"
	// OSWindows represents Windows operating systems
	OSWindows = "windows"
)

// Platform holds detected platform information including the operating system,
// Linux distribution, hostname, current user, and privilege status.
type Platform struct {
	EnvVars  map[string]string
	OS       string
	Distro   string
	Hostname string
	User     string
	IsArch   bool
}

// Detect detects the current platform characteristics including OS type,
// Linux distribution (if applicable), hostname, current user, and root status.
func Detect() *Platform {
	p := &Platform{
		OS:       detectOS(),
		Hostname: detectHostname(),
		User:     detectUser(),
		EnvVars:  make(map[string]string),
	}

	if p.OS == OSLinux {
		p.Distro = detectDistro()
		p.IsArch = p.Distro == "arch"
	}

	if p.OS == OSWindows {
		p.detectPowerShellProfile()
	}

	return p
}

// detectDistro returns the Linux distribution ID from /etc/os-release
// Returns values like "arch", "ubuntu", "fedora", "debian", etc.
func detectDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		slog.Debug("failed to read /etc/os-release", "error", err)
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "ID=") {
			id := strings.TrimPrefix(line, "ID=")
			id = strings.Trim(id, "\"")

			return id
		}
	}

	return ""
}

func detectHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		slog.Debug("failed to detect hostname", "error", err)
		return ""
	}

	return hostname
}

func detectUser() string {
	u, err := user.Current()
	if err != nil {
		slog.Debug("failed to detect current user", "error", err)
		return ""
	}

	return u.Username
}

func detectOS() string {
	if runtime.GOOS == "windows" {
		return OSWindows
	}

	// Also check OS environment variable (for cross-platform scripts)
	osEnv := os.Getenv("OS")
	if strings.Contains(strings.ToLower(osEnv), "windows") {
		return OSWindows
	}

	return OSLinux
}

func (p *Platform) detectPowerShellProfile() {
	cmd := exec.Command("pwsh", "-NoProfile", "-Command", "echo $PROFILE")

	output, err := cmd.Output()
	if err != nil {
		return
	}

	profile := strings.TrimSpace(string(output))
	if profile != "" {
		p.EnvVars["PWSH_PROFILE"] = profile
		p.EnvVars["PWSH_PROFILE_FILE"] = getBasename(profile)
		p.EnvVars["PWSH_PROFILE_PATH"] = getDirname(profile)
	}
}

func getBasename(path string) string {
	// Handle both Unix and Windows separators
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}

	return path
}

func getDirname(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}

	return "."
}

// WithOS returns a copy of the Platform with the OS field overridden.
func (p *Platform) WithOS(osType string) *Platform {
	newP := *p
	newP.OS = osType

	return &newP
}

// WithHostname returns a copy of the Platform with the Hostname field overridden.
func (p *Platform) WithHostname(hostname string) *Platform {
	newP := *p
	newP.Hostname = hostname

	return &newP
}

// WithUser returns a copy of the Platform with the User field overridden.
func (p *Platform) WithUser(username string) *Platform {
	newP := *p
	newP.User = username

	return &newP
}

// WithDistro returns a copy of the Platform with the Distro field overridden.
func (p *Platform) WithDistro(distro string) *Platform {
	newP := *p
	newP.Distro = distro

	return &newP
}

// IsCommandAvailable checks if a command is available in PATH
func IsCommandAvailable(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// KnownPackageManagers is the list of supported package managers across all platforms.
// Includes Arch Linux (yay, paru, pacman), Debian/Fedora/macOS (apt, dnf, brew),
// and Windows (winget, scoop, choco) package managers.
var KnownPackageManagers = []string{
	"yay", "paru", "pacman", // Arch Linux
	"apt", "dnf", "brew", // Debian/Fedora/macOS
	"winget", "scoop", "choco", // Windows
}

// DetectAvailableManagers returns a list of package managers available on the system
// by checking which managers from KnownPackageManagers are present in the PATH.
func DetectAvailableManagers() []string {
	available := make([]string, 0, len(KnownPackageManagers))

	for _, mgr := range KnownPackageManagers {
		if IsCommandAvailable(mgr) {
			available = append(available, mgr)
		}
	}

	return available
}
