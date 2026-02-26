package preview

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fsnotify/fsnotify"

	tmpl "github.com/AntoineGS/tidydots/internal/template"
)

const debounceInterval = 100 * time.Millisecond

// sourceMapResponse is the NDJSON response sent to stdout after each render.
type sourceMapResponse struct {
	SourceMap  map[string]int    `json:"source_map"`
	ReverseMap map[string]int    `json:"reverse_map"`
	LineTypes  map[string]string `json:"line_types"`
	File       string            `json:"file"`
}

// Watcher watches template files and re-renders them on changes.
type Watcher struct {
	engine      *tmpl.Engine
	logger      *slog.Logger
	stdout      io.Writer
	lastContent string
	lastSrcMap  map[int]int
}

// New creates a new Watcher with the given template engine and logger.
func New(engine *tmpl.Engine, logger *slog.Logger) *Watcher {
	return &Watcher{
		engine: engine,
		logger: logger,
		stdout: os.Stdout,
	}
}

// emitSourceMap writes a source map response as NDJSON to stdout.
// It computes reverse_map and line_types from the template content and forward source map.
func (w *Watcher) emitSourceMap(path, tmplContent string, srcMap map[int]int) {
	lineTypes := tmpl.ClassifyLineTypes(tmplContent)
	reverseMap := tmpl.BuildReverseMap(srcMap, lineTypes)

	resp := sourceMapResponse{
		SourceMap:  make(map[string]int, len(srcMap)),
		ReverseMap: make(map[string]int, len(reverseMap)),
		LineTypes:  make(map[string]string, len(lineTypes)),
		File:       path,
	}

	for k, v := range srcMap {
		resp.SourceMap[strconv.Itoa(k)] = v
	}
	for k, v := range reverseMap {
		resp.ReverseMap[strconv.Itoa(k)] = v
	}
	for k, v := range lineTypes {
		resp.LineTypes[strconv.Itoa(k)] = v
	}

	data, err := json.Marshal(resp)
	if err != nil {
		w.logger.Error("marshaling source map", slog.String("error", err.Error()))
		return
	}
	_, _ = fmt.Fprintf(w.stdout, "%s\n", data)
}

// renderTemplate reads a .tmpl file, renders it, and writes the output to .tmpl.rendered.
// On error, the rendered file is not written (preserving any previous good render).
func (w *Watcher) renderTemplate(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading template %s: %w", path, err)
	}

	rendered, srcMap, err := w.engine.RenderStringWithSourceMap(filepath.Base(path), string(content))
	if err != nil {
		return fmt.Errorf("rendering template %s: %w", path, err)
	}

	renderedPath := tmpl.RenderedPath(path)
	if err := os.WriteFile(renderedPath, []byte(rendered), 0o600); err != nil {
		return fmt.Errorf("writing rendered file %s: %w", renderedPath, err)
	}

	w.emitSourceMap(path, string(content), srcMap)

	w.lastContent = string(content)
	w.lastSrcMap = srcMap

	return nil
}

// renderContent renders template content from a string and writes the output to .tmpl.rendered.
// Uses the path for the template name (for error messages) and output file location.
// On error, the rendered file is not written (preserving any previous good render).
func (w *Watcher) renderContent(path, content string) error {
	rendered, srcMap, err := w.engine.RenderStringWithSourceMap(filepath.Base(path), content)
	if err != nil {
		return fmt.Errorf("rendering template %s: %w", path, err)
	}

	renderedPath := tmpl.RenderedPath(path)
	if err := os.WriteFile(renderedPath, []byte(rendered), 0o600); err != nil {
		return fmt.Errorf("writing rendered file %s: %w", renderedPath, err)
	}

	w.emitSourceMap(path, content, srcMap)

	w.lastContent = content
	w.lastSrcMap = srcMap

	return nil
}

// stdinMessage is a union type for all stdin NDJSON messages.
type stdinMessage struct {
	Content      string               `json:"content,omitempty"`
	RenderedEdit *renderedEditPayload `json:"rendered_edit,omitempty"`
}

// readStdin reads NDJSON render requests from the given reader and renders each one.
// It blocks until the reader is exhausted or ctx is canceled.
// Errors on individual renders are printed to stderr (same as file-watch renders)
// but do not stop processing.
func (w *Watcher) readStdin(ctx context.Context, path string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var msg stdinMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}

		switch {
		case msg.RenderedEdit != nil:
			printRenderStatus(path, w.handleRenderedEdit(path, *msg.RenderedEdit))
		case msg.Content != "":
			printRenderStatus(path, w.renderContent(path, msg.Content))
		}
	}

	if err := scanner.Err(); err != nil {
		w.logger.Error("stdin read error", slog.String("error", err.Error()))
	}
}

// handleRenderedEdit applies structural edits from the rendered buffer back to the template.
// It uses the last known template state (content + source map) to map rendered lines back
// to template lines, applies the edits, emits a template_update response, and re-renders.
func (w *Watcher) handleRenderedEdit(path string, edit renderedEditPayload) error {
	if w.lastContent == "" || w.lastSrcMap == nil {
		return fmt.Errorf("no template state available for rendered edit")
	}

	lineTypes := tmpl.ClassifyLineTypes(w.lastContent)
	reverseMap := tmpl.BuildReverseMap(w.lastSrcMap, lineTypes)

	updated, cursorLine, err := ApplyRenderedEdit(w.lastContent, reverseMap, lineTypes, edit)
	if err != nil {
		return fmt.Errorf("applying rendered edit: %w", err)
	}

	w.emitTemplateUpdate(updated, cursorLine)

	return w.renderContent(path, updated)
}

// emitTemplateUpdate writes a template_update NDJSON response to stdout.
// This tells the editor what the new template content is and where to place the cursor.
func (w *Watcher) emitTemplateUpdate(content string, cursorLine int) {
	resp := templateUpdateResponse{
		TemplateUpdate: templateUpdatePayload{
			Content:    content,
			CursorLine: cursorLine,
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		w.logger.Error("marshaling template update", slog.String("error", err.Error()))
		return
	}
	_, _ = fmt.Fprintf(w.stdout, "%s\n", data)
}

// discoverTemplates finds all .tmpl files at the given path.
// If path is a file, it validates it has a .tmpl suffix.
// If path is a directory, it walks recursively for .tmpl files.
func (w *Watcher) discoverTemplates(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	if !info.IsDir() {
		if !tmpl.IsTemplateFile(path) {
			return nil, fmt.Errorf("%s is not a template file (must end in .tmpl)", path)
		}

		return []string{path}, nil
	}

	var templates []string

	err = filepath.WalkDir(path, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		if tmpl.IsTemplateFile(p) {
			templates = append(templates, p)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory %s: %w", path, err)
	}

	return templates, nil
}

// collectWatchDirs returns the unique set of directories containing the given template files.
func collectWatchDirs(templates []string) []string {
	seen := make(map[string]bool)
	var dirs []string

	for _, t := range templates {
		dir := filepath.Dir(t)
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}

	return dirs
}

// printRenderStatus prints an error message when a template render fails.
// Successful renders are silent.
func printRenderStatus(path string, renderErr error) {
	if renderErr != nil {
		timestamp := time.Now().Format("15:04:05")
		_, _ = fmt.Fprintf(os.Stderr, "\u2717 %s error: %v (%s)\n",
			filepath.Base(path), renderErr, timestamp)
	}
}

// Watch discovers templates at the given path, performs an initial render,
// then watches for changes and re-renders on .tmpl file modifications.
// It blocks until ctx is canceled.
func (w *Watcher) Watch(ctx context.Context, path string) error {
	return w.WatchWithStdin(ctx, path, nil)
}

// WatchWithStdin is like Watch but also reads NDJSON render requests from stdin.
// When stdin is non-nil and exactly one template is being watched, content received
// on stdin is rendered directly (bypassing file reads). This enables live preview
// from editors that pipe buffer content on each keystroke.
// It blocks until ctx is canceled.
func (w *Watcher) WatchWithStdin(ctx context.Context, path string, stdin io.Reader) error {
	templates, err := w.discoverTemplates(path)
	if err != nil {
		return fmt.Errorf("discovering templates: %w", err)
	}

	if len(templates) == 0 {
		return fmt.Errorf("no template files found at %s", path)
	}

	// Build a set of known template paths for quick lookup.
	templateSet := make(map[string]bool, len(templates))
	for _, t := range templates {
		templateSet[t] = true
	}

	// Initial render of all templates.
	for _, t := range templates {
		printRenderStatus(t, w.renderTemplate(t))
	}

	// Start stdin reader for single-file watches.
	if stdin != nil && len(templates) == 1 {
		go w.readStdin(ctx, templates[0], stdin)
	}

	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("creating file watcher: %w", err)
	}
	defer func() { _ = fsWatcher.Close() }()

	for _, dir := range collectWatchDirs(templates) {
		if addErr := fsWatcher.Add(dir); addErr != nil {
			return fmt.Errorf("watching directory %s: %w", dir, addErr)
		}
	}

	return w.watchLoop(ctx, fsWatcher, templateSet)
}

// watchLoop processes file system events with debouncing until ctx is canceled.
func (w *Watcher) watchLoop(ctx context.Context, fsWatcher *fsnotify.Watcher, templateSet map[string]bool) error {
	timers := make(map[string]*time.Timer)

	for {
		select {
		case <-ctx.Done():
			for _, timer := range timers {
				timer.Stop()
			}

			return nil

		case event, ok := <-fsWatcher.Events:
			if !ok {
				return fmt.Errorf("file watcher events channel closed unexpectedly")
			}

			w.handleEvent(event, templateSet, timers)

		case watchErr, ok := <-fsWatcher.Errors:
			if !ok {
				return fmt.Errorf("file watcher errors channel closed unexpectedly")
			}

			w.logger.Error("file watcher error", slog.String("error", watchErr.Error()))
		}
	}
}

// handleEvent processes a single fsnotify event, debouncing re-renders for template files.
func (w *Watcher) handleEvent(event fsnotify.Event, templateSet map[string]bool, timers map[string]*time.Timer) {
	if !event.Has(fsnotify.Write) {
		return
	}

	if !tmpl.IsTemplateFile(event.Name) || !templateSet[event.Name] {
		return
	}

	eventName := event.Name
	if timer, exists := timers[eventName]; exists {
		timer.Reset(debounceInterval)
	} else {
		timers[eventName] = time.AfterFunc(debounceInterval, func() {
			printRenderStatus(eventName, w.renderTemplate(eventName))
		})
	}
}
