package template

import (
	"bytes"
	"testing"
)

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
