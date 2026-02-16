package packages

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AntoineGS/tidydots/internal/platform"
)

// Install installs a single package using the best available method.
// It tries git packages first, then package managers (in order of availability),
// then custom commands, and finally URL-based installation. Returns an InstallResult
// indicating success or failure with a descriptive message.
func (m *Manager) Install(pkg Package) InstallResult {
	result := InstallResult{Package: pkg.Name}

	// Phase 1: Install dependencies across all managers
	if method, msg, ok := m.installDeps(pkg); !ok {
		result.Method = method
		result.Success = false
		result.Message = msg
		return result
	}

	// Check if this is a git package
	if gitValue, ok := pkg.Managers[Git]; ok && gitValue.IsGit() {
		result.Method = string(Git)
		success, msg := m.installGitPackage(*gitValue.Git)
		result.Success = success
		result.Message = msg
		return result
	}

	// Check if this is an installer package
	if installerValue, ok := pkg.Managers[Installer]; ok && installerValue.IsInstaller() {
		result.Method = string(Installer)
		success, msg := m.installInstallerPackage(*installerValue.Installer)
		result.Success = success
		result.Message = msg
		return result
	}

	// Try package managers
	if len(pkg.Managers) > 0 {
		for _, mgr := range m.Available {
			// Skip git and installer managers (already handled above)
			if mgr == Git || mgr == Installer {
				continue
			}

			if val, ok := pkg.Managers[mgr]; ok {
				result.Method = string(mgr)
				success, msg := m.installWithManager(mgr, val.PackageName)
				result.Success = success
				result.Message = msg

				return result
			}
		}
	}

	// Try custom command
	if cmd, ok := pkg.Custom[m.OS]; ok {
		result.Method = MethodCustom
		success, msg := m.runCustomCommand(cmd)
		result.Success = success
		result.Message = msg

		return result
	}

	// Try URL install
	if urlInstall, ok := pkg.URL[m.OS]; ok {
		result.Method = MethodURL
		success, msg := m.installFromURL(urlInstall)
		result.Success = success
		result.Message = msg

		return result
	}

	result.Success = false
	result.Message = "No installation method available for this OS/system"

	return result
}

// installDeps installs all dependencies for a package across its managers.
// It skips managers that are not available on the current system.
// It returns the manager method, an error message, and false if any dependency fails.
// Returns ("", "", true) if all dependencies installed successfully.
func (m *Manager) installDeps(pkg Package) (string, string, bool) {
	for mgr, val := range pkg.Managers {
		if len(val.Deps) == 0 {
			continue
		}
		// Skip git and installer - they don't have traditional deps
		if mgr == Git || mgr == Installer {
			continue
		}
		// Skip managers not available on this system
		if !m.availableSet[mgr] {
			continue
		}
		for _, dep := range val.Deps {
			success, msg := m.installWithManager(mgr, dep)
			if !success {
				return string(mgr), fmt.Sprintf("Dependency %s failed: %s", dep, msg), false
			}
		}
	}

	return "", "", true
}

// InstallAll installs all packages in the provided slice sequentially.
// It returns a slice of InstallResult, one for each package, indicating
// the success or failure of each installation.
func (m *Manager) InstallAll(packages []Package) []InstallResult {
	results := make([]InstallResult, 0, len(packages))
	for _, pkg := range packages {
		results = append(results, m.Install(pkg))
	}

	return results
}

func (m *Manager) installWithManager(mgr PackageManager, pkgName string) (bool, string) {
	mc, ok := managerCmds[mgr]
	if !ok {
		if mgr == Git {
			return false, "Git packages should be installed via installGitPackage"
		}

		if mgr == Installer {
			return false, "Installer packages should be installed via installInstallerPackage"
		}

		return false, fmt.Sprintf("Unknown package manager: %s", mgr)
	}

	args := expandArgs(mc.install, pkgName)
	cmd := exec.CommandContext(m.ctx, args[0], args[1:]...) //nolint:gosec // args from trusted lookup table

	if m.DryRun {
		return true, fmt.Sprintf("Would run: %s", strings.Join(cmd.Args, " "))
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return false, fmt.Sprintf("Installation failed: %v", err)
	}

	return true, fmt.Sprintf("Installed via %s", mgr)
}

// installGitPackage clones or updates a git repository.
func (m *Manager) installGitPackage(gitCfg GitConfig) (bool, string) {
	// Get target path for current OS
	targetPath, ok := gitCfg.Targets[m.OS]
	if !ok {
		return false, fmt.Sprintf("No git target path defined for OS: %s", m.OS)
	}

	// Expand path (handle ~)
	if strings.HasPrefix(targetPath, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, fmt.Sprintf("Failed to get home directory: %v", err)
		}
		targetPath = filepath.Join(home, targetPath[1:])
	}

	// Check if already cloned
	gitDir := filepath.Join(targetPath, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return m.gitPull(targetPath, gitCfg.Sudo)
	}

	return m.gitClone(gitCfg.URL, targetPath, gitCfg.Branch, gitCfg.Sudo)
}

func (m *Manager) gitClone(repoURL, targetPath, branch string, sudo bool) (bool, string) {
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "-b", branch)
	}
	args = append(args, repoURL, targetPath)

	var cmd *exec.Cmd
	if sudo {
		// Prepend sudo to the command
		args = append([]string{"git"}, args...)
		cmd = exec.CommandContext(m.ctx, "sudo", args...)
	} else {
		cmd = exec.CommandContext(m.ctx, "git", args...)
	}

	if m.DryRun {
		return true, fmt.Sprintf("Would run: %s", strings.Join(cmd.Args, " "))
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return false, fmt.Sprintf("Git clone failed: %v", err)
	}

	return true, "Repository cloned successfully"
}

func (m *Manager) gitPull(repoPath string, sudo bool) (bool, string) {
	var cmd *exec.Cmd
	if sudo {
		cmd = exec.CommandContext(m.ctx, "sudo", "git", "-C", repoPath, "pull")
	} else {
		cmd = exec.CommandContext(m.ctx, "git", "-C", repoPath, "pull")
	}

	if m.DryRun {
		return true, fmt.Sprintf("Would run: %s", strings.Join(cmd.Args, " "))
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return false, fmt.Sprintf("Git pull failed: %v", err)
	}

	return true, "Repository updated successfully"
}

// installInstallerPackage runs an OS-specific shell command to install a package.
// SECURITY NOTE: This intentionally executes arbitrary shell commands from the
// user's configuration file. Users should only use configurations they trust,
// as malicious configs could execute harmful commands.
func (m *Manager) installInstallerPackage(cfg InstallerConfig) (bool, string) {
	command, ok := cfg.Command[m.OS]
	if !ok {
		return false, fmt.Sprintf("No installer command defined for OS: %s", m.OS)
	}

	if m.DryRun {
		return true, fmt.Sprintf("Would run: %s", command)
	}

	var cmd *exec.Cmd
	if m.OS == platform.OSWindows {
		cmd = exec.CommandContext(m.ctx, "powershell", "-Command", command) //nolint:gosec // intentional install command from user config
	} else {
		cmd = exec.CommandContext(m.ctx, "sh", "-c", command) //nolint:gosec // intentional install command from user config
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return false, fmt.Sprintf("Installer command failed: %v", err)
	}

	return true, "Installed via installer"
}

// runCustomCommand executes a custom shell command from the configuration.
// SECURITY NOTE: This intentionally executes arbitrary shell commands from the
// user's configuration file. Users should only use configurations they trust,
// as malicious configs could execute harmful commands.
func (m *Manager) runCustomCommand(command string) (bool, string) {
	if m.DryRun {
		return true, fmt.Sprintf("Would run: %s", command)
	}

	var cmd *exec.Cmd
	if m.OS == platform.OSWindows {
		cmd = exec.CommandContext(m.ctx, "powershell", "-Command", command)
	} else {
		cmd = exec.CommandContext(m.ctx, "sh", "-c", command)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return false, fmt.Sprintf("Custom command failed: %v", err)
	}

	return true, "Installed via custom command"
}

// installFromURL downloads a file from a URL and runs an install command.
// SECURITY NOTE: This intentionally downloads and executes content from URLs
// specified in the user's configuration file. Users should only use configurations
// they trust, as malicious configs could download and execute harmful code.
func (m *Manager) installFromURL(urlInstall URLInstall) (bool, string) {
	if m.DryRun {
		return true, fmt.Sprintf("Would download %s and run: %s", urlInstall.URL, urlInstall.Command)
	}

	// Create temp directory to avoid TOCTOU race on the file path
	tmpDir, err := os.MkdirTemp("", "tidydots-*")
	if err != nil {
		return false, fmt.Sprintf("Failed to create temp directory: %v", err)
	}

	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			// Log but don't fail on cleanup errors
			fmt.Printf("[WARN] Failed to remove temp directory %s: %v\n", tmpDir, err)
		}
	}()

	tmpPath := filepath.Join(tmpDir, "installer")

	// Download file
	var downloadCmd *exec.Cmd

	if m.OS == platform.OSWindows {
		// Escape single quotes in URL and path for PowerShell
		escapedURL := strings.ReplaceAll(urlInstall.URL, "'", "''")
		escapedPath := strings.ReplaceAll(tmpPath, "'", "''")
		downloadCmd = exec.CommandContext(m.ctx, "powershell", "-Command", //nolint:gosec // intentional download command
			fmt.Sprintf("Invoke-WebRequest -Uri '%s' -OutFile '%s'", escapedURL, escapedPath))
	} else {
		downloadCmd = exec.CommandContext(m.ctx, "curl", "-fsSL", "-o", tmpPath, urlInstall.URL) //nolint:gosec // intentional download command
	}

	if err := downloadCmd.Run(); err != nil {
		return false, fmt.Sprintf("Download failed: %v", err)
	}

	// Make executable on Unix
	if m.OS != platform.OSWindows {
		if err := os.Chmod(tmpPath, ExecPerms); err != nil { //nolint:gosec // installer scripts need to be executable
			return false, fmt.Sprintf("Failed to make executable: %v", err)
		}
	}

	// Run install command
	command := strings.ReplaceAll(urlInstall.Command, "{file}", tmpPath)

	var installCmd *exec.Cmd
	if m.OS == platform.OSWindows {
		installCmd = exec.CommandContext(m.ctx, "powershell", "-Command", command) //nolint:gosec // intentional install command
	} else {
		installCmd = exec.CommandContext(m.ctx, "sh", "-c", command) //nolint:gosec // intentional install command
	}

	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	installCmd.Stdin = os.Stdin

	if err := installCmd.Run(); err != nil {
		return false, fmt.Sprintf("Install command failed: %v", err)
	}

	return true, "Installed via URL"
}
