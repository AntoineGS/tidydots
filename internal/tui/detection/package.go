package detection

import (
	"context"
	"time"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/packages"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/tui/tuishared"
)

// PackageCheckTimeout is the maximum time allowed for a package status check
// command to complete. This prevents the TUI from hanging indefinitely when
// a package manager is locked or a sudo prompt is waiting.
const PackageCheckTimeout = 10 * time.Second

// IsPackageInstalled checks the selected main package and every dependency in
// the installation plan using the packages package.
func IsPackageInstalled(pkg *config.EntryPackage, plan packages.InstallationPlan, entryName, osType string) bool {
	if pkg == nil || plan.Method == packages.MethodNone {
		return false
	}

	if !isPackageMethodInstalled(pkg, plan.Method, entryName, osType) {
		return false
	}

	for _, dependency := range plan.Dependencies {
		if err := packages.ValidatePackageName(dependency.Name); err != nil {
			return false
		}
		if !isNativePackageInstalled(dependency.Name, string(dependency.Manager)) {
			return false
		}
	}

	return true
}

func isPackageMethodInstalled(pkg *config.EntryPackage, method, entryName, osType string) bool {
	// Handle installer packages via binary PATH lookup
	if method == tuishared.TypeInstaller {
		if val, ok := pkg.Managers[method]; ok && val.IsInstaller() {
			return packages.IsInstallerInstalled(val.Installer.Binary)
		}
		return false
	}

	// Handle git packages via target directory check
	if method == tuishared.TypeGit {
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

	return isNativePackageInstalled(pkgName, method)
}

func isNativePackageInstalled(pkgName, method string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), PackageCheckTimeout)
	defer cancel()

	return packages.IsInstalled(ctx, pkgName, method)
}

// GetPackageInstallPlan determines how a package would be installed and which
// dependencies would run before the selected main method. Callers with
// repository preferences should pass them so status and execution agree.
func GetPackageInstallPlan(pkg *config.EntryPackage, osType string, preferences *packages.Config) packages.InstallationPlan {
	if pkg == nil {
		return packages.InstallationPlan{Method: packages.MethodNone}
	}

	available := make([]packages.PackageManager, 0)
	for _, mgr := range DetectAvailableManagers() {
		available = append(available, packages.PackageManager(mgr))
	}
	converted := packages.FromPackageSpec("", pkg)
	if converted == nil {
		return packages.InstallationPlan{Method: packages.MethodNone}
	}
	return packages.PlanInstallation(*converted, preferences, osType, available)
}

// GetPackageInstallMethod determines how a package would be installed. Callers
// with repository preferences should pass them so status and execution agree.
func GetPackageInstallMethod(pkg *config.EntryPackage, osType string, preferences *packages.Config) string {
	return GetPackageInstallPlan(pkg, osType, preferences).Method
}

// DetectAvailableManagers returns the list of package managers available on the current system.
func DetectAvailableManagers() []string {
	return platform.DetectAvailableManagers()
}
