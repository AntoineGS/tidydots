package preview

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	tmpl "github.com/AntoineGS/tidydots/internal/template"
)

const debounceInterval = 100 * time.Millisecond

// Watcher watches template files and re-renders them on changes.
type Watcher struct {
	engine *tmpl.Engine
	logger *slog.Logger
}

// New creates a new Watcher with the given template engine and logger.
func New(engine *tmpl.Engine, logger *slog.Logger) *Watcher {
	return &Watcher{
		engine: engine,
		logger: logger,
	}
}

// renderTemplate reads a .tmpl file, renders it, and writes the output to .tmpl.rendered.
// On error, the rendered file is not written (preserving any previous good render).
func (w *Watcher) renderTemplate(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading template %s: %w", path, err)
	}

	rendered, err := w.engine.RenderBytes(filepath.Base(path), content)
	if err != nil {
		return fmt.Errorf("rendering template %s: %w", path, err)
	}

	renderedPath := tmpl.RenderedPath(path)
	if err := os.WriteFile(renderedPath, rendered, 0o600); err != nil {
		return fmt.Errorf("writing rendered file %s: %w", renderedPath, err)
	}

	return nil
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

// printRenderStatus prints a success or error message for a template render.
func printRenderStatus(path string, renderErr error) {
	timestamp := time.Now().Format("15:04:05")
	if renderErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "\u2717 %s error: %v (%s)\n",
			filepath.Base(path), renderErr, timestamp)
	} else {
		_, _ = fmt.Fprintf(os.Stdout, "\u2713 %s rendered (%s)\n",
			filepath.Base(path), timestamp)
	}
}

// printWatchSummary prints the list of templates being watched.
func printWatchSummary(templates []string) {
	_, _ = fmt.Fprintf(os.Stdout, "Watching %d template(s)...\n", len(templates))
	for _, t := range templates {
		_, _ = fmt.Fprintf(os.Stdout, "  %s\n", filepath.Base(t))
	}
}

// Watch discovers templates at the given path, performs an initial render,
// then watches for changes and re-renders on .tmpl file modifications.
// It blocks until ctx is canceled.
func (w *Watcher) Watch(ctx context.Context, path string) error {
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

	printWatchSummary(templates)

	// Initial render of all templates.
	for _, t := range templates {
		printRenderStatus(t, w.renderTemplate(t))
	}

	_, _ = fmt.Fprintln(os.Stdout)

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
