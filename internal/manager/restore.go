package manager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/AntoineGS/dot-manager/internal/config"
)

func (m *Manager) Restore() error {
	m.log("Restoring configurations for OS: %s", m.Platform.OS)

	// Check config version
	if m.Config.Version == 3 {
		return m.restoreV3()
	}

	// v2 format - existing logic
	entries := m.GetEntries()

	for _, entry := range entries {
		target := entry.GetTarget(m.Platform.OS)
		if target == "" {
			m.logVerbose("Skipping %s: no target for OS %s", entry.Name, m.Platform.OS)
			continue
		}

		if err := m.restoreEntry(entry, target); err != nil {
			m.log("Error restoring %s: %v", entry.Name, err)
		}
	}

	// Restore git entries (clones)
	gitEntries := m.GetGitEntries()
	for _, entry := range gitEntries {
		target := entry.GetTarget(m.Platform.OS)
		if target == "" {
			m.logVerbose("Skipping git entry %s: no target for OS %s", entry.Name, m.Platform.OS)
			continue
		}

		if err := m.restoreGitEntry(entry, target); err != nil {
			m.log("Error restoring git entry %s: %v", entry.Name, err)
		}
	}

	return nil
}

func (m *Manager) restoreEntry(entry config.Entry, target string) error {
	backupPath := m.resolvePath(entry.Backup)

	if entry.IsFolder() {
		return m.restoreFolder(entry, backupPath, target)
	}
	return m.restoreFiles(entry, backupPath, target)
}

func (m *Manager) restoreFolder(entry config.Entry, source, target string) error {
	// Skip if already a symlink
	if isSymlink(target) {
		m.logVerbose("Already a symlink: %s", target)
		return nil
	}

	// Check if we need to adopt: target exists but backup doesn't
	if !pathExists(source) && pathExists(target) {
		m.log("Adopting folder %s -> %s", target, source)
		if !m.DryRun {
			// Create backup parent directory
			backupParent := filepath.Dir(source)
			if !pathExists(backupParent) {
				if err := os.MkdirAll(backupParent, 0755); err != nil {
					return fmt.Errorf("creating backup parent directory: %w", err)
				}
			}
			// Move target to backup location
			if entry.Sudo {
				cmd := exec.Command("sudo", "mv", target, source)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("adopting folder (moving to backup): %w", err)
				}
			} else {
				if err := os.Rename(target, source); err != nil {
					return fmt.Errorf("adopting folder (moving to backup): %w", err)
				}
			}
		}
	}

	// Now check if backup exists
	if !pathExists(source) {
		m.logVerbose("Source folder does not exist: %s", source)
		return nil
	}

	// Create parent directory if it doesn't exist
	parentDir := filepath.Dir(target)
	if !pathExists(parentDir) {
		m.log("Creating directory %s", parentDir)
		if !m.DryRun {
			if entry.Sudo {
				cmd := exec.Command("sudo", "mkdir", "-p", parentDir)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("creating parent directory: %w", err)
				}
			} else {
				if err := os.MkdirAll(parentDir, 0755); err != nil {
					return fmt.Errorf("creating parent directory: %w", err)
				}
			}
		}
	}

	// Remove existing folder (if still there after adopt check)
	if pathExists(target) && !isSymlink(target) {
		m.log("Removing folder %s", target)
		if !m.DryRun {
			if entry.Sudo {
				cmd := exec.Command("sudo", "rm", "-rf", target)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("removing existing folder: %w", err)
				}
			} else {
				if err := removeAll(target); err != nil {
					return fmt.Errorf("removing existing folder: %w", err)
				}
			}
		}
	}

	m.log("Creating symlink %s -> %s", target, source)
	if !m.DryRun {
		return createSymlink(source, target, entry.Sudo)
	}
	return nil
}

func (m *Manager) restoreFiles(entry config.Entry, source, target string) error {
	// Create backup directory if it doesn't exist (needed for adopting)
	if !pathExists(source) {
		if !m.DryRun {
			if err := os.MkdirAll(source, 0755); err != nil {
				return fmt.Errorf("creating backup directory: %w", err)
			}
		}
	}

	// Create target directory if it doesn't exist
	if !pathExists(target) {
		m.log("Creating directory %s", target)
		if !m.DryRun {
			if entry.Sudo {
				cmd := exec.Command("sudo", "mkdir", "-p", target)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("creating target directory: %w", err)
				}
			} else {
				if err := os.MkdirAll(target, 0755); err != nil {
					return fmt.Errorf("creating target directory: %w", err)
				}
			}
		}
	}

	for _, file := range entry.Files {
		srcFile := filepath.Join(source, file)
		dstFile := filepath.Join(target, file)

		// Skip if already a symlink
		if isSymlink(dstFile) {
			m.logVerbose("Already a symlink: %s", dstFile)
			continue
		}

		// Check if we need to adopt: target exists but backup doesn't
		if !pathExists(srcFile) && pathExists(dstFile) {
			m.log("Adopting file %s -> %s", dstFile, srcFile)
			if !m.DryRun {
				// Move target file to backup location
				if entry.Sudo {
					cmd := exec.Command("sudo", "mv", dstFile, srcFile)
					if err := cmd.Run(); err != nil {
						return fmt.Errorf("adopting file (moving to backup): %w", err)
					}
				} else {
					if err := os.Rename(dstFile, srcFile); err != nil {
						// If rename fails (cross-device), try copy and delete
						if err := copyFile(dstFile, srcFile); err != nil {
							return fmt.Errorf("adopting file (copying to backup): %w", err)
						}
						if err := os.Remove(dstFile); err != nil {
							return fmt.Errorf("adopting file (removing original): %w", err)
						}
					}
				}
			}
		}

		if !pathExists(srcFile) {
			m.logVerbose("Source file does not exist: %s", srcFile)
			continue
		}

		// Remove existing file (if still there after adopt check)
		if pathExists(dstFile) && !isSymlink(dstFile) {
			m.log("Removing file %s", dstFile)
			if !m.DryRun {
				if entry.Sudo {
					cmd := exec.Command("sudo", "rm", "-f", dstFile)
					if err := cmd.Run(); err != nil {
						return fmt.Errorf("removing existing file: %w", err)
					}
				} else {
					if err := os.Remove(dstFile); err != nil {
						return fmt.Errorf("removing existing file: %w", err)
					}
				}
			}
		}

		m.log("Creating symlink %s -> %s", dstFile, srcFile)
		if !m.DryRun {
			if err := createSymlink(srcFile, dstFile, entry.Sudo); err != nil {
				return fmt.Errorf("creating symlink: %w", err)
			}
		}
	}

	return nil
}

func createSymlink(source, target string, useSudo bool) error {
	// Validate source exists
	if _, err := os.Stat(source); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("symlink source does not exist: %s", source)
		}
		return fmt.Errorf("cannot access symlink source %s: %w", source, err)
	}

	if runtime.GOOS == "windows" {
		// Check if source is a directory
		info, err := os.Stat(source)
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Use mklink /J for directory junctions on Windows
			cmd := exec.Command("cmd", "/c", "mklink", "/J", target, source)
			return cmd.Run()
		}
		// Use mklink for files
		cmd := exec.Command("cmd", "/c", "mklink", target, source)
		return cmd.Run()
	}

	if useSudo {
		cmd := exec.Command("sudo", "ln", "-s", source, target)
		return cmd.Run()
	}
	return os.Symlink(source, target)
}

// restoreGitEntry clones or updates a git repository
func (m *Manager) restoreGitEntry(entry config.Entry, target string) error {
	if pathExists(target) {
		// Check if it's a git repository
		gitDir := filepath.Join(target, ".git")
		if pathExists(gitDir) {
			m.log("Updating git repo %s at %s...", entry.Name, target)
			if !m.DryRun {
				var cmd *exec.Cmd
				if entry.Sudo {
					cmd = exec.Command("sudo", "git", "-C", target, "pull")
				} else {
					cmd = exec.Command("git", "-C", target, "pull")
				}
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("failed to update %s: %w", entry.Name, err)
				}
				m.log("[ok] %s updated successfully", entry.Name)
			}
			return nil
		}
		// Target exists but is not a git repo - skip
		m.logVerbose("Target %s exists but is not a git repository, skipping", target)
		return nil
	}

	// Clone the repository
	m.log("Cloning %s to %s...", entry.Name, target)
	if !m.DryRun {
		parentDir := filepath.Dir(target)
		if !pathExists(parentDir) {
			if entry.Sudo {
				mkdirCmd := exec.Command("sudo", "mkdir", "-p", parentDir)
				if err := mkdirCmd.Run(); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", parentDir, err)
				}
			} else {
				if err := os.MkdirAll(parentDir, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", parentDir, err)
				}
			}
		}

		args := []string{"clone"}
		if entry.Branch != "" {
			args = append(args, "-b", entry.Branch)
		}
		args = append(args, entry.Repo, target)

		var cmd *exec.Cmd
		if entry.Sudo {
			cmd = exec.Command("sudo", append([]string{"git"}, args...)...)
		} else {
			cmd = exec.Command("git", args...)
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to clone %s: %w", entry.Name, err)
		}
		m.log("[ok] %s cloned successfully", entry.Name)
	}

	return nil
}

func (m *Manager) restoreV3() error {
	apps := m.GetApplications()

	for _, app := range apps {
		m.log("Restoring application: %s", app.Name)

		for _, subEntry := range app.Entries {
			target := subEntry.GetTarget(m.Platform.OS)
			if target == "" {
				m.logVerbose("Skipping %s/%s: no target for OS %s", app.Name, subEntry.Name, m.Platform.OS)
				continue
			}

			if subEntry.IsConfig() {
				if err := m.restoreSubEntry(app.Name, subEntry, target); err != nil {
					m.log("Error restoring %s/%s: %v", app.Name, subEntry.Name, err)
				}
			} else if subEntry.IsGit() {
				if err := m.restoreGitSubEntry(app.Name, subEntry, target); err != nil {
					m.log("Error restoring git %s/%s: %v", app.Name, subEntry.Name, err)
				}
			}
		}
	}

	return nil
}

func (m *Manager) restoreSubEntry(appName string, subEntry config.SubEntry, target string) error {
	backupPath := m.resolvePath(subEntry.Backup)

	if subEntry.IsFolder() {
		return m.restoreFolderSubEntry(appName, subEntry, backupPath, target)
	}
	return m.restoreFilesSubEntry(appName, subEntry, backupPath, target)
}

func (m *Manager) restoreFolderSubEntry(appName string, subEntry config.SubEntry, source, target string) error {
	// Similar to restoreFolder but use subEntry fields
	if isSymlink(target) {
		m.logVerbose("Already a symlink: %s", target)
		return nil
	}

	if !pathExists(source) && pathExists(target) {
		m.log("Adopting folder %s -> %s", target, source)
		if !m.DryRun {
			backupParent := filepath.Dir(source)
			if !pathExists(backupParent) {
				if err := os.MkdirAll(backupParent, 0755); err != nil {
					return fmt.Errorf("creating backup parent directory: %w", err)
				}
			}
			if subEntry.Sudo {
				cmd := exec.Command("sudo", "mv", target, source)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("adopting folder (moving to backup): %w", err)
				}
			} else {
				if err := os.Rename(target, source); err != nil {
					return fmt.Errorf("adopting folder (moving to backup): %w", err)
				}
			}
		}
	}

	if !pathExists(source) {
		m.logVerbose("Source folder does not exist: %s", source)
		return nil
	}

	parentDir := filepath.Dir(target)
	if !pathExists(parentDir) {
		m.log("Creating directory %s", parentDir)
		if !m.DryRun {
			if subEntry.Sudo {
				cmd := exec.Command("sudo", "mkdir", "-p", parentDir)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("creating parent directory: %w", err)
				}
			} else {
				if err := os.MkdirAll(parentDir, 0755); err != nil {
					return fmt.Errorf("creating parent directory: %w", err)
				}
			}
		}
	}

	if pathExists(target) && !isSymlink(target) {
		m.log("Removing folder %s", target)
		if !m.DryRun {
			if subEntry.Sudo {
				cmd := exec.Command("sudo", "rm", "-rf", target)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("removing existing folder: %w", err)
				}
			} else {
				if err := removeAll(target); err != nil {
					return fmt.Errorf("removing existing folder: %w", err)
				}
			}
		}
	}

	m.log("Creating symlink %s -> %s", target, source)
	if !m.DryRun {
		return createSymlink(source, target, subEntry.Sudo)
	}
	return nil
}

func (m *Manager) restoreFilesSubEntry(appName string, subEntry config.SubEntry, source, target string) error {
	// Similar to restoreFiles but use subEntry fields
	if !pathExists(source) {
		if !m.DryRun {
			if err := os.MkdirAll(source, 0755); err != nil {
				return fmt.Errorf("creating backup directory: %w", err)
			}
		}
	}

	if !pathExists(target) {
		m.log("Creating directory %s", target)
		if !m.DryRun {
			if subEntry.Sudo {
				cmd := exec.Command("sudo", "mkdir", "-p", target)
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("creating target directory: %w", err)
				}
			} else {
				if err := os.MkdirAll(target, 0755); err != nil {
					return fmt.Errorf("creating target directory: %w", err)
				}
			}
		}
	}

	for _, file := range subEntry.Files {
		srcFile := filepath.Join(source, file)
		dstFile := filepath.Join(target, file)

		if isSymlink(dstFile) {
			m.logVerbose("Already a symlink: %s", dstFile)
			continue
		}

		if !pathExists(srcFile) && pathExists(dstFile) {
			m.log("Adopting file %s -> %s", dstFile, srcFile)
			if !m.DryRun {
				if subEntry.Sudo {
					cmd := exec.Command("sudo", "mv", dstFile, srcFile)
					if err := cmd.Run(); err != nil {
						return fmt.Errorf("adopting file (moving to backup): %w", err)
					}
				} else {
					if err := os.Rename(dstFile, srcFile); err != nil {
						if err := copyFile(dstFile, srcFile); err != nil {
							return fmt.Errorf("adopting file (copying to backup): %w", err)
						}
						if err := os.Remove(dstFile); err != nil {
							return fmt.Errorf("adopting file (removing original): %w", err)
						}
					}
				}
			}
		}

		if !pathExists(srcFile) {
			m.logVerbose("Source file does not exist: %s", srcFile)
			continue
		}

		if pathExists(dstFile) && !isSymlink(dstFile) {
			m.log("Removing file %s", dstFile)
			if !m.DryRun {
				if subEntry.Sudo {
					cmd := exec.Command("sudo", "rm", "-f", dstFile)
					if err := cmd.Run(); err != nil {
						return fmt.Errorf("removing existing file: %w", err)
					}
				} else {
					if err := os.Remove(dstFile); err != nil {
						return fmt.Errorf("removing existing file: %w", err)
					}
				}
			}
		}

		m.log("Creating symlink %s -> %s", dstFile, srcFile)
		if !m.DryRun {
			if err := createSymlink(srcFile, dstFile, subEntry.Sudo); err != nil {
				return fmt.Errorf("creating symlink: %w", err)
			}
		}
	}

	return nil
}

func (m *Manager) restoreGitSubEntry(appName string, subEntry config.SubEntry, target string) error {
	// Similar to restoreGitEntry but use subEntry fields
	if pathExists(target) {
		gitDir := filepath.Join(target, ".git")
		if pathExists(gitDir) {
			m.log("Updating git repo %s/%s at %s...", appName, subEntry.Name, target)
			if !m.DryRun {
				var cmd *exec.Cmd
				if subEntry.Sudo {
					cmd = exec.Command("sudo", "git", "-C", target, "pull")
				} else {
					cmd = exec.Command("git", "-C", target, "pull")
				}
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					return fmt.Errorf("failed to update %s/%s: %w", appName, subEntry.Name, err)
				}
				m.log("[ok] %s/%s updated successfully", appName, subEntry.Name)
			}
			return nil
		}
		m.logVerbose("Target %s exists but is not a git repository, skipping", target)
		return nil
	}

	m.log("Cloning %s/%s to %s...", appName, subEntry.Name, target)
	if !m.DryRun {
		parentDir := filepath.Dir(target)
		if !pathExists(parentDir) {
			if subEntry.Sudo {
				mkdirCmd := exec.Command("sudo", "mkdir", "-p", parentDir)
				if err := mkdirCmd.Run(); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", parentDir, err)
				}
			} else {
				if err := os.MkdirAll(parentDir, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", parentDir, err)
				}
			}
		}

		args := []string{"clone"}
		if subEntry.Branch != "" {
			args = append(args, "-b", subEntry.Branch)
		}
		args = append(args, subEntry.Repo, target)

		var cmd *exec.Cmd
		if subEntry.Sudo {
			cmd = exec.Command("sudo", append([]string{"git"}, args...)...)
		} else {
			cmd = exec.Command("git", args...)
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to clone %s/%s: %w", appName, subEntry.Name, err)
		}
		m.log("[ok] %s/%s cloned successfully", appName, subEntry.Name)
	}

	return nil
}
