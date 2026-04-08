package cmdexec

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// OsRunner is the real implementation of Runner using os/exec.
type OsRunner struct{}

// Run executes a command and captures its stdout and stderr.
// If the command exits with a non-zero status, the error is returned along
// with whatever output was captured and the exit code from ProcessState.
func (r OsRunner) Run(ctx context.Context, name string, args ...string) (Result, error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	runErr := cmd.Run()

	result := Result{
		Stdout: stdoutBuf.Bytes(),
		Stderr: stderrBuf.Bytes(),
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			// Not an exit error (e.g. command not found); propagate as-is.
			return result, runErr
		}
	}

	return result, runErr
}

// RunWithSudo prepends "sudo" to the command and delegates to Run.
func (r OsRunner) RunWithSudo(ctx context.Context, name string, args ...string) (Result, error) {
	newArgs := make([]string, 0, 1+len(args))
	newArgs = append(newArgs, name)
	newArgs = append(newArgs, args...)

	return r.Run(ctx, "sudo", newArgs...)
}

// LookPath delegates to exec.LookPath.
func (r OsRunner) LookPath(name string) (string, error) {
	return exec.LookPath(name)
}
