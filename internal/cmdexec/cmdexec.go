package cmdexec

import "context"

// Result holds the output of a command execution.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Runner abstracts command execution.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (Result, error)
	RunWithSudo(ctx context.Context, name string, args ...string) (Result, error)
	LookPath(name string) (string, error)
}
