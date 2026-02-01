package manager

import (
	"fmt"
	"strings"
)

func (m *Manager) List() error {
	fmt.Printf("Configuration paths for OS: %s (root: %v)\n\n", m.Platform.OS, m.Platform.IsRoot)

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

	return nil
}
