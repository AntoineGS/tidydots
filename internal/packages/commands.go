package packages

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/AntoineGS/tidydots/internal/platform"
)

// bulkListFunc runs a single command to list all installed packages and returns
// a set of lowercase package IDs. Used by managers where per-package queries are
// slow or unreliable under concurrency (e.g. winget).
type bulkListFunc func(ctx context.Context) map[string]bool

// managerCmd defines the install and check commands for a package manager.
// The placeholder "{pkg}" in args is replaced with the actual package name.
type managerCmd struct {
	install  []string     // command args for install, e.g. {"sudo", "pacman", "-S", "--noconfirm", "{pkg}"}
	check    []string     // command args for checking install status, e.g. {"pacman", "-Q", "{pkg}"}
	bulkList bulkListFunc // if set, IsInstalled uses a single bulk query instead of per-package checks
}

var managerCmds = map[PackageManager]managerCmd{
	Pacman: {install: []string{"sudo", "pacman", "-S", "--noconfirm", "{pkg}"}, check: []string{"pacman", "-Q", "{pkg}"}},
	Yay:    {install: []string{"yay", "-S", "--noconfirm", "{pkg}"}, check: []string{"pacman", "-Q", "{pkg}"}},
	Paru:   {install: []string{"paru", "-S", "--noconfirm", "{pkg}"}, check: []string{"pacman", "-Q", "{pkg}"}},
	Apt:    {install: []string{"sudo", "apt-get", "install", "-y", "{pkg}"}, check: []string{"dpkg", "-s", "{pkg}"}},
	Dnf:    {install: []string{"sudo", "dnf", "install", "-y", "{pkg}"}, check: []string{"rpm", "-q", "{pkg}"}},
	Brew:   {install: []string{"brew", "install", "{pkg}"}, check: []string{"brew", "list", "{pkg}"}},
	Winget: {install: []string{"winget", "install", "--accept-package-agreements", "--accept-source-agreements", "{pkg}"}, bulkList: wingetBulkList},
	Scoop:  {install: []string{"scoop", "install", "{pkg}"}, check: []string{"scoop", "info", "{pkg}"}},
	Choco:  {install: []string{"choco", "install", "-y", "{pkg}"}, check: []string{"choco", "list", "--local-only", "{pkg}"}},
}

// wingetBulkList runs "winget list" once and parses the output to build a set of
// installed package IDs. This avoids N slow serial "winget list --id" calls and
// the concurrency bugs (0x8a150001) that winget has with parallel queries.
func wingetBulkList(ctx context.Context) map[string]bool {
	slog.Debug("running winget bulk list")

	cmd := exec.CommandContext(ctx, "winget", "list", "--disable-interactivity", "--accept-source-agreements")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err != nil {
		slog.Debug("winget bulk list failed",
			slog.String("error", err.Error()),
			slog.String("stderr", strings.TrimSpace(stderr.String())))
		return make(map[string]bool)
	}

	return parseWingetListOutput(stdout.String())
}

// parseWingetListOutput extracts package IDs from winget list output.
// The output has a header row with column names separated by dashes, then data rows.
// The Id column position is detected from the header.
func parseWingetListOutput(output string) map[string]bool {
	ids := make(map[string]bool)
	lines := cleanWingetOutput(output)

	// Find the header separator line (all dashes) to locate column positions
	headerIdx := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 0 && strings.Count(trimmed, "-") == len(trimmed) {
			headerIdx = i
			break
		}
	}

	if headerIdx < 1 {
		slog.Debug("winget bulk list: could not find header separator")
		return ids
	}

	// Parse column header to find the Id column start position
	header := lines[headerIdx-1]
	idStart := strings.Index(header, "Id")
	if idStart < 0 {
		slog.Debug("winget bulk list: could not find Id column in header")
		return ids
	}

	// Find the next column after Id (Version) to determine Id column end
	idEnd := len(header)
	versionIdx := strings.Index(header, "Version")
	if versionIdx > idStart {
		idEnd = versionIdx
	}

	// Parse data rows
	for _, line := range lines[headerIdx+1:] {
		if len(line) <= idStart {
			continue
		}

		end := idEnd
		if end > len(line) {
			end = len(line)
		}

		id := strings.TrimSpace(line[idStart:end])
		if id != "" {
			ids[strings.ToLower(id)] = true
		}
	}

	slog.Debug("winget bulk list complete",
		slog.Int("packages_found", len(ids)))

	return ids
}

// cleanWingetOutput handles winget's progress spinner and encoding quirks.
// When stdout is piped (not a terminal), winget writes \r-based spinner
// characters that accumulate in the buffer. Windows line endings (\r\n) are
// normalized first, then remaining \r characters (from the spinner) are handled
// by taking the last \r-delimited segment per line.
func cleanWingetOutput(output string) []string {
	// Normalize Windows line endings before splitting
	output = strings.ReplaceAll(output, "\r\n", "\n")
	raw := strings.Split(output, "\n")
	lines := make([]string, 0, len(raw))

	for _, line := range raw {
		// Take last \r segment (handles progress spinner overwriting)
		if idx := strings.LastIndex(line, "\r"); idx >= 0 {
			line = line[idx+1:]
		}

		lines = append(lines, line)
	}

	return lines
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
