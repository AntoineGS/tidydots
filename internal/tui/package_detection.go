package tui

import (
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/tui/detection"
)

// isPackageInstalledFromPackage checks if a package is installed using the packages package.
func isPackageInstalledFromPackage(pkg *config.EntryPackage, method, entryName, osType string) bool {
	return detection.IsPackageInstalled(pkg, method, entryName, osType)
}

// getPackageInstallMethodFromPackage determines how a package would be installed.
func getPackageInstallMethodFromPackage(pkg *config.EntryPackage, osType string) string {
	return detection.GetPackageInstallMethod(pkg, osType)
}
