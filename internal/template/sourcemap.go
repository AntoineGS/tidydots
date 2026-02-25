package template

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
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

// leftTrimPattern matches lines that start with optional whitespace followed by {{-
var leftTrimPattern = regexp.MustCompile(`^(\s*)\{\{-\s*`)

// instrumentTemplate injects {{ __srcmap N }} markers at the start of each line.
// Lines starting with {{- get special handling: the {{- is replaced with
// {{- __srcmap N }}{{ so the marker absorbs the left-trim.
func instrumentTemplate(tmplStr string) string {
	lines := strings.Split(tmplStr, "\n")
	result := make([]string, len(lines))

	for i, line := range lines {
		lineNum := i + 1
		if m := leftTrimPattern.FindStringSubmatchIndex(line); m != nil {
			// Line starts with optional whitespace + {{-
			// m[2]:m[3] is the whitespace capture group
			ws := line[m[2]:m[3]]
			rest := line[m[1]:]
			result[i] = fmt.Sprintf("%s{{- __srcmap %d }}{{ %s", ws, lineNum, rest)
		} else {
			result[i] = fmt.Sprintf("{{ __srcmap %d }}%s", lineNum, line)
		}
	}

	return strings.Join(result, "\n")
}
