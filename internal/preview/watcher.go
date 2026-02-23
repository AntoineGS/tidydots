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
