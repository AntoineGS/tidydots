package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSkipIfNoSymlink verifies that SkipIfNoSymlink does not skip on the current
// platform (it would only skip on Windows without Developer Mode). We just call it
// and verify the test is not skipped.
func TestSkipIfNoSymlink(t *testing.T) {
	SkipIfNoSymlink(t)
	// If we get here, symlinks are supported – nothing else to assert.
}

// TestPrependPath_EmptyOriginalPATH verifies behavior when PATH env var is empty.
func TestPrependPath_EmptyOriginalPATH(t *testing.T) {
	t.Setenv("PATH", "")

	dir := t.TempDir()
	result := PrependPath(t, dir)

	// Result should be dir + separator (+ empty original path)
	if len(result) == 0 {
		t.Error("PrependPath returned empty string")
	}

	if result[:len(dir)] != dir {
		t.Errorf("PrependPath result %q does not start with %q", result, dir)
	}
}

// TestCreateWindowsBat_FileContent calls the Windows bat generation function directly so it
// gets counted even on non-Windows platforms. We only verify the file is created
// with correct content – actually executing it is a Windows-only concern.
func TestCreateWindowsBat_FileContent(t *testing.T) {
	dir := t.TempDir()

	path := createWindowsBat(t, dir, "myscript", 42, "hello", "err msg")

	// File should have .bat extension
	if filepath.Ext(path) != ".bat" {
		t.Errorf("expected .bat extension, got %q", filepath.Ext(path))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading bat file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "@echo off") {
		t.Error("bat file missing @echo off")
	}
	if !strings.Contains(content, "hello") {
		t.Error("bat file missing stdout content")
	}
	if !strings.Contains(content, "err msg") {
		t.Error("bat file missing stderr content")
	}
	if !strings.Contains(content, "exit /b 42") {
		t.Error("bat file missing exit code")
	}
}

// TestCreateWindowsBat_NoOutput verifies a bat with empty stdout/stderr.
func TestCreateWindowsBat_NoOutput(t *testing.T) {
	dir := t.TempDir()

	path := createWindowsBat(t, dir, "silent", 0, "", "")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading bat file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "exit /b 0") {
		t.Error("bat file missing exit /b 0")
	}
}
