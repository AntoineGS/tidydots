package template

import (
	"bytes"
	"io"
)

// lineCountingWriter wraps an io.Writer and tracks the current output line number.
// Line numbering is 1-based: before any writes, Line() returns 1.
// Each newline byte increments the line counter.
type lineCountingWriter struct {
	inner io.Writer
	line  int
}

// newLineCountingWriter creates a lineCountingWriter wrapping the given writer.
func newLineCountingWriter(w io.Writer) *lineCountingWriter {
	return &lineCountingWriter{inner: w, line: 1}
}

// Write passes bytes through to the inner writer and counts newlines.
func (w *lineCountingWriter) Write(p []byte) (int, error) {
	w.line += bytes.Count(p, []byte("\n"))
	return w.inner.Write(p)
}

// Line returns the current 1-based line number in the output.
func (w *lineCountingWriter) Line() int {
	return w.line
}
