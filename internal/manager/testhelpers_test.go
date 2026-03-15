package manager

import (
	"runtime"
	"testing"
)

func skipIfNoSymlink(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows")
	}
}
