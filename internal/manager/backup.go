package manager

import (
	"fmt"
	"path/filepath"

	"github.com/AntoineGS/dot-manager/internal/config"
)

func (m *Manager) Backup() error {
	m.log("Backing up configurations for OS: %s (root: %v)", m.Platform.OS, m.Platform.IsRoot)

	paths := m.GetPaths()

	for _, path := range paths {
		target := path.GetTarget(m.Platform.OS)
		if target == "" {
			m.logVerbose("Skipping %s: no target for OS %s", path.Name, m.Platform.OS)
			continue
		}

		if err := m.backupPath(path, target); err != nil {
			m.log("Error backing up %s: %v", path.Name, err)
		}
	}

	return nil
}

func (m *Manager) backupPath(spec config.PathSpec, source string) error {
	backupPath := m.resolvePath(spec.Backup)

	if spec.IsFolder() {
		return m.backupFolder(spec.Name, source, backupPath)
	}
	return m.backupFiles(spec.Name, spec.Files, source, backupPath)
}

func (m *Manager) backupFolder(name, source, backup string) error {
	if !pathExists(source) {
		m.logVerbose("Source folder does not exist: %s", source)
		return nil
	}

	// Skip symlinks - they point to our backup already
	if isSymlink(source) {
		m.logVerbose("Skipping symlink: %s", source)
		return nil
	}

	m.log("Backing up folder %s to %s", source, backup)
	if !m.DryRun {
		// Copy source folder into backup directory (e.g., /source/config -> /backup/config)
		destPath := filepath.Join(backup, filepath.Base(source))
		if err := copyDir(source, destPath); err != nil {
			return fmt.Errorf("copying folder: %w", err)
		}
	}
	return nil
}

func (m *Manager) backupFiles(name string, files []string, source, backup string) error {
	for _, file := range files {
		srcFile := filepath.Join(source, file)
		dstFile := filepath.Join(backup, file)

		if !pathExists(srcFile) {
			m.logVerbose("Source file does not exist: %s", srcFile)
			continue
		}

		// Skip symlinks
		if isSymlink(srcFile) {
			m.logVerbose("Skipping symlink: %s", srcFile)
			continue
		}

		m.log("Backing up file %s to %s", srcFile, dstFile)
		if !m.DryRun {
			if err := copyFile(srcFile, dstFile); err != nil {
				return fmt.Errorf("copying file: %w", err)
			}
		}
	}

	return nil
}
