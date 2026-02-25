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
