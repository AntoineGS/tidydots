package preview

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"testing"
	"time"

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

func TestDiscoverTemplates_SingleFile(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	tmplPath := filepath.Join(dir, "config.tmpl")
	if err := os.WriteFile(tmplPath, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := w.discoverTemplates(tmplPath)
	if err != nil {
		t.Fatalf("discoverTemplates() error: %v", err)
	}

	if len(got) != 1 || got[0] != tmplPath {
		t.Errorf("discoverTemplates() = %v, want [%s]", got, tmplPath)
	}
}

func TestDiscoverTemplates_Directory(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	tmplPath := filepath.Join(dir, "config.tmpl")
	txtPath := filepath.Join(dir, "readme.txt")
	renderedPath := filepath.Join(dir, "config.tmpl.rendered")

	for _, f := range []struct {
		path    string
		content string
	}{
		{tmplPath, "template"},
		{txtPath, "plain text"},
		{renderedPath, "rendered"},
	} {
		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := w.discoverTemplates(dir)
	if err != nil {
		t.Fatalf("discoverTemplates() error: %v", err)
	}

	if len(got) != 1 || got[0] != tmplPath {
		t.Errorf("discoverTemplates() = %v, want [%s]", got, tmplPath)
	}
}

func TestDiscoverTemplates_NestedDirectory(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	subDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tmpl1 := filepath.Join(dir, "root.tmpl")
	tmpl2 := filepath.Join(subDir, "nested.tmpl")
	txt := filepath.Join(subDir, "other.txt")

	for _, f := range []struct {
		path    string
		content string
	}{
		{tmpl1, "root"},
		{tmpl2, "nested"},
		{txt, "text"},
	} {
		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := w.discoverTemplates(dir)
	if err != nil {
		t.Fatalf("discoverTemplates() error: %v", err)
	}

	sort.Strings(got)
	want := []string{tmpl1, tmpl2}
	sort.Strings(want)

	if !slices.Equal(got, want) {
		t.Errorf("discoverTemplates() = %v, want %v", got, want)
	}
}

func TestDiscoverTemplates_NonTemplateFile(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	txtPath := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(txtPath, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := w.discoverTemplates(txtPath)
	if err == nil {
		t.Fatal("expected error for non-template file")
	}
}

// waitForFile polls until the file exists and is non-empty, or the timeout expires.
func waitForFile(t *testing.T, path string, timeout time.Duration) []byte {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for file %s", path)

	return nil
}

// waitForContent polls until the file contains the expected content, or the timeout expires.
func waitForContent(t *testing.T, path, want string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil && string(data) == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	got, _ := os.ReadFile(path)
	t.Fatalf("timed out waiting for content %q in %s, got %q", want, path, string(got))
}

func TestWatch_InitialRender(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	tmplPath := filepath.Join(dir, "config.tmpl")
	if err := os.WriteFile(tmplPath, []byte("host={{ .Hostname }}"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Watch(ctx, dir)
	}()

	renderedPath := tmpl.RenderedPath(tmplPath)
	got := waitForFile(t, renderedPath, 2*time.Second)

	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	want := "host=testhost"
	if string(got) != want {
		t.Errorf("rendered content = %q, want %q", string(got), want)
	}
}

func TestWatch_RerendersOnChange(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	tmplPath := filepath.Join(dir, "config.tmpl")
	if err := os.WriteFile(tmplPath, []byte("host={{ .Hostname }}"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Watch(ctx, dir)
	}()

	renderedPath := tmpl.RenderedPath(tmplPath)
	waitForFile(t, renderedPath, 2*time.Second)

	// Modify the template
	if err := os.WriteFile(tmplPath, []byte("user={{ .User }}"), 0o644); err != nil {
		t.Fatal(err)
	}

	waitForContent(t, renderedPath, "user=testuser", 2*time.Second)

	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
}

func TestWatch_SyntaxErrorPreservesLastGoodRender(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	tmplPath := filepath.Join(dir, "config.tmpl")
	if err := os.WriteFile(tmplPath, []byte("host={{ .Hostname }}"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Watch(ctx, dir)
	}()

	renderedPath := tmpl.RenderedPath(tmplPath)
	waitForContent(t, renderedPath, "host=testhost", 2*time.Second)

	// Write invalid template
	if err := os.WriteFile(tmplPath, []byte("{{ .Invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for the debounce + render attempt
	time.Sleep(300 * time.Millisecond)

	// Rendered file should still have the good content
	got, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("reading rendered file: %v", err)
	}

	want := "host=testhost"
	if string(got) != want {
		t.Errorf("rendered content = %q, want %q (should preserve last good render)", string(got), want)
	}

	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("Watch() error: %v", err)
	}
}

func TestWatch_SingleFile(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	tmplPath := filepath.Join(dir, "config.tmpl")
	if err := os.WriteFile(tmplPath, []byte("os={{ .OS }}"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Watch(ctx, tmplPath)
	}()

	renderedPath := tmpl.RenderedPath(tmplPath)
	got := waitForFile(t, renderedPath, 2*time.Second)

	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("Watch() error: %v", err)
	}

	want := "os=linux"
	if string(got) != want {
		t.Errorf("rendered content = %q, want %q", string(got), want)
	}
}

func TestWatch_NoTemplatesError(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	// Create a non-template file
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := w.Watch(ctx, dir)
	if err == nil {
		t.Fatal("expected error when no templates found")
	}
}
