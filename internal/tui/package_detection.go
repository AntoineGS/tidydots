package tui

import (
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/packages"
	"github.com/AntoineGS/tidydots/internal/tui/detection"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

// isPackageInstalledFromPackage checks a package and its installation-plan
// dependencies using the packages package.
func isPackageInstalledFromPackage(
	pkg *config.EntryPackage,
	plan packages.InstallationPlan,
	entryName, osType string,
) bool {
	return detection.IsPackageInstalled(pkg, plan, entryName, osType)
}

func (m Model) packageConfigForRole(role workspace.Role) *packages.Config {
	cfg := &packages.Config{}
	ownerConfig := m.Config
	if role != "" && m.Workspace != nil {
		ownerConfig = m.configForRole(role)
	}
	if ownerConfig != nil {
		cfg.DefaultManager = packages.PackageManager(ownerConfig.DefaultManager)
		for _, mgr := range ownerConfig.ManagerPriority {
			cfg.ManagerPriority = append(cfg.ManagerPriority, packages.PackageManager(mgr))
		}
	}
	return cfg
}
