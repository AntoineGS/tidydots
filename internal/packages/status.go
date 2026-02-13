package packages

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"sync"

	"github.com/AntoineGS/tidydots/internal/platform"
)

// installedCache holds the lazily-populated set of installed package IDs for
// managers that support bulk listing (see managerCmd.bulkList).
// The cache is populated once per manager on the first IsInstalled call.
var installedCache sync.Map // map[string]*bulkCacheEntry

type bulkCacheEntry struct {
	once       sync.Once
	installedIDs map[string]bool // lowercase ID → true
}

// IsInstalled checks if a package is installed on the system.
// For managers with bulk list support, it runs a single list command and caches
// the results. For other managers, it runs the per-package check command.
// Returns true if the package is installed, false otherwise.
func IsInstalled(ctx context.Context, pkgName string, manager string) bool {
	mc, ok := managerCmds[PackageManager(manager)]
	if !ok {
		slog.Debug("no check command for manager, assuming not installed",
			slog.String("package", pkgName),
			slog.String("manager", manager))
		return false
	}

	// Managers with bulk list support: run one command, cache all installed IDs
	if mc.bulkList != nil {
		return isInstalledBulk(ctx, pkgName, manager, mc)
	}

	return isInstalledSingle(ctx, pkgName, manager, mc)
}

// isInstalledBulk checks installation via cached bulk list output.
func isInstalledBulk(ctx context.Context, pkgName, manager string, mc managerCmd) bool {
	val, _ := installedCache.LoadOrStore(manager, &bulkCacheEntry{})
	entry := val.(*bulkCacheEntry)

	entry.once.Do(func() {
		entry.installedIDs = mc.bulkList(ctx)
	})

	found := entry.installedIDs[strings.ToLower(pkgName)]
	if found {
		slog.Debug("package detected as installed (bulk cache)",
			slog.String("package", pkgName),
			slog.String("manager", manager))
	} else {
		slog.Debug("package not found in bulk cache",
			slog.String("package", pkgName),
			slog.String("manager", manager))
	}

	return found
}

// isInstalledSingle checks installation by running the per-package check command.
func isInstalledSingle(ctx context.Context, pkgName, manager string, mc managerCmd) bool {
	args := expandArgs(mc.check, pkgName)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // args from trusted lookup table

	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		slog.Debug("package check command failed",
			slog.String("package", pkgName),
			slog.String("manager", manager),
			slog.String("command", strings.Join(args, " ")),
			slog.String("error", err.Error()),
			slog.String("stderr", errMsg))
		return false
	}

	slog.Debug("package detected as installed",
		slog.String("package", pkgName),
		slog.String("manager", manager))

	return true
}

// ResetInstalledCache clears the bulk installed cache, causing the next
// IsInstalled call to re-query. Useful for tests and after install operations.
func ResetInstalledCache() {
	installedCache = sync.Map{}
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
