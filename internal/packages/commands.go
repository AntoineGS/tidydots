package packages

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/AntoineGS/tidydots/internal/platform"
)

// managerCmd defines the install and check commands for a package manager.
// The placeholder "{pkg}" in args is replaced with the actual package name.
type managerCmd struct {
	install []string // command args for install, e.g. {"sudo", "pacman", "-S", "--noconfirm", "{pkg}"}
	check   []string // command args for checking install status, e.g. {"pacman", "-Q", "{pkg}"}
}

var managerCmds = map[PackageManager]managerCmd{
	Pacman: {install: []string{"sudo", "pacman", "-S", "--noconfirm", "{pkg}"}, check: []string{"pacman", "-Q", "{pkg}"}},
	Yay:    {install: []string{"yay", "-S", "--noconfirm", "{pkg}"}, check: []string{"pacman", "-Q", "{pkg}"}},
	Paru:   {install: []string{"paru", "-S", "--noconfirm", "{pkg}"}, check: []string{"pacman", "-Q", "{pkg}"}},
	Apt:    {install: []string{"sudo", "apt-get", "install", "-y", "{pkg}"}, check: []string{"dpkg", "-s", "{pkg}"}},
	Dnf:    {install: []string{"sudo", "dnf", "install", "-y", "{pkg}"}, check: []string{"rpm", "-q", "{pkg}"}},
	Brew:   {install: []string{"brew", "install", "{pkg}"}, check: []string{"brew", "list", "{pkg}"}},
	Winget: {install: []string{"winget", "install", "--accept-package-agreements", "--accept-source-agreements", "{pkg}"}, check: []string{"winget", "list", "--id", "{pkg}"}},
	Scoop:  {install: []string{"scoop", "install", "{pkg}"}, check: []string{"scoop", "info", "{pkg}"}},
	Choco:  {install: []string{"choco", "install", "-y", "{pkg}"}, check: []string{"choco", "list", "--local-only", "{pkg}"}},
}

// expandArgs replaces "{pkg}" placeholders in args with the actual package name.
func expandArgs(args []string, pkgName string) []string {
	result := make([]string, len(args))
	for i, arg := range args {
		if arg == "{pkg}" {
			result[i] = pkgName
		} else {
			result[i] = arg
		}
	}

	return result
}

// BuildCommand creates an *exec.Cmd for installing a package using the given method.
// It is a pure command builder — the caller controls execution, stdio wiring, and dry-run logic.
// Returns nil if no command can be built for the given method.
func BuildCommand(ctx context.Context, pkg Package, method, osType string) *exec.Cmd { //nolint:gocyclo // switch over package manager types is inherently branchy
	pm := PackageManager(method)

	// Package managers (pacman, yay, apt, etc.)
	if mc, ok := managerCmds[pm]; ok {
		if val, exists := pkg.Managers[pm]; exists {
			args := expandArgs(mc.install, val.PackageName)
			return exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // args from trusted lookup table
		}
	}

	switch method {
	case string(Git):
		gitVal, ok := pkg.Managers[Git]
		if !ok || !gitVal.IsGit() {
			return nil
		}
		target := gitVal.Git.Targets[osType]
		if target == "" {
			return nil
		}
		args := []string{"clone"}
		if gitVal.Git.Branch != "" {
			args = append(args, "-b", gitVal.Git.Branch)
		}
		args = append(args, gitVal.Git.URL, target)
		if gitVal.Git.Sudo {
			args = append([]string{"git"}, args...)
			return exec.CommandContext(ctx, "sudo", args...) //nolint:gosec // intentional command from user config
		}
		return exec.CommandContext(ctx, "git", args...) //nolint:gosec // intentional command from user config

	case string(Installer):
		installerVal, ok := pkg.Managers[Installer]
		if !ok || !installerVal.IsInstaller() {
			return nil
		}
		command, hasCmd := installerVal.Installer.Command[osType]
		if !hasCmd {
			return nil
		}
		if osType == platform.OSWindows {
			return exec.CommandContext(ctx, "powershell", "-Command", command) //nolint:gosec // intentional install command from user config
		}
		return exec.CommandContext(ctx, "sh", "-c", command) //nolint:gosec // intentional install command from user config

	case MethodCustom:
		command, ok := pkg.Custom[osType]
		if !ok {
			return nil
		}
		if osType == platform.OSWindows {
			return exec.CommandContext(ctx, "powershell", "-Command", command) //nolint:gosec // intentional command from user config
		}
		return exec.CommandContext(ctx, "sh", "-c", command) //nolint:gosec // intentional command from user config

	case MethodURL:
		urlInstall, ok := pkg.URL[osType]
		if !ok {
			return nil
		}
		if osType == platform.OSWindows {
			script := fmt.Sprintf(`
				$tmpFile = [System.IO.Path]::GetTempFileName()
				Invoke-WebRequest -Uri '%s' -OutFile $tmpFile
				$command = '%s' -replace '\{file\}', $tmpFile
				Invoke-Expression $command
				Remove-Item $tmpFile -ErrorAction SilentlyContinue
			`, urlInstall.URL, urlInstall.Command)
			return exec.CommandContext(ctx, "powershell", "-Command", script) //nolint:gosec // intentional command from user config
		}
		script := fmt.Sprintf(`
			tmpfile=$(mktemp)
			trap "rm -f $tmpfile" EXIT
			curl -fsSL -o "$tmpfile" '%s' && \
			chmod +x "$tmpfile" && \
			%s
		`, urlInstall.URL, strings.ReplaceAll(urlInstall.Command, "{file}", "$tmpfile"))
		return exec.CommandContext(ctx, "sh", "-c", script) //nolint:gosec // intentional command from user config
	}

	return nil
}
