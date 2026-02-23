package preview

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/AntoineGS/tidydots/internal/platform"
	tmpl "github.com/AntoineGS/tidydots/internal/template"
)

func testEngine() *tmpl.Engine {
	ctx := tmpl.NewContextFromPlatform(&platform.Platform{
		OS:       "linux",
		Distro:   "arch",
		Hostname: "testhost",
		User:     "testuser",
		EnvVars:  make(map[string]string),
	})
	return tmpl.NewEngine(ctx)
}

func testWatcher() *Watcher {
	return New(testEngine(), slog.Default())
}

func TestRenderTemplate_WritesRenderedFile(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	tmplPath := filepath.Join(dir, "config.tmpl")
	if err := os.WriteFile(tmplPath, []byte("host={{ .Hostname }}"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := w.renderTemplate(tmplPath); err != nil {
		t.Fatalf("renderTemplate() error: %v", err)
	}

	renderedPath := tmpl.RenderedPath(tmplPath)
	got, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("reading rendered file: %v", err)
	}

	want := "host=testhost"
	if string(got) != want {
		t.Errorf("rendered content = %q, want %q", string(got), want)
	}
}

func TestRenderTemplate_SyntaxError(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	tmplPath := filepath.Join(dir, "bad.tmpl")
	if err := os.WriteFile(tmplPath, []byte("{{ .Invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := w.renderTemplate(tmplPath)
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}

	renderedPath := tmpl.RenderedPath(tmplPath)
	if _, statErr := os.Stat(renderedPath); !os.IsNotExist(statErr) {
		t.Error("rendered file should not exist after syntax error")
	}
}

func TestRenderTemplate_FileNotFound(t *testing.T) {
	w := testWatcher()

	err := w.renderTemplate("/nonexistent/path/missing.tmpl")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestRenderTemplate_PlainContent(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	tmplPath := filepath.Join(dir, "plain.tmpl")
	content := "no template delimiters here"
	if err := os.WriteFile(tmplPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := w.renderTemplate(tmplPath); err != nil {
		t.Fatalf("renderTemplate() error: %v", err)
	}

	renderedPath := tmpl.RenderedPath(tmplPath)
	got, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("reading rendered file: %v", err)
	}

	if string(got) != content {
		t.Errorf("rendered content = %q, want %q", string(got), content)
	}
}
