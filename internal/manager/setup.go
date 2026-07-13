package manager

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/AntoineGS/tidydots/internal/cmdexec"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// commandSucceeded reports whether a command completed with a zero exit status.
//
// Both the error and the exit code must be consulted: OsRunner reports a
// non-zero exit as an *exec.ExitError AND sets ExitCode, while StubRunner
// returns a nil error and signals failure through ExitCode alone. Checking only
// the error would make every one of these paths untestable with the stub.
func commandSucceeded(res cmdexec.Result, err error) bool {
	return err == nil && res.ExitCode == 0
}

// shellCommand returns the shell and arguments used to execute a raw command
// string on the given platform. This mirrors runCustomCommand in
// internal/packages/install.go.
//
// SECURITY NOTE: this intentionally executes arbitrary shell commands from the
// user's configuration file. Users should only use configurations they trust.
func shellCommand(osType, command string) (string, []string) {
	if osType == platform.OSWindows {
		return "powershell", []string{"-Command", command}
	}

	return "sh", []string{"-c", command}
}

// setupWorkDir returns the fully expanded configurations repo root. Setup
// commands run from here so that repo-relative script paths resolve.
func (m *Manager) setupWorkDir() string {
	return config.ExpandPathWithTemplate(m.Config.BackupRoot, m.Platform.EnvVars, m.templateEngine)
}

// runCheck executes a check command and reports whether the desired system
// state already holds. A non-zero exit means "not set up" and is NOT an error.
func (m *Manager) runCheck(command string) bool {
	name, args := shellCommand(m.Platform.OS, command)

	res, err := m.runner.RunIn(m.ctx, cmdexec.RunOptions{Dir: m.setupWorkDir()}, name, args...) //nolint:gosec // command from trusted config

	return commandSucceeded(res, err)
}

// IsSetupApplied runs the entry's check command for the current OS and reports
// whether it passes. An entry that declares no check for this OS does not apply
// here, and is reported as applied (nothing outstanding).
//
// The check command is executed every time this is called. Checks must therefore
// be side-effect free and fast; see docs/configuration/setup.md.
func (m *Manager) IsSetupApplied(e config.SubEntry) bool {
	check := e.GetCheck(m.Platform.OS)
	if check == "" {
		return true
	}

	return m.runCheck(check)
}

// runSetupEntry executes a single setup sub-entry:
//
//  1. no run command for this OS  -> skip
//  2. check passes                -> skip (already set up)
//  3. dry run                     -> report, never execute the run command
//  4. execute the run command     -> non-zero exit is an error
//  5. re-run the check            -> still failing is an error (silent failure)
func (m *Manager) runSetupEntry(appName string, e config.SubEntry) error {
	command := e.GetRun(m.Platform.OS)
	if command == "" {
		m.logger.Debug("skipping setup entry",
			slog.String("app", appName),
			slog.String("entry", e.Name),
			slog.String("os", m.Platform.OS),
			slog.String("reason", "no run command for OS"))

		return nil
	}

	// Validation guarantees this, but a hand-built config could bypass it. An
	// empty check would run as `sh -c ""`, exit 0, and silently suppress the
	// setup forever — so fail loudly instead.
	check := e.GetCheck(m.Platform.OS)
	if check == "" {
		return fmt.Errorf("setup %s/%s: run command for %s has no matching check command",
			appName, e.Name, m.Platform.OS)
	}

	if m.runCheck(check) {
		m.logger.Info("setup already applied",
			slog.String("app", appName),
			slog.String("entry", e.Name))

		return nil
	}

	if m.DryRun {
		m.logger.Info("would run setup",
			slog.String("app", appName),
			slog.String("entry", e.Name),
			slog.String("command", command))

		return nil
	}

	name, args := shellCommand(m.Platform.OS, command)

	res, err := m.runner.RunIn(m.ctx, //nolint:gosec // command from trusted config
		cmdexec.RunOptions{Dir: m.setupWorkDir(), Sudo: e.Sudo}, name, args...)

	if !commandSucceeded(res, err) {
		stderr := strings.TrimSpace(string(res.Stderr))

		return fmt.Errorf("setup %s/%s: command failed (exit %d): %s",
			appName, e.Name, res.ExitCode, stderr)
	}

	// Confirm the command actually achieved what it claimed. A script can exit 0
	// without doing its job; without this, that failure would be invisible.
	if !m.runCheck(check) {
		return fmt.Errorf("setup %s/%s: command succeeded but check still fails",
			appName, e.Name)
	}

	m.logger.Info("setup applied",
		slog.String("app", appName),
		slog.String("entry", e.Name))

	return nil
}
