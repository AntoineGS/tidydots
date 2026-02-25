package template

import (
	"bytes"
	"testing"
)

func TestInstrumentTemplate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain text lines",
			input: "line one\nline two\nline three",
			want:  "{{ __srcmap 1 }}line one\n{{ __srcmap 2 }}line two\n{{ __srcmap 3 }}line three",
		},
		{
			name:  "line with template action",
			input: "{{ .Name }}",
			want:  "{{ __srcmap 1 }}{{ .Name }}",
		},
		{
			name:  "line starting with left trim",
			input: "text\n{{- if .X }}\ncontent\n{{- end }}",
			want:  "{{ __srcmap 1 }}text\n{{- __srcmap 2 }}{{ if .X }}\n{{ __srcmap 3 }}content\n{{- __srcmap 4 }}{{ end }}",
		},
		{
			name:  "left trim with leading whitespace",
			input: "text\n  {{- .Name }}",
			want:  "{{ __srcmap 1 }}text\n  {{- __srcmap 2 }}{{ .Name }}",
		},
		{
			name:  "right trim only not affected",
			input: "{{ if .X -}}\ncontent",
			want:  "{{ __srcmap 1 }}{{ if .X -}}\n{{ __srcmap 2 }}content",
		},
		{
			name:  "empty line",
			input: "before\n\nafter",
			want:  "{{ __srcmap 1 }}before\n{{ __srcmap 2 }}\n{{ __srcmap 3 }}after",
		},
		{
			name:  "single line",
			input: "hello",
			want:  "{{ __srcmap 1 }}hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := instrumentTemplate(tt.input)
			if got != tt.want {
				t.Errorf("instrumentTemplate() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestRenderStringWithSourceMap(t *testing.T) {
	ctx := &Context{
		OS:       "linux",
		Distro:   "arch",
		Hostname: "testhost",
		User:     "testuser",
	}
	engine := NewEngine(ctx)

	tests := []struct {
		name       string
		template   string
		wantOutput string
		wantMap    map[int]int
	}{
		{
			name:       "plain text identity mapping",
			template:   "line one\nline two\nline three",
			wantOutput: "line one\nline two\nline three",
			wantMap:    map[int]int{1: 1, 2: 2, 3: 3},
		},
		{
			name:       "if block taken",
			template:   "header\n{{ if eq .OS \"linux\" }}\nlinux line\n{{ end }}\nfooter",
			wantOutput: "header\n\nlinux line\n\nfooter",
			wantMap:    map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5},
		},
		{
			name:       "if block untaken",
			template:   "header\n{{ if eq .OS \"windows\" }}\nwindows line\n{{ end }}\nfooter",
			wantOutput: "header\n\nfooter",
			wantMap:    map[int]int{1: 1, 2: 2, 3: 2, 4: 2, 5: 3},
		},
		{
			name:       "trim markers preserve output",
			template:   "header\n{{- if eq .OS \"linux\" }}\nlinux line\n{{- end }}\nfooter",
			wantOutput: "header\nlinux line\nfooter",
			wantMap:    map[int]int{1: 1, 2: 1, 3: 2, 4: 2, 5: 3},
		},
		{
			name:       "mixed content and action",
			template:   "user={{ .User }}",
			wantOutput: "user=testuser",
			wantMap:    map[int]int{1: 1},
		},
		{
			name:       "variable substitution",
			template:   "OS is {{ .OS }}\nDistro is {{ .Distro }}",
			wantOutput: "OS is linux\nDistro is arch",
			wantMap:    map[int]int{1: 1, 2: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, srcMap, err := engine.RenderStringWithSourceMap("test", tt.template)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if output != tt.wantOutput {
				t.Errorf("output =\n%q\nwant:\n%q", output, tt.wantOutput)
			}

			for tmplLine, wantRendered := range tt.wantMap {
				if got, ok := srcMap[tmplLine]; !ok {
					t.Errorf("source map missing template line %d", tmplLine)
				} else if got != wantRendered {
					t.Errorf("source map[%d] = %d, want %d", tmplLine, got, wantRendered)
				}
			}
		})
	}
}

func TestLineCountingWriter(t *testing.T) {
	tests := []struct {
		name      string
		writes    []string
		wantLine  int
		wantBytes string
	}{
		{
			name:      "empty write",
			writes:    []string{},
			wantLine:  1,
			wantBytes: "",
		},
		{
			name:      "single line no newline",
			writes:    []string{"hello"},
			wantLine:  1,
			wantBytes: "hello",
		},
		{
			name:      "single newline",
			writes:    []string{"hello\n"},
			wantLine:  2,
			wantBytes: "hello\n",
		},
		{
			name:      "multiple lines single write",
			writes:    []string{"line1\nline2\nline3\n"},
			wantLine:  4,
			wantBytes: "line1\nline2\nline3\n",
		},
		{
			name:      "multiple writes",
			writes:    []string{"line1\n", "line2\n"},
			wantLine:  3,
			wantBytes: "line1\nline2\n",
		},
		{
			name:      "split across newline boundary",
			writes:    []string{"hel", "lo\nwor", "ld\n"},
			wantLine:  3,
			wantBytes: "hello\nworld\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := newLineCountingWriter(&buf)

			for _, s := range tt.writes {
				_, err := w.Write([]byte(s))
				if err != nil {
					t.Fatalf("unexpected write error: %v", err)
				}
			}

			if w.Line() != tt.wantLine {
				t.Errorf("Line() = %d, want %d", w.Line(), tt.wantLine)
			}
			if buf.String() != tt.wantBytes {
				t.Errorf("output = %q, want %q", buf.String(), tt.wantBytes)
			}
		})
	}
}

func TestBuildReverseMap(t *testing.T) {
	tests := []struct {
		name       string
		forwardMap map[int]int
		lineTypes  map[int]string
		want       map[int]int
	}{
		{
			name:       "identity mapping all text",
			forwardMap: map[int]int{1: 1, 2: 2, 3: 3},
			lineTypes:  map[int]string{1: "text", 2: "text", 3: "text"},
			want:       map[int]int{1: 1, 2: 2, 3: 3},
		},
		{
			name:       "prefers text over directive",
			forwardMap: map[int]int{1: 1, 2: 1, 3: 2, 4: 2, 5: 3},
			lineTypes:  map[int]string{1: "text", 2: "directive", 3: "text", 4: "directive", 5: "text"},
			want:       map[int]int{1: 1, 2: 3, 3: 5},
		},
		{
			name:       "prefers expression over directive",
			forwardMap: map[int]int{1: 1, 2: 1, 3: 2},
			lineTypes:  map[int]string{1: "directive", 2: "expression", 3: "text"},
			want:       map[int]int{1: 2, 2: 3},
		},
		{
			name:       "prefers text over expression",
			forwardMap: map[int]int{1: 1, 2: 1},
			lineTypes:  map[int]string{1: "expression", 2: "text"},
			want:       map[int]int{1: 2},
		},
		{
			name:       "directive only for a rendered line",
			forwardMap: map[int]int{1: 1, 2: 1},
			lineTypes:  map[int]string{1: "directive", 2: "directive"},
			want:       map[int]int{1: 1},
		},
		{
			name:       "single line",
			forwardMap: map[int]int{1: 1},
			lineTypes:  map[int]string{1: "text"},
			want:       map[int]int{1: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildReverseMap(tt.forwardMap, tt.lineTypes)
			for renderedLine, wantTmplLine := range tt.want {
				if gotTmplLine, ok := got[renderedLine]; !ok {
					t.Errorf("missing rendered line %d", renderedLine)
				} else if gotTmplLine != wantTmplLine {
					t.Errorf("reverse[%d] = %d, want %d", renderedLine, gotTmplLine, wantTmplLine)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %d entries, want %d", len(got), len(tt.want))
			}
		})
	}
}

func TestClassifyLineTypes(t *testing.T) {
	tests := []struct {
		name     string
		template string
		want     map[int]string
	}{
		{
			name:     "all plain text",
			template: "line one\nline two\nline three",
			want:     map[int]string{1: "text", 2: "text", 3: "text"},
		},
		{
			name:     "directive only",
			template: "{{ if .X }}\ncontent\n{{ end }}",
			want:     map[int]string{1: "directive", 2: "text", 3: "directive"},
		},
		{
			name:     "expression with static text",
			template: "Hello {{ .User }}",
			want:     map[int]string{1: "expression"},
		},
		{
			name:     "mixed lines",
			template: "header\n{{ if eq .OS \"linux\" }}\nHello {{ .User }}\nplain line\n{{ end }}\nfooter",
			want:     map[int]string{1: "text", 2: "directive", 3: "expression", 4: "text", 5: "directive", 6: "text"},
		},
		{
			name:     "directive with trim markers",
			template: "{{- if .X }}",
			want:     map[int]string{1: "directive"},
		},
		{
			name:     "expression with trim markers",
			template: "name={{- .User }}",
			want:     map[int]string{1: "expression"},
		},
		{
			name:     "range directive",
			template: "{{ range .Items }}{{ .Name }}{{ end }}",
			want:     map[int]string{1: "directive"},
		},
		{
			name:     "single line no delimiters",
			template: "hello world",
			want:     map[int]string{1: "text"},
		},
		{
			name:     "empty template",
			template: "",
			want:     map[int]string{1: "text"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyLineTypes(tt.template)
			for line, wantType := range tt.want {
				if gotType, ok := got[line]; !ok {
					t.Errorf("missing line %d", line)
				} else if gotType != wantType {
					t.Errorf("line %d = %q, want %q", line, gotType, wantType)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %d entries, want %d", len(got), len(tt.want))
			}
		})
	}
}
