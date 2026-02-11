package packages

import (
	"context"
	"os/exec"

	"github.com/AntoineGS/tidydots/internal/platform"
)

// IsInstalled checks if a package is installed on the system.
// It uses the appropriate package manager query command based on the installation method.
// Returns true if the package is installed, false otherwise.
func IsInstalled(ctx context.Context, pkgName string, manager string) bool {
	mc, ok := managerCmds[PackageManager(manager)]
	if !ok {
		// For git, custom, url methods, we can't easily check installation status
		return false
	}

	args := expandArgs(mc.check, pkgName)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // args from trusted lookup table

	// Run silently - just check exit code
	cmd.Stdout = nil
	cmd.Stderr = nil
	err := cmd.Run()

	return err == nil
}

// IsInstallerInstalled checks if an installer package is installed by looking up
// the binary name in PATH. Returns false if no binary name is configured.
func IsInstallerInstalled(binary string) bool {
	if binary == "" {
		return false
	}

	return platform.IsCommandAvailable(binary)
}

// CanInstall checks if a package can be installed on this system.
// It returns true if any of the package's installation methods (manager,
// custom command, or URL) are available for the current OS and package managers.
func (m *Manager) CanInstall(pkg Package) bool {
	// Check managers
	for _, mgr := range m.Available {
		if _, ok := pkg.Managers[mgr]; ok {
			return true
		}
	}
	// Check installer (always available when configured with a command for current OS)
	if val, ok := pkg.Managers[Installer]; ok && val.IsInstaller() {
		if _, hasCmd := val.Installer.Command[m.OS]; hasCmd {
			return true
		}
	}
	// Check custom
	if _, ok := pkg.Custom[m.OS]; ok {
		return true
	}
	// Check URL
	if _, ok := pkg.URL[m.OS]; ok {
		return true
	}

	return false
}

// GetInstallMethod returns the method that would be used to install a package.
// It returns the name of the first available package manager, "installer" for
// installer packages, "custom" if a custom command is available, "url" for
// URL-based installation, or "none" if no installation method is available.
func (m *Manager) GetInstallMethod(pkg Package) string {
	for _, mgr := range m.Available {
		if _, ok := pkg.Managers[mgr]; ok {
			return string(mgr)
		}
	}

	// Check installer (always available when configured with a command for current OS)
	if val, ok := pkg.Managers[Installer]; ok && val.IsInstaller() {
		if _, hasCmd := val.Installer.Command[m.OS]; hasCmd {
			return string(Installer)
		}
	}

	if _, ok := pkg.Custom[m.OS]; ok {
		return MethodCustom
	}

	if _, ok := pkg.URL[m.OS]; ok {
		return MethodURL
	}

	return "none"
}

// GetInstallablePackages returns packages from the configuration that can be
// installed on this system. It filters the configured packages to only those
// with at least one available installation method.
func (m *Manager) GetInstallablePackages() []Package {
	result := make([]Package, 0, len(m.Config.Packages))

	for _, pkg := range m.Config.Packages {
		if m.CanInstall(pkg) {
			result = append(result, pkg)
		}
	}

	return result
}
