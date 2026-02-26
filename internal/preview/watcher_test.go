package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
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

func TestRenderContent_WritesRenderedFile(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "config.tmpl")

	err := w.renderContent(tmplPath, "host={{ .Hostname }}")
	if err != nil {
		t.Fatalf("renderContent() error: %v", err)
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

func TestRenderContent_SyntaxError(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "bad.tmpl")

	err := w.renderContent(tmplPath, "{{ .Invalid")
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}

	renderedPath := tmpl.RenderedPath(tmplPath)
	if _, statErr := os.Stat(renderedPath); !os.IsNotExist(statErr) {
		t.Error("rendered file should not exist after syntax error")
	}
}

func TestRenderContent_PlainContent(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "plain.tmpl")

	content := "no template delimiters here"
	if err := w.renderContent(tmplPath, content); err != nil {
		t.Fatalf("renderContent() error: %v", err)
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

// waitForContent polls until the file contains the expected content, or a 2-second timeout expires.
func waitForContent(t *testing.T, path, want string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
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

	waitForContent(t, renderedPath, "user=testuser")

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
	waitForContent(t, renderedPath, "host=testhost")

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

func TestReadStdin_RendersContent(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "config.tmpl")

	if err := os.WriteFile(tmplPath, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := `{"content":"host={{ .Hostname }}"}` + "\n" +
		`{"content":"user={{ .User }}"}` + "\n"
	reader := strings.NewReader(input)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.readStdin(ctx, tmplPath, reader)

	renderedPath := tmpl.RenderedPath(tmplPath)
	got, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("reading rendered file: %v", err)
	}

	want := "user=testuser"
	if string(got) != want {
		t.Errorf("rendered content = %q, want %q", string(got), want)
	}
}

func TestReadStdin_SyntaxErrorPreservesLastGoodRender(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "config.tmpl")

	if err := os.WriteFile(tmplPath, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := `{"content":"host={{ .Hostname }}"}` + "\n" +
		`{"content":"{{ .Invalid"}` + "\n"
	reader := strings.NewReader(input)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.readStdin(ctx, tmplPath, reader)

	renderedPath := tmpl.RenderedPath(tmplPath)
	got, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("reading rendered file: %v", err)
	}

	want := "host=testhost"
	if string(got) != want {
		t.Errorf("rendered content = %q, want %q (should preserve last good render)", string(got), want)
	}
}

func TestReadStdin_SkipsMalformedJSON(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "config.tmpl")

	if err := os.WriteFile(tmplPath, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := "not json\n" +
		`{"content":"host={{ .Hostname }}"}` + "\n"
	reader := strings.NewReader(input)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.readStdin(ctx, tmplPath, reader)

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

func TestReadStdin_RespectsContextCancellation(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "config.tmpl")

	if err := os.WriteFile(tmplPath, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.readStdin(ctx, tmplPath, pr)
		close(done)
	}()

	cancel()
	pw.Close()

	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("readStdin did not return after context cancellation")
	}
}

func TestWatch_RendersFromStdin(t *testing.T) {
	w := testWatcher()
	dir := t.TempDir()

	tmplPath := filepath.Join(dir, "config.tmpl")
	if err := os.WriteFile(tmplPath, []byte("host={{ .Hostname }}"), 0o644); err != nil {
		t.Fatal(err)
	}

	pr, pw := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.WatchWithStdin(ctx, tmplPath, pr)
	}()

	renderedPath := tmpl.RenderedPath(tmplPath)

	// Wait for initial render from file
	waitForContent(t, renderedPath, "host=testhost")

	// Send new content via stdin
	_, err := fmt.Fprintln(pw, `{"content":"user={{ .User }}"}`)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for stdin-driven render
	waitForContent(t, renderedPath, "user=testuser")

	pw.Close()
	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("WatchWithStdin() error: %v", err)
	}
}

func TestRenderContent_EmitsEnrichedSourceMap(t *testing.T) {
	w := testWatcher()
	var stdoutBuf bytes.Buffer
	w.stdout = &stdoutBuf

	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "config.tmpl")

	err := w.renderContent(tmplPath, "header\n{{ if eq .OS \"linux\" }}\nlinux\n{{ end }}\nfooter")
	if err != nil {
		t.Fatalf("renderContent() error: %v", err)
	}

	var resp sourceMapResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdoutBuf.String())), &resp); err != nil {
		t.Fatalf("failed to parse source map NDJSON: %v", err)
	}

	// Verify reverse_map is present
	if resp.ReverseMap == nil {
		t.Fatal("reverse_map is nil")
	}
	if resp.ReverseMap["1"] != 1 {
		t.Errorf("reverse_map[1] = %d, want 1", resp.ReverseMap["1"])
	}

	// Verify line_types is present
	if resp.LineTypes == nil {
		t.Fatal("line_types is nil")
	}
	if resp.LineTypes["1"] != "text" {
		t.Errorf("line_types[1] = %q, want \"text\"", resp.LineTypes["1"])
	}
	if resp.LineTypes["2"] != "directive" {
		t.Errorf("line_types[2] = %q, want \"directive\"", resp.LineTypes["2"])
	}
	if resp.LineTypes["3"] != "text" {
		t.Errorf("line_types[3] = %q, want \"text\"", resp.LineTypes["3"])
	}
}

func TestReadStdin_HandlesRenderedEdit(t *testing.T) {
	w := testWatcher()
	var stdoutBuf bytes.Buffer
	w.stdout = &stdoutBuf

	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "config.tmpl")
	if err := os.WriteFile(tmplPath, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	// First send content to establish state, then a rendered_edit
	input := `{"content":"header\nfooter"}` + "\n" +
		`{"rendered_edit":{"inserts":[{"after_rendered_line":1,"text":"middle"}]}}` + "\n"
	reader := strings.NewReader(input)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.readStdin(ctx, tmplPath, reader)

	// Verify template_update was emitted
	lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
	foundUpdate := false
	for _, line := range lines {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		if _, ok := raw["template_update"]; ok {
			foundUpdate = true
			var resp templateUpdateResponse
			if err := json.Unmarshal([]byte(line), &resp); err != nil {
				t.Fatalf("failed to parse template_update: %v", err)
			}
			want := "header\nmiddle\nfooter"
			if resp.TemplateUpdate.Content != want {
				t.Errorf("template_update content = %q, want %q", resp.TemplateUpdate.Content, want)
			}
		}
	}
	if !foundUpdate {
		t.Error("no template_update message found in stdout")
	}
}

func TestReadStdin_StateTracking(t *testing.T) {
	w := testWatcher()
	var stdoutBuf bytes.Buffer
	w.stdout = &stdoutBuf

	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "config.tmpl")
	if err := os.WriteFile(tmplPath, []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := `{"content":"header\nfooter"}` + "\n"
	reader := strings.NewReader(input)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.readStdin(ctx, tmplPath, reader)

	if w.lastContent != "header\nfooter" {
		t.Errorf("lastContent = %q, want %q", w.lastContent, "header\nfooter")
	}
	if w.lastSrcMap == nil {
		t.Error("lastSrcMap is nil after renderContent")
	}
}

func TestRenderTemplate_EstablishesStateForRenderedEdit(t *testing.T) {
	w := testWatcher()
	var stdoutBuf bytes.Buffer
	w.stdout = &stdoutBuf

	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "config.tmpl")
	if err := os.WriteFile(tmplPath, []byte("header\nfooter"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Initial render from file (no content message via stdin)
	if err := w.renderTemplate(tmplPath); err != nil {
		t.Fatalf("renderTemplate() error: %v", err)
	}

	stdoutBuf.Reset()

	// Send a rendered_edit directly — no preceding content message
	input := `{"rendered_edit":{"inserts":[{"after_rendered_line":1,"text":"middle"}]}}` + "\n"
	reader := strings.NewReader(input)

	w.readStdin(t.Context(), tmplPath, reader)

	// Verify template_update was emitted (not an error)
	lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
	foundUpdate := false
	for _, line := range lines {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		if _, ok := raw["template_update"]; ok {
			foundUpdate = true
			var resp templateUpdateResponse
			if err := json.Unmarshal([]byte(line), &resp); err != nil {
				t.Fatalf("failed to parse template_update: %v", err)
			}
			want := "header\nmiddle\nfooter"
			if resp.TemplateUpdate.Content != want {
				t.Errorf("template_update content = %q, want %q", resp.TemplateUpdate.Content, want)
			}
		}
	}
	if !foundUpdate {
		t.Error("no template_update message found — renderTemplate did not establish state for rendered edits")
	}
}

func TestRenderContent_EmitsSourceMap(t *testing.T) {
	w := testWatcher()
	var stdoutBuf bytes.Buffer
	w.stdout = &stdoutBuf

	dir := t.TempDir()
	tmplPath := filepath.Join(dir, "config.tmpl")

	err := w.renderContent(tmplPath, "header\n{{ if eq .OS \"linux\" }}\nlinux\n{{ end }}\nfooter")
	if err != nil {
		t.Fatalf("renderContent() error: %v", err)
	}

	// Verify rendered file content
	renderedPath := tmpl.RenderedPath(tmplPath)
	got, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatalf("reading rendered file: %v", err)
	}
	wantOutput := "header\n\nlinux\n\nfooter"
	if string(got) != wantOutput {
		t.Errorf("rendered content = %q, want %q", string(got), wantOutput)
	}

	// Verify source map was emitted on stdout as NDJSON
	output := stdoutBuf.String()
	if output == "" {
		t.Fatal("no source map emitted to stdout")
	}

	var resp sourceMapResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		t.Fatalf("failed to parse source map NDJSON: %v (output: %q)", err, output)
	}

	if resp.File != tmplPath {
		t.Errorf("source map file = %q, want %q", resp.File, tmplPath)
	}

	// Verify source map has entries for all 5 template lines
	if len(resp.SourceMap) != 5 {
		t.Errorf("source map has %d entries, want 5", len(resp.SourceMap))
	}

	// Line 1 (header) should map to output line 1
	if resp.SourceMap["1"] != 1 {
		t.Errorf("source map[1] = %d, want 1", resp.SourceMap["1"])
	}
}
