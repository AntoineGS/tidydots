// Package packages provides multi-package-manager support.
package packages

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// File permissions constants
const (
	// ExecPerms are the permissions for executable files (rwxr-xr-x)
	// Owner: read, write, execute; Group: read, execute; Other: read, execute
	ExecPerms os.FileMode = 0755
)

// PackageManager represents a supported package manager identifier.
// It is used to specify which package manager should be used for installing
// a package, such as pacman, apt, brew, winget, etc. The supported values
// are defined as constants (Pacman, Yay, Paru, Apt, Dnf, Brew, Winget, Scoop, Choco).
type PackageManager string

// Supported package manager identifiers.
const (
	// Pacman is the Arch Linux package manager
	Pacman PackageManager = "pacman"
	// Yay is an AUR helper for Arch Linux
	Yay PackageManager = "yay"
	// Paru is an AUR helper for Arch Linux
	Paru PackageManager = "paru"
	// Apt is the Debian/Ubuntu package manager
	Apt PackageManager = "apt"
	// Dnf is the Fedora package manager
	Dnf PackageManager = "dnf"
	// Brew is the macOS package manager
	Brew PackageManager = "brew"
	// Winget is the Windows package manager
	Winget PackageManager = "winget"
	// Scoop is a Windows package manager
	Scoop PackageManager = "scoop"
	// Choco is the Chocolatey Windows package manager
	Choco PackageManager = "choco"
	// Git is the git package manager for repository clones
	Git PackageManager = "git"
	// Installer is the installer package manager for shell command-based installation
	Installer PackageManager = "installer"
)

// Install method identifiers for non-manager methods.
const (
	// MethodCustom identifies a custom shell command install method
	MethodCustom = "custom"
	// MethodURL identifies a URL-based download and install method
	MethodURL = "url"
)

// GitConfig represents git-specific package configuration.
// It contains the repository URL, optional branch, and OS-specific clone destinations.
type GitConfig struct {
	URL     string            `yaml:"url"`
	Branch  string            `yaml:"branch,omitempty"`
	Targets map[string]string `yaml:"targets"`
	Sudo    bool              `yaml:"sudo,omitempty"`
}

// InstallerConfig represents installer-specific package configuration.
// It contains OS-specific shell commands and an optional binary name for install checks.
type InstallerConfig struct {
	Command map[string]string `yaml:"command"`
	Binary  string            `yaml:"binary,omitempty"`
}

// ManagerValue represents a typed value for a package manager entry.
// It holds either a package name string (for traditional managers like pacman, apt),
// a GitConfig (for git repositories), or an InstallerConfig (for shell command-based installation).
type ManagerValue struct {
	PackageName string
	Git         *GitConfig
	Installer   *InstallerConfig
	Deps        []string
}

// IsGit returns true if this manager value represents a git package configuration.
func (v ManagerValue) IsGit() bool { return v.Git != nil }

// IsInstaller returns true if this manager value represents an installer package configuration.
func (v ManagerValue) IsInstaller() bool { return v.Installer != nil }

// Package represents a package to install with multiple installation methods.
// A package can be installed via a package manager (Managers), a custom shell
// command (Custom), or by downloading from a URL (URL). The installation method
// is selected based on availability, with package managers tried first, then
// custom commands, and finally URL-based installation. A `when` expression can
// conditionally include the package based on template variables.
type Package struct {
	Name        string                          `yaml:"name"`
	Description string                          `yaml:"description,omitempty"`
	Managers    map[PackageManager]ManagerValue `yaml:"managers,omitempty"`
	Custom      map[string]string               `yaml:"custom,omitempty"` // OS -> command
	URL         map[string]URLInstall           `yaml:"url,omitempty"`    // OS -> URL install
	When        string                          `yaml:"when,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshaling for Package.
// It handles the Managers field specially to support both string values (for traditional
// package managers) and GitConfig structs (for git repositories).
func (p *Package) UnmarshalYAML(node *yaml.Node) error {
	// Define a temporary struct that matches Package but with a different Managers type
	type packageAlias struct {
		Name        string                `yaml:"name"`
		Description string                `yaml:"description,omitempty"`
		Managers    map[string]yaml.Node  `yaml:"managers,omitempty"`
		Custom      map[string]string     `yaml:"custom,omitempty"`
		URL         map[string]URLInstall `yaml:"url,omitempty"`
		When        string                `yaml:"when,omitempty"`
	}

	var alias packageAlias
	if err := node.Decode(&alias); err != nil {
		return err
	}

	// Copy simple fields
	p.Name = alias.Name
	p.Description = alias.Description
	p.Custom = alias.Custom
	p.URL = alias.URL
	p.When = alias.When

	// Process managers map
	p.Managers = make(map[PackageManager]ManagerValue)
	for key, valueNode := range alias.Managers {
		pm := PackageManager(key)

		switch pm { //nolint:exhaustive // default handles all traditional string-based managers
		case Git:
			var gitCfg GitConfig
			if err := valueNode.Decode(&gitCfg); err != nil {
				return fmt.Errorf("failed to decode git config: %w", err)
			}
			p.Managers[pm] = ManagerValue{Git: &gitCfg}

		case Installer:
			var installerCfg InstallerConfig
			if err := valueNode.Decode(&installerCfg); err != nil {
				return fmt.Errorf("failed to decode installer config: %w", err)
			}
			p.Managers[pm] = ManagerValue{Installer: &installerCfg}

		default:
			// Try string first (backward compat)
			var pkgName string
			if err := valueNode.Decode(&pkgName); err == nil {
				p.Managers[pm] = ManagerValue{PackageName: pkgName}
				continue
			}

			// Try object with name/deps
			type nativeManagerObj struct {
				Name string   `yaml:"name"`
				Deps []string `yaml:"deps"`
			}

			var obj nativeManagerObj
			if err := valueNode.Decode(&obj); err != nil {
				return fmt.Errorf("failed to decode manager %s: expected string or object with name/deps: %w", key, err)
			}

			p.Managers[pm] = ManagerValue{PackageName: obj.Name, Deps: obj.Deps}
		}
	}

	return nil
}

// URLInstall represents installation from a URL with download and command execution.
// The URL field specifies where to download the installer file, and the Command
// field specifies the shell command to run after download. Use {file} as a
// placeholder in Command to reference the downloaded file path.
type URLInstall struct {
	URL     string `yaml:"url"`
	Command string `yaml:"command"` // Command to run after download, use {file} as placeholder
}

// Config holds the packages configuration including the list of packages
// to manage, default package manager settings, and priority ordering for manager
// selection. DefaultManager specifies which manager to prefer when multiple are
// available, and ManagerPriority allows fine-grained control over the order in
// which package managers are tried.
type Config struct {
	Packages        []Package        `yaml:"packages"`
	DefaultManager  PackageManager   `yaml:"default_manager,omitempty"`
	ManagerPriority []PackageManager `yaml:"manager_priority,omitempty"`
}

// InstallResult represents the result of a package installation attempt.
// It contains the package name, whether the installation succeeded, a message
// describing the outcome, and the method used (e.g., "pacman", "custom", "url").
// This is returned by Install and InstallAll methods to report installation status.
type InstallResult struct {
	Package string
	Message string
	Method  string
	Success bool
}
