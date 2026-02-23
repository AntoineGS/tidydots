package preview

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	tmpl "github.com/AntoineGS/tidydots/internal/template"
)

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
	if err := os.WriteFile(renderedPath, rendered, 0o644); err != nil {
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
