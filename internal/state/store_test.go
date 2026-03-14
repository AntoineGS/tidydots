package state

import (
	"context"
	"path/filepath"
	"testing"
)

const testTemplate = "test.tmpl"

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), ".tidydots.db")
	store, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() }) //nolint:errcheck // cleanup is best-effort
	return store
}

func TestOpen_CreatesDBAndSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "subdir", ".tidydots.db")
	store, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = store.Close() }() //nolint:errcheck // cleanup is best-effort

	// Should have schema_version table with version 1
	var version int
	ctx := context.Background()
	if err := store.db.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("failed to read schema version: %v", err)
	}
	if version != 1 {
		t.Errorf("schema version = %d, want 1", version)
	}
}

func TestOpen_Idempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), ".tidydots.db")

	// Open twice - should not error
	store1, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	_ = store1.Close() //nolint:errcheck // cleanup is best-effort

	store2, err := Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	defer func() { _ = store2.Close() }() //nolint:errcheck // cleanup is best-effort

	var version int
	ctx := context.Background()
	if err := store2.db.QueryRowContext(ctx, `SELECT version FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("failed to read schema version: %v", err)
	}
	if version != 1 {
		t.Errorf("schema version = %d, want 1", version)
	}
}

func TestSaveAndGetLatestRender(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tmplPath := "zsh/.zshrc.tmpl"
	content := []byte("export EDITOR=nvim\n")
	hash := "abc123"

	if err := store.SaveRender(ctx, tmplPath, content, hash, "linux", "myhost"); err != nil {
		t.Fatalf("SaveRender failed: %v", err)
	}

	record, err := store.GetLatestRender(ctx, tmplPath)
	if err != nil {
		t.Fatalf("GetLatestRender failed: %v", err)
	}
	if record == nil {
		t.Fatal("expected record, got nil")
	}

	if record.TemplatePath != tmplPath {
		t.Errorf("TemplatePath = %q, want %q", record.TemplatePath, tmplPath)
	}
	if string(record.PureRender) != string(content) {
		t.Errorf("PureRender = %q, want %q", string(record.PureRender), string(content))
	}
	if record.TemplateHash != hash {
		t.Errorf("TemplateHash = %q, want %q", record.TemplateHash, hash)
	}
	if record.PlatformOS != "linux" {
		t.Errorf("PlatformOS = %q, want %q", record.PlatformOS, "linux")
	}
	if record.PlatformHost != "myhost" {
		t.Errorf("PlatformHost = %q, want %q", record.PlatformHost, "myhost")
	}
}

func TestGetLatestRender_NoRecord(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	record, err := store.GetLatestRender(ctx, "nonexistent.tmpl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if record != nil {
		t.Errorf("expected nil, got record ID %d", record.ID)
	}
}

func TestGetLatestRender_ReturnsNewest(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tmplPath := testTemplate
	if err := store.SaveRender(ctx, tmplPath, []byte("v1"), "hash1", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRender(ctx, tmplPath, []byte("v2"), "hash2", "linux", "host"); err != nil {
		t.Fatal(err)
	}

	record, err := store.GetLatestRender(ctx, tmplPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(record.PureRender) != "v2" {
		t.Errorf("expected v2, got %q", string(record.PureRender))
	}
	if record.TemplateHash != "hash2" {
		t.Errorf("expected hash2, got %q", record.TemplateHash)
	}
}

func TestGetRenderHistory(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tmplPath := testTemplate
	for i := range 5 {
		if err := store.SaveRender(ctx, tmplPath, []byte("v"+string(rune('0'+i))), "hash", "linux", "host"); err != nil {
			t.Fatal(err)
		}
	}

	// Get last 3
	records, err := store.GetRenderHistory(ctx, tmplPath, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Should be in reverse chronological order (newest first)
	if records[0].ID < records[1].ID || records[1].ID < records[2].ID {
		t.Error("records not in reverse chronological order")
	}
}

func TestGetRenderByID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SaveRender(ctx, testTemplate, []byte("content"), "hash", "linux", "host"); err != nil {
		t.Fatal(err)
	}

	latest, err := store.GetLatestRender(ctx, testTemplate)
	if err != nil {
		t.Fatal(err)
	}

	record, err := store.GetRenderByID(ctx, latest.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record == nil {
		t.Fatal("expected record, got nil")
	}
	if string(record.PureRender) != "content" {
		t.Errorf("expected content, got %q", string(record.PureRender))
	}
}

func TestGetRenderByID_NotFound(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	record, err := store.GetRenderByID(ctx, 9999)
	if err != nil {
		t.Fatal(err)
	}
	if record != nil {
		t.Error("expected nil for non-existent ID")
	}
}

func TestPruneHistory(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tmplPath := testTemplate
	for range 10 {
		if err := store.SaveRender(ctx, tmplPath, []byte("content"), "hash", "linux", "host"); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.PruneHistory(ctx, tmplPath, 3); err != nil {
		t.Fatal(err)
	}

	records, err := store.GetRenderHistory(ctx, tmplPath, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 records after prune, got %d", len(records))
	}
}

func TestRemoveTemplate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tmplPath := testTemplate
	if err := store.SaveRender(ctx, tmplPath, []byte("v1"), "h1", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRender(ctx, tmplPath, []byte("v2"), "h2", "linux", "host"); err != nil {
		t.Fatal(err)
	}

	if err := store.RemoveTemplate(ctx, tmplPath); err != nil {
		t.Fatal(err)
	}

	record, err := store.GetLatestRender(ctx, tmplPath)
	if err != nil {
		t.Fatal(err)
	}
	if record != nil {
		t.Error("expected nil after removal")
	}
}

func TestRemoveTemplate_DoesNotAffectOthers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SaveRender(ctx, "a.tmpl", []byte("a"), "h", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRender(ctx, "b.tmpl", []byte("b"), "h", "linux", "host"); err != nil {
		t.Fatal(err)
	}

	if err := store.RemoveTemplate(ctx, "a.tmpl"); err != nil {
		t.Fatal(err)
	}

	record, err := store.GetLatestRender(ctx, "b.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if record == nil {
		t.Error("b.tmpl should still exist")
	}
}

func TestSchemaMigration_Version0To1(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), ".tidydots.db")
	ctx := context.Background()

	// Open creates schema from scratch (version 0 -> 1)
	store, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}

	version := store.getSchemaVersion(ctx)
	if version != 1 {
		t.Errorf("expected version 1, got %d", version)
	}

	_ = store.Close() //nolint:errcheck // cleanup is best-effort

	// Re-open should not re-run migrations
	store2, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store2.Close() }() //nolint:errcheck // cleanup is best-effort

	version = store2.getSchemaVersion(ctx)
	if version != 1 {
		t.Errorf("expected version 1 after re-open, got %d", version)
	}
}
