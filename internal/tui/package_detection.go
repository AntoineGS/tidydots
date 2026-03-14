package tui

import (
	"context"
	"time"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/packages"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// packageCheckTimeout is the maximum time allowed for a package status check
// command to complete. This prevents the TUI from hanging indefinitely when
// a package manager is locked or a sudo prompt is waiting.
const packageCheckTimeout = 10 * time.Second

// isPackageInstalledFromPackage checks if a package is installed using the packages package
func isPackageInstalledFromPackage(pkg *config.EntryPackage, method, entryName, osType string) bool {
	if pkg == nil {
		return false
	}

	// Handle installer packages via binary PATH lookup
	if method == TypeInstaller {
		if val, ok := pkg.Managers[method]; ok && val.IsInstaller() {
			return packages.IsInstallerInstalled(val.Installer.Binary)
		}
		return false
	}

	// Handle git packages via target directory check
	if method == TypeGit {
		if val, ok := pkg.Managers[method]; ok && val.IsGit() {
			return packages.IsGitInstalled(val.Git.Targets, osType)
		}
		return false
	}

	// Get the package name for the detected manager
	pkgName := ""
	if val, ok := pkg.Managers[method]; ok {
		// Skip git and installer packages
		if !val.IsGit() && !val.IsInstaller() {
			pkgName = val.PackageName
		}
	} else {
		// For custom/url methods, use the entry name
		pkgName = entryName
	}

	ctx, cancel := context.WithTimeout(context.Background(), packageCheckTimeout)
	defer cancel()

	return packages.IsInstalled(ctx, pkgName, method)
}

// getPackageInstallMethodFromPackage determines how a package would be installed
func getPackageInstallMethodFromPackage(pkg *config.EntryPackage, osType string) string {
	if pkg == nil {
		return TypeNone
	}

	// Check package managers
	availableManagers := detectAvailableManagers()
	for _, mgr := range availableManagers {
		if _, ok := pkg.Managers[mgr]; ok {
			return mgr
		}
	}
	// Check installer (always available when configured with a command for current OS)
	if val, ok := pkg.Managers[TypeInstaller]; ok && val.IsInstaller() {
		if _, hasCmd := val.Installer.Command[osType]; hasCmd {
			return TypeInstaller
		}
	}
	// Check custom
	if _, ok := pkg.Custom[osType]; ok {
		return "custom"
	}
	// Check URL
	if _, ok := pkg.URL[osType]; ok {
		return "url"
	}

	return TypeNone
}

func detectAvailableManagers() []string {
	return platform.DetectAvailableManagers()
}
