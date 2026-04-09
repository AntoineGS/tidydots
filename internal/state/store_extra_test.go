package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen_InvalidPath(t *testing.T) {
	// Use a path where the parent is a file (so MkdirAll will fail)
	parentFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	dbPath := filepath.Join(parentFile, "subdir", ".tidydots.db")
	_, err := Open(context.Background(), dbPath)
	if err == nil {
		t.Fatal("expected error opening DB with invalid path, got nil")
	}
}

func TestOperations_AfterClose(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), ".tidydots.db")
	store, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	ctx := context.Background()

	if err := store.SaveRender(ctx, "x.tmpl", []byte("y"), "hash", "linux", "host"); err == nil {
		t.Error("SaveRender: expected error after close, got nil")
	}

	if _, err := store.GetLatestRender(ctx, "x.tmpl"); err == nil {
		t.Error("GetLatestRender: expected error after close, got nil")
	}

	if _, err := store.GetRenderHistory(ctx, "x.tmpl", 5); err == nil {
		t.Error("GetRenderHistory: expected error after close, got nil")
	}

	if _, err := store.GetRenderByID(ctx, 1); err == nil {
		t.Error("GetRenderByID: expected error after close, got nil")
	}

	if err := store.PruneHistory(ctx, "x.tmpl", 3); err == nil {
		t.Error("PruneHistory: expected error after close, got nil")
	}

	if err := store.RemoveTemplate(ctx, "x.tmpl"); err == nil {
		t.Error("RemoveTemplate: expected error after close, got nil")
	}
}

func TestParseTime_InvalidFormat(t *testing.T) {
	_, err := parseTime("not-a-time")
	if err == nil {
		t.Fatal("expected error parsing invalid time, got nil")
	}
}

func TestParseTime_Formats(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"RFC3339", "2024-01-15T10:30:00Z"},
		{"SQLite datetime", "2024-01-15 10:30:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseTime(tt.input)
			if err != nil {
				t.Errorf("parseTime(%q) unexpected error: %v", tt.input, err)
			}
			if result.IsZero() {
				t.Errorf("parseTime(%q) returned zero time", tt.input)
			}
		})
	}
}

func TestGetRenderHistory_Empty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	records, err := store.GetRenderHistory(ctx, "nonexistent.tmpl", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestPruneHistory_FewerThanKeepN(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Save 2 records, prune to keep 5 (no-op)
	for range 2 {
		if err := store.SaveRender(ctx, testTemplate, []byte("x"), "h", "linux", "host"); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.PruneHistory(ctx, testTemplate, 5); err != nil {
		t.Fatal(err)
	}

	records, err := store.GetRenderHistory(ctx, testTemplate, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
}
