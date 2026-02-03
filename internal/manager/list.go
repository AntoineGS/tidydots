package manager

import (
	"fmt"
	"strings"
)

// List displays all managed configuration entries with their current status.
func (m *Manager) List() error {
	fmt.Printf("Configuration paths for OS: %s\n\n", m.Platform.OS)

	apps := m.GetApplications()

	for _, app := range apps {
		fmt.Printf("Application: %s\n", app.Name)

		if app.Description != "" {
			fmt.Printf("  %s\n", app.Description)
		}

		for _, entry := range app.Entries {
			if !entry.IsConfig() {
				continue
			}

			target := entry.GetTarget(m.Platform.OS)
			if target == "" {
				continue
			}

			fmt.Printf("├─ %s [config]\n", entry.Name)

			var files string
			if entry.IsFolder() {
				files = "[folder]"
			} else {
				files = strings.Join(entry.Files, ", ")
			}

			fmt.Printf("     files: %s\n", files)
			fmt.Printf("     backup: %s\n", m.resolvePath(entry.Backup))
			fmt.Printf("     target: %s\n", target)
		}

		if app.HasPackage() {
			fmt.Printf("  └─ package: %v\n", app.Package.Managers)
		}

		fmt.Println()
	}

	return nil
}
