package manager

import (
	"fmt"
	"strings"
)

// List displays all managed configuration entries with their current status.
func (m *Manager) List() error {
	fmt.Printf("Configuration paths for OS: %s\n\n", m.Platform.OS)

	// Check version
	if m.Config.Version == 3 {
		return m.listV3()
	}

	// v2 format - existing logic
	paths := m.GetPaths()

	for _, path := range paths {
		target := path.GetTarget(m.Platform.OS)

		var files string
		if path.IsFolder() {
			files = "[folder]"
		} else {
			files = strings.Join(path.Files, ", ")
		}

		backup := m.resolvePath(path.Backup)

		if target != "" {
			fmt.Printf("%-25s %s\n", path.Name+":", files)
			fmt.Printf("  backup: %s\n", backup)
			fmt.Printf("  target: %s\n\n", target)
		} else {
			fmt.Printf("%-25s %s (not applicable for %s)\n", path.Name+":", files, m.Platform.OS)
			fmt.Printf("  backup: %s\n\n", backup)
		}
	}

	// List git entries
	gitEntries := m.GetGitEntries()

	for _, entry := range gitEntries {
		target := entry.GetTarget(m.Platform.OS)

		if target != "" {
			fmt.Printf("%-25s [git]\n", entry.Name+":")
			fmt.Printf("  repo: %s\n", entry.Repo)

			if entry.Branch != "" {
				fmt.Printf("  branch: %s\n", entry.Branch)
			}

			fmt.Printf("  target: %s\n\n", target)
		} else {
			fmt.Printf("%-25s [git] (not applicable for %s)\n", entry.Name+":", m.Platform.OS)
			fmt.Printf("  repo: %s\n\n", entry.Repo)
		}
	}

	return nil
}

func (m *Manager) listV3() error {
	apps := m.GetApplications()

	for _, app := range apps {
		fmt.Printf("Application: %s\n", app.Name)

		if app.Description != "" {
			fmt.Printf("  %s\n", app.Description)
		}

		for _, entry := range app.Entries {
			target := entry.GetTarget(m.Platform.OS)
			if target == "" {
				continue
			}

			fmt.Printf("  ├─ %s [%s]\n", entry.Name, entry.Type)

			if entry.IsConfig() {
				var files string
				if entry.IsFolder() {
					files = "[folder]"
				} else {
					files = strings.Join(entry.Files, ", ")
				}

				fmt.Printf("     files: %s\n", files)
				fmt.Printf("     backup: %s\n", m.resolvePath(entry.Backup))
			} else if entry.IsGit() {
				fmt.Printf("     repo: %s\n", entry.Repo)

				if entry.Branch != "" {
					fmt.Printf("     branch: %s\n", entry.Branch)
				}
			}

			fmt.Printf("     target: %s\n", target)
		}

		if app.HasPackage() {
			fmt.Printf("  └─ package: %v\n", app.Package.Managers)
		}

		fmt.Println()
	}

	return nil
}
