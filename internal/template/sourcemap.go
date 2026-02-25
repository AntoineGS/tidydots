package template

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/template"
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

// templateActionPattern matches {{ ... }} blocks (including trim markers) for stripping.
var templateActionPattern = regexp.MustCompile(`\{\{-?\s*.*?\s*-?\}\}`)

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

// fillSourceMapGaps fills in missing template lines by mapping them to the nearest
// mapped line above (or the first mapped line below if at the start).
func fillSourceMapGaps(srcMap map[int]int, totalLines int) {
	// Forward fill: carry the last known rendered line forward
	lastKnown := 0
	for line := 1; line <= totalLines; line++ {
		if val, ok := srcMap[line]; ok {
			lastKnown = val
		} else if lastKnown > 0 {
			srcMap[line] = lastKnown
		}
	}

	// Backward fill: if the first lines have no mapping, use the first known value
	firstKnown := 0
	for line := 1; line <= totalLines; line++ {
		if val, ok := srcMap[line]; ok {
			firstKnown = val
			break
		}
	}
	if firstKnown > 0 {
		for line := 1; line <= totalLines; line++ {
			if _, ok := srcMap[line]; !ok {
				srcMap[line] = firstKnown
			} else {
				break
			}
		}
	}
}

// RenderStringWithSourceMap renders a template string and returns the rendered output
// along with a source map (template line -> rendered line, both 1-based).
func (e *Engine) RenderStringWithSourceMap(name, tmplStr string) (string, map[int]int, error) {
	totalLines := strings.Count(tmplStr, "\n") + 1

	// If no template delimiters, return identity mapping
	if !strings.Contains(tmplStr, "{{") {
		srcMap := make(map[int]int, totalLines)
		for i := 1; i <= totalLines; i++ {
			srcMap[i] = i
		}
		return tmplStr, srcMap, nil
	}

	// Build instrumented FuncMap with __srcmap closure
	srcMap := make(map[int]int)
	var lcw *lineCountingWriter

	funcMap := make(template.FuncMap, len(e.funcMap)+1)
	for k, v := range e.funcMap {
		funcMap[k] = v
	}
	funcMap["__srcmap"] = func(templateLine int) string {
		if _, exists := srcMap[templateLine]; !exists {
			srcMap[templateLine] = lcw.Line()
		}
		return ""
	}

	// Instrument and parse
	instrumented := instrumentTemplate(tmplStr)
	tmpl, err := template.New(name).Funcs(funcMap).Parse(instrumented)
	if err != nil {
		return "", nil, fmt.Errorf("parsing instrumented template %q: %w", name, err)
	}

	// Render with line counting
	var buf bytes.Buffer
	lcw = newLineCountingWriter(&buf)
	if err := tmpl.Execute(lcw, e.ctx); err != nil {
		return "", nil, fmt.Errorf("executing template %q: %w", name, err)
	}

	// Fill gaps for unmapped lines
	fillSourceMapGaps(srcMap, totalLines)

	return buf.String(), srcMap, nil
}

// lineTypePriority maps line types to priority values for reverse map selection.
// Lower values are preferred when multiple template lines map to the same rendered line.
var lineTypePriority = map[string]int{"text": 0, "expression": 1, "directive": 2}

// BuildReverseMap inverts a forward source map (template line -> rendered line)
// into a reverse map (rendered line -> template line).
// When multiple template lines map to the same rendered line, the one with
// the highest-priority line type is chosen (text > expression > directive).
func BuildReverseMap(forwardMap map[int]int, lineTypes map[int]string) map[int]int {
	candidates := make(map[int][]int)
	for tmplLine, renderedLine := range forwardMap {
		candidates[renderedLine] = append(candidates[renderedLine], tmplLine)
	}

	reverseMap := make(map[int]int, len(candidates))
	for renderedLine, tmplLines := range candidates {
		best := tmplLines[0]
		bestPri := lineTypePriority[lineTypes[best]]
		for _, tl := range tmplLines[1:] {
			p := lineTypePriority[lineTypes[tl]]
			if p < bestPri {
				best = tl
				bestPri = p
			}
		}
		reverseMap[renderedLine] = best
	}

	return reverseMap
}

// ClassifyLineTypes classifies each line in a template string by its type:
//   - "text"       — no {{ delimiters; editable in reverse editing
//   - "expression" — has {{ with remaining non-whitespace after stripping {{ ... }} blocks; read-only
//   - "directive"  — has {{ but only whitespace remains after stripping; read-only, no visible output
func ClassifyLineTypes(tmplStr string) map[int]string {
	lines := strings.Split(tmplStr, "\n")
	types := make(map[int]string, len(lines))

	for i, line := range lines {
		lineNum := i + 1
		if !strings.Contains(line, "{{") {
			types[lineNum] = "text"
			continue
		}
		stripped := templateActionPattern.ReplaceAllString(line, "")
		if strings.TrimSpace(stripped) == "" {
			types[lineNum] = "directive"
		} else {
			types[lineNum] = "expression"
		}
	}

	return types
}
