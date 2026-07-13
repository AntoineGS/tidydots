package cmdexec_test

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/cmdexec"
)

func TestOsRunner_RunIn_SetsWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pwd is not available on Windows")
	}

	dir := t.TempDir()

	res, err := cmdexec.OsRunner{}.RunIn(context.Background(), cmdexec.RunOptions{Dir: dir}, "pwd")
	if err != nil {
		t.Fatalf("RunIn returned error: %v", err)
	}

	// t.TempDir can sit under a symlinked root (macOS /tmp -> /private/tmp),
	// so compare fully resolved paths.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	if got := strings.TrimSpace(string(res.Stdout)); got != want {
		t.Errorf("pwd = %q, want %q", got, want)
	}
}

func TestOsRunner_RunIn_NonZeroExitReportsExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}

	res, err := cmdexec.OsRunner{}.RunIn(context.Background(), cmdexec.RunOptions{}, "sh", "-c", "exit 3")
	if err == nil {
		t.Fatal("RunIn returned nil error for a non-zero exit")
	}

	if res.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", res.ExitCode)
	}
}
