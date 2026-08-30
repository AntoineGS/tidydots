package config

import (
	"bytes"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

// mockWhenRenderer is a simple PathRenderer for testing EvaluateWhen.
// It renders templates by looking up values in its data map.
type mockWhenRenderer struct {
	result string
	err    error
}

func (m *mockWhenRenderer) RenderString(_, _ string) (string, error) {
	if m.err != nil {
		return "", m.err
	}

	return m.result, nil
}

type mappedWhenRenderer struct {
	results map[string]string
	errOn   string
}

func (m *mappedWhenRenderer) RenderString(_, tmplStr string) (string, error) {
	if tmplStr == m.errOn {
		return "", fmt.Errorf("render failed")
	}
	return m.results[tmplStr], nil
}

func TestEvaluateWhen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		when     string
		renderer PathRenderer
		want     bool
	}{
		{
			name:     "empty string returns true",
			when:     "",
			renderer: &mockWhenRenderer{result: ""},
			want:     true,
		},
		{
			name:     "whitespace only returns true",
			when:     "   ",
			renderer: &mockWhenRenderer{result: ""},
			want:     true,
		},
		{
			name:     "nil renderer returns false",
			when:     "{{ eq .OS \"linux\" }}",
			renderer: nil,
			want:     false,
		},
		{
			name:     "renders to true",
			when:     "{{ eq .OS \"linux\" }}",
			renderer: &mockWhenRenderer{result: "true"},
			want:     true,
		},
		{
			name:     "renders to true with whitespace",
			when:     "{{ eq .OS \"linux\" }}",
			renderer: &mockWhenRenderer{result: " true "},
			want:     true,
		},
		{
			name:     "renders to false",
			when:     "{{ eq .OS \"linux\" }}",
			renderer: &mockWhenRenderer{result: "false"},
			want:     false,
		},
		{
			name:     "renders to arbitrary string",
			when:     "{{ .OS }}",
			renderer: &mockWhenRenderer{result: "linux"},
			want:     false,
		},
		{
			name:     "render error returns false",
			when:     "{{ invalid }}",
			renderer: &mockWhenRenderer{err: fmt.Errorf("template error")},
			want:     false,
		},
		{
			name:     "renders to empty string",
			when:     "{{ .Missing }}",
			renderer: &mockWhenRenderer{result: ""},
			want:     false,
		},
		{
			name:     "renders to TRUE (case sensitive)",
			when:     "{{ .OS }}",
			renderer: &mockWhenRenderer{result: "TRUE"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := EvaluateWhen(tt.when, tt.renderer)
			if got != tt.want {
				t.Errorf("EvaluateWhen(%q) = %v, want %v", tt.when, got, tt.want)
			}
		})
	}
}

func TestEvaluateWhenWithLogger_LogsRenderErrors(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	renderer := &mockWhenRenderer{err: fmt.Errorf("template parse: unexpected {{")}

	got := EvaluateWhenWithLogger("{{ eq .OS \"linux\" }}", renderer, logger)
	if got {
		t.Errorf("EvaluateWhenWithLogger on error = true, want false")
	}
	if !strings.Contains(buf.String(), "when expression") {
		t.Errorf("expected warning about when expression; log = %q", buf.String())
	}
}

func TestEvaluateWhenWithLogger_NilLoggerDoesNotPanic(t *testing.T) {
	t.Parallel()

	renderer := &mockWhenRenderer{err: fmt.Errorf("boom")}
	got := EvaluateWhenWithLogger("{{ .OS }}", renderer, nil)
	if got {
		t.Errorf("EvaluateWhenWithLogger(nil logger) on error = true, want false")
	}
}

func TestEvaluateWhenWithLogger_SuccessDoesNotLog(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	renderer := &mockWhenRenderer{result: "true"}
	got := EvaluateWhenWithLogger("{{ eq .OS \"linux\" }}", renderer, logger)
	if !got {
		t.Errorf("EvaluateWhenWithLogger on success = false, want true")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no log output on success, got %q", buf.String())
	}
}

func TestFilterSubEntries(t *testing.T) {
	t.Parallel()

	entries := []SubEntry{
		{Name: "empty"},
		{Name: "true", When: "true"},
		{Name: "false", When: "false"},
		{Name: "error", When: "error"},
	}
	original := append([]SubEntry(nil), entries...)

	tests := []struct {
		name    string
		results map[string]string
		errOn   string
		want    []string
	}{
		{name: "empty and true", results: map[string]string{"true": "true"}, want: []string{"empty", "true"}},
		{name: "false", results: map[string]string{"true": "false", "false": "false"}, want: []string{"empty"}},
		{name: "renderer error", results: map[string]string{"true": "false", "false": "false"}, errOn: "error", want: []string{"empty"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer := &mappedWhenRenderer{
				results: tt.results,
				errOn:   tt.errOn,
			}
			got := FilterSubEntries(entries, renderer)
			var names []string
			for _, entry := range got {
				names = append(names, entry.Name)
			}
			if !reflect.DeepEqual(names, tt.want) {
				t.Errorf("FilterSubEntries() names = %v, want %v", names, tt.want)
			}
		})
	}

	if !reflect.DeepEqual(entries, original) {
		t.Errorf("FilterSubEntries mutated input: got %#v, want %#v", entries, original)
	}
}

func TestFilterSubEntriesWithLogger_LogsRenderErrors(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	entries := []SubEntry{{Name: "broken", When: "broken"}}

	got := FilterSubEntriesWithLogger(entries, &mockWhenRenderer{err: fmt.Errorf("boom")}, logger)
	if len(got) != 0 {
		t.Fatalf("FilterSubEntriesWithLogger() returned %d entries, want 0", len(got))
	}
	if !strings.Contains(buf.String(), "when expression") {
		t.Errorf("expected warning about when expression, log = %q", buf.String())
	}
}
