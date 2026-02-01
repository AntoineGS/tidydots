package platform

import (
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
)

const (
	OSLinux   = "linux"
	OSWindows = "windows"
)

type Platform struct {
	OS       string
	IsRoot   bool
	IsArch   bool
	EnvVars  map[string]string
}

func Detect() *Platform {
	p := &Platform{
		OS:      detectOS(),
		EnvVars: make(map[string]string),
	}

	if p.OS == OSLinux {
		p.IsRoot = detectRoot()
		p.IsArch = detectArchLinux()
	}

	if p.OS == OSWindows {
		p.detectPowerShellProfile()
	}

	return p
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

func detectRoot() bool {
	u, err := user.Current()
	if err != nil {
		return false
	}
	return u.Uid == "0"
}

func detectArchLinux() bool {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "ID=") {
			id := strings.TrimPrefix(line, "ID=")
			id = strings.Trim(id, "\"")
			return id == "arch"
		}
	}

	return false
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

func (p *Platform) WithOS(osType string) *Platform {
	newP := *p
	newP.OS = osType
	return &newP
}

// IsCommandAvailable checks if a command is available in PATH
func IsCommandAvailable(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// KnownPackageManagers returns the list of supported package managers
var KnownPackageManagers = []string{
	"yay", "paru", "pacman", // Arch Linux
	"apt", "dnf", "brew", // Debian/Fedora/macOS
	"winget", "scoop", "choco", // Windows
}

// DetectAvailableManagers returns a list of package managers available on the system
func DetectAvailableManagers() []string {
	var available []string
	for _, mgr := range KnownPackageManagers {
		if IsCommandAvailable(mgr) {
			available = append(available, mgr)
		}
	}
	return available
}
