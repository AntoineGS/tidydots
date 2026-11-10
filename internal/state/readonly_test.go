package state

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOpenReadOnlyDoesNotCreate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".tidydots.db")
	store, err := OpenReadOnly(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if store != nil {
		t.Fatal("missing DB should yield nil store")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("read-only access wrote %v", entries)
	}
}

func TestOpenReadOnlyEnumeratesEveryRenderInDatabaseOrder(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), ".tidydots.db")
	writer, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		key, body, osName, host string
	}{
		{`copy:"./a.tmpl":"C:\\Users\\host\\config"`, "windows", "windows", "host-win"},
		{`copy:"./a.tmpl":"/home/other/config"`, "linux", "linux", "host-linux"},
	} {
		if err := writer.SaveRender(ctx, item.key, []byte(item.body), "hash", item.osName, item.host); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }() //nolint:errcheck // best-effort test cleanup
	got, err := reader.EnumerateRenders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID >= got[1].ID || got[0].PlatformHost != "host-win" || got[1].PlatformHost != "host-linux" {
		t.Fatalf("enumerated records = %#v, want all records in database ID order", got)
	}
}

func TestOpenReadOnlyReadsExistingWALSnapshotWithoutSideEffects(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tidydots.db")
	writer, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.SaveRender(context.Background(), "./wal.tmpl", []byte("uncheckpointed"), "hash", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	// Keep the writer open so the committed record remains in the WAL.
	walPath, shmPath := path+"-wal", path+"-shm"
	if _, err := os.Stat(walPath); err != nil {
		t.Fatalf("expected WAL file: %v", err)
	}
	before := fileSnapshot(t, path, walPath, shmPath)

	reader, err := OpenReadOnly(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if reader == nil {
		t.Fatal("existing DB returned nil store")
	}
	defer func() { _ = reader.Close() }() //nolint:errcheck // cleanup is best-effort
	got, err := reader.GetLatestRender(context.Background(), "./wal.tmpl", "linux", "host")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || string(got.PureRender) != "uncheckpointed" {
		t.Fatalf("WAL snapshot record = %#v", got)
	}
	if after := fileSnapshot(t, path, walPath, shmPath); !reflect.DeepEqual(before, after) {
		t.Fatalf("read-only inspection changed DB sidecars: before=%v after=%v", before, after)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func fileSnapshot(t *testing.T, paths ...string) map[string][]byte {
	t.Helper()
	result := make(map[string][]byte, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			result[path] = nil
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		result[path] = data
	}
	return result
}

func TestTransferRendersRoundTripAndReplay(t *testing.T) {
	ctx := context.Background()
	source := newTestStore(t)
	destination := newTestStore(t)
	when := time.Date(2025, 3, 4, 5, 6, 7, 123, time.UTC)
	if err := destination.SaveRender(ctx, "unrelated.tmpl", []byte("already present"), "u", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	for _, record := range []RenderRecord{
		{TemplatePath: "./app.tmpl", PureRender: []byte("linux-one"), TemplateHash: "a", RenderedAt: when, PlatformOS: "linux", PlatformHost: "host-a"},
		{TemplatePath: "./app.tmpl", PureRender: []byte("linux-two"), TemplateHash: "b", RenderedAt: when.Add(time.Second), PlatformOS: "linux", PlatformHost: "host-a"},
		{TemplatePath: "./app.tmpl", PureRender: []byte("windows"), TemplateHash: "c", RenderedAt: when, PlatformOS: "windows", PlatformHost: "host-b"},
		{TemplatePath: `copy:"./app.tmpl":"/home/a/config"`, PureRender: []byte("copy"), TemplateHash: "d", RenderedAt: when, PlatformOS: "linux", PlatformHost: "host-a"},
	} {
		if _, err := source.db.ExecContext(ctx, `INSERT INTO template_renders (template_path,pure_render,template_hash,rendered_at,platform_os,platform_host) VALUES (?,?,?,?,?,?)`, record.TemplatePath, record.PureRender, record.TemplateHash, record.RenderedAt.Format(time.RFC3339Nano), record.PlatformOS, record.PlatformHost); err != nil {
			t.Fatal(err)
		}
	}
	want, err := source.ExportRenders(ctx, []string{"./app.tmpl", `copy:"./app.tmpl":"/home/a/config"`})
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 4 || want[0].ID >= want[1].ID {
		t.Fatalf("export did not preserve full ordered history: %#v", want)
	}
	imports, err := destination.ImportRenders(ctx, "move-1", want)
	if err != nil {
		t.Fatal(err)
	}
	destinationPath := destination.Path()
	if err := destination.Close(); err != nil {
		t.Fatal(err)
	}
	destination, err = Open(ctx, destinationPath)
	if err != nil {
		t.Fatalf("reopening destination after committed import: %v", err)
	}
	t.Cleanup(func() { _ = destination.Close() }) //nolint:errcheck // cleanup is best-effort
	if err := destination.SaveRender(ctx, "unrelated.tmpl", []byte("unrelated"), "u", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	replayed, err := destination.ImportRenders(ctx, "move-1", want)
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if !reflect.DeepEqual(replayed, imports) {
		t.Fatalf("replay mapping changed: got %#v, want %#v", replayed, imports)
	}
	if err := destination.SaveRender(ctx, "./app.tmpl", []byte("later destination history"), "later", "linux", "new-host"); err != nil {
		t.Fatal(err)
	}
	changed := append([]RenderRecord(nil), want...)
	changed[0].TemplateHash = "different-replay"
	if _, err := destination.ImportRenders(ctx, "move-1", changed); err == nil {
		t.Fatal("replay with a changed snapshot unexpectedly succeeded")
	}
	got, err := destination.ExportRenders(ctx, []string{"./app.tmpl", `copy:"./app.tmpl":"/home/a/config"`})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want)+1 {
		t.Fatalf("round trip count = %d, want %d imported rows plus appended history: %#v", len(got), len(want), got)
	}
	for i, record := range got[:len(want)] {
		if record.ID != imports[i].DestinationID || record.TemplatePath != want[i].TemplatePath ||
			!reflect.DeepEqual(record.PureRender, want[i].PureRender) || record.TemplateHash != want[i].TemplateHash ||
			!record.RenderedAt.Equal(want[i].RenderedAt) || record.PlatformOS != want[i].PlatformOS || record.PlatformHost != want[i].PlatformHost {
			t.Fatalf("round trip record %d mismatch: got %#v, source %#v, mapping %#v", i, record, want[i], imports[i])
		}
	}
	if string(got[len(got)-1].PureRender) != "later destination history" {
		t.Fatalf("appended destination row missing: %#v", got[len(got)-1])
	}
	if empty, err := source.ExportRenders(ctx, nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty export = %v, %v", empty, err)
	}
	if err := source.DeleteRenders(ctx, "empty-delete", nil); err != nil {
		t.Fatalf("empty delete: %v", err)
	}
	if err := destination.DeleteImportedRenders(ctx, "move-1"); err != nil {
		t.Fatalf("rollback imported rows: %v", err)
	}
	if rows, err := destination.ExportRenders(ctx, []string{"./app.tmpl", `copy:"./app.tmpl":"/home/a/config"`}); err != nil || len(rows) != 1 || string(rows[0].PureRender) != "later destination history" {
		t.Fatalf("rollback removed later destination history or left imported rows: %v, %v", rows, err)
	}
}

func TestImportRendersCollisionIsAtomic(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	if err := store.SaveRender(ctx, "./collision.tmpl", []byte("existing"), "existing", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	var existing RenderRecord
	var renderedAt string
	if err := store.db.QueryRowContext(ctx, `SELECT id,template_path,pure_render,template_hash,rendered_at,platform_os,platform_host FROM template_renders`).Scan(&existing.ID, &existing.TemplatePath, &existing.PureRender, &existing.TemplateHash, &renderedAt, &existing.PlatformOS, &existing.PlatformHost); err != nil {
		t.Fatal(err)
	}
	parsedTime, err := parseTime(renderedAt)
	if err != nil {
		t.Fatal(err)
	}
	existing.RenderedAt = parsedTime
	conflict := existing
	conflict.PureRender = []byte("different")
	other := conflict
	other.ID = existing.ID + 1
	other.TemplatePath = "./must-rollback.tmpl"
	if _, err := store.ImportRenders(ctx, "collision", []RenderRecord{other, conflict}); err == nil {
		t.Fatal("expected destination key collision error")
	}
	rows, err := store.ExportRenders(ctx, []string{existing.TemplatePath, other.TemplatePath})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !reflect.DeepEqual(rows[0], existing) {
		t.Fatalf("collision import was not atomic: %#v", rows)
	}
}

func TestDeleteRendersDeletesOnlySnapshottedRows(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	if err := store.SaveRender(ctx, "./owned.tmpl", []byte("first"), "1", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.ExportRenders(ctx, []string{"./owned.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRender(ctx, "./owned.tmpl", []byte("later"), "2", "windows", "other-host"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteRenders(ctx, "delete-1", snapshot); err != nil {
		t.Fatal(err)
	}
	storePath := store.Path()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, storePath)
	if err != nil {
		t.Fatalf("reopening after committed deletion: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() }) //nolint:errcheck // cleanup is best-effort
	if err := store.SaveRender(ctx, "./owned.tmpl", []byte("after commit"), "3", "linux", "new-host"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteRenders(ctx, "delete-1", snapshot); err != nil {
		t.Fatalf("replaying committed deletion: %v", err)
	}
	remaining, err := store.ExportRenders(ctx, []string{"./owned.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 || string(remaining[0].PureRender) != "later" || string(remaining[1].PureRender) != "after commit" {
		t.Fatalf("cleanup removed rows outside snapshot: %#v", remaining)
	}
}

func TestDeleteRendersRejectsChangedSnapshotAtomically(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	for _, path := range []string{"./changed.tmpl", "./untouched.tmpl"} {
		if err := store.SaveRender(ctx, path, []byte(path), "hash", "linux", "host"); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := store.ExportRenders(ctx, []string{"./changed.tmpl", "./untouched.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE template_renders SET pure_render = ? WHERE id = ?`, []byte("external edit"), snapshot[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteRenders(ctx, "changed-snapshot", snapshot); err == nil {
		t.Fatal("changed snapshot unexpectedly deleted")
	}
	remaining, err := store.ExportRenders(ctx, []string{"./changed.tmpl", "./untouched.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 2 {
		t.Fatalf("stale snapshot deletion was not atomic: %#v", remaining)
	}
}

func TestDeleteImportedRendersRejectsModifiedImportedRow(t *testing.T) {
	ctx := context.Background()
	source := newTestStore(t)
	if err := source.SaveRender(ctx, "./rollback.tmpl", []byte("original"), "hash", "linux", "host"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.ExportRenders(ctx, []string{"./rollback.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	destination := newTestStore(t)
	if _, err := destination.ImportRenders(ctx, "rollback-modified", snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := destination.db.ExecContext(ctx, `UPDATE template_renders SET pure_render = ? WHERE template_path = ?`, []byte("external change"), "./rollback.tmpl"); err != nil {
		t.Fatal(err)
	}
	if err := destination.DeleteImportedRenders(ctx, "rollback-modified"); err == nil {
		t.Fatal("rollback removed a modified imported row")
	}
	rows, err := destination.ExportRenders(ctx, []string{"./rollback.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || string(rows[0].PureRender) != "external change" {
		t.Fatalf("modified imported row was not preserved: %#v", rows)
	}
}

func TestTransferCleanupLeavesNoPlaintextInOldStoreReceipts(t *testing.T) {
	ctx := context.Background()
	marker := []byte("distinctive-private-render-marker-7f1c9a")
	privateSource := newTestStore(t)
	if err := privateSource.SaveRender(ctx, "./private.tmpl", marker, "hash", "linux", "trusted-host"); err != nil {
		t.Fatal(err)
	}
	privateSnapshot, err := privateSource.ExportRenders(ctx, []string{"./private.tmpl"})
	if err != nil {
		t.Fatal(err)
	}

	public := newTestStore(t)
	if _, err := public.ImportRenders(ctx, "private-to-public", privateSnapshot); err != nil {
		t.Fatal(err)
	}
	publicSnapshot, err := public.ExportRenders(ctx, []string{"./private.tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	privateDestination := newTestStore(t)
	if _, err := privateDestination.ImportRenders(ctx, "public-to-private", publicSnapshot); err != nil {
		t.Fatal(err)
	}
	if err := public.DeleteRenders(ctx, "public-to-private", publicSnapshot); err != nil {
		t.Fatal(err)
	}
	if err := public.DeleteRenders(ctx, "public-to-private", publicSnapshot); err != nil {
		t.Fatalf("replaying committed cleanup: %v", err)
	}
	publicPath := public.Path()
	if err := public.Close(); err != nil {
		t.Fatal(err)
	}
	public, err = Open(ctx, publicPath)
	if err != nil {
		t.Fatalf("reopening old public store: %v", err)
	}
	t.Cleanup(func() { _ = public.Close() }) //nolint:errcheck // cleanup is best-effort
	for _, table := range []string{"template_renders", "render_imports", "render_deletions"} {
		if hasMarker := tableContainsValue(t, public, table, marker); hasMarker {
			t.Errorf("old public table %s retains private render plaintext", table)
		}
	}
	if count := tableRowCount(t, public, "render_imports"); count != 0 {
		t.Errorf("old import receipt rows remain after source cleanup: %d", count)
	}
}

func TestLegacyPlaintextReceiptsMigrateToFingerprints(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), ".tidydots.db")
	legacyDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := legacyDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateV1(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := migrateV2(ctx, tx); err != nil {
		t.Fatal(err)
	}
	marker := []byte("legacy-private-receipt-marker-d45a")
	importsTime := time.Date(2025, 7, 8, 9, 10, 11, 0, time.UTC)
	deletionsTime := importsTime.Add(time.Minute)
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version(version) VALUES (2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO template_renders
		(id,template_path,pure_render,template_hash,rendered_at,platform_os,platform_host)
		VALUES (17,?,?,?,?,?,?)`, "./legacy-import.tmpl", marker, "import-hash", importsTime.Format(time.RFC3339Nano), "linux", "host"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO render_imports
		(transaction_id,source_id,destination_id,template_path,pure_render,template_hash,rendered_at,platform_os,platform_host)
		VALUES (?,?,?,?,?,?,?,?,?)`, "legacy-import", 4, 17, "./legacy-import.tmpl", marker, "import-hash", importsTime.Format(time.RFC3339Nano), "linux", "host"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO render_deletions
		(transaction_id,source_id,template_path,pure_render,template_hash,rendered_at,platform_os,platform_host)
		VALUES (?,?,?,?,?,?,?,?)`, "legacy-delete", 5, "./legacy-delete.tmpl", marker, "delete-hash", deletionsTime.Format(time.RFC3339Nano), "windows", "other-host"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("migrating legacy receipts: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() }) //nolint:errcheck // cleanup is best-effort
	importSnapshot := RenderRecord{ID: 4, TemplatePath: "./legacy-import.tmpl", PureRender: marker, TemplateHash: "import-hash", RenderedAt: importsTime, PlatformOS: "linux", PlatformHost: "host"}
	mapping, err := store.ImportRenders(ctx, "legacy-import", []RenderRecord{importSnapshot})
	if err != nil || len(mapping) != 1 || mapping[0].DestinationID != 17 {
		t.Fatalf("legacy import receipt replay = %#v, %v", mapping, err)
	}
	deleteSnapshot := RenderRecord{ID: 5, TemplatePath: "./legacy-delete.tmpl", PureRender: marker, TemplateHash: "delete-hash", RenderedAt: deletionsTime, PlatformOS: "windows", PlatformHost: "other-host"}
	if err := store.DeleteRenders(ctx, "legacy-delete", []RenderRecord{deleteSnapshot}); err != nil {
		t.Fatalf("legacy deletion receipt replay: %v", err)
	}
	for _, table := range []string{"render_imports", "render_deletions"} {
		if hasMarker := tableContainsValue(t, store, table, marker); hasMarker {
			t.Errorf("migrated table %s retains plaintext receipt data", table)
		}
	}
}

func tableContainsValue(t *testing.T, store *Store, table string, marker []byte) bool {
	t.Helper()
	var query string
	switch table {
	case "template_renders":
		query = `SELECT * FROM template_renders`
	case "render_imports":
		query = `SELECT * FROM render_imports`
	case "render_deletions":
		query = `SELECT * FROM render_deletions`
	default:
		t.Fatalf("unexpected table name %q", table)
	}
	rows, err := store.db.QueryContext(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }() //nolint:errcheck // cleanup is best-effort
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		values := make([]any, len(columns))
		destinations := make([]any, len(columns))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			t.Fatal(err)
		}
		for _, value := range values {
			switch value := value.(type) {
			case []byte:
				if bytes.Contains(value, marker) {
					return true
				}
			case string:
				if bytes.Contains([]byte(value), marker) {
					return true
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}

func tableRowCount(t *testing.T, store *Store, table string) int {
	t.Helper()
	var query string
	switch table {
	case "render_imports":
		query = `SELECT COUNT(*) FROM render_imports`
	case "render_deletions":
		query = `SELECT COUNT(*) FROM render_deletions`
	default:
		t.Fatalf("unexpected table name %q", table)
	}
	var count int
	if err := store.db.QueryRowContext(context.Background(), query).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestSnapshotRejectsRollbackJournalCreatedAfterAttemptGuard(t *testing.T) {
	path, writer := newRollbackJournalTestDatabase(t)
	defer func() { _ = writer.Close() }() //nolint:errcheck // cleanup is best-effort
	destination := filepath.Join(t.TempDir(), "snapshot.db")
	var pending *sql.Tx
	err := copyDatabaseSnapshotWithHooks(path, destination, snapshotHooks{
		afterJournalCheck: func(attempt int) error {
			if attempt != 0 {
				return nil
			}
			pending = spillUncommittedUpdate(t, writer, path)
			return nil
		},
	})
	if pending != nil {
		defer func() { _ = pending.Rollback() }() //nolint:errcheck // cleanup is best-effort
	}
	if err == nil {
		t.Fatal("snapshot accepted an uncommitted rollback-journal database")
	}
	if pending == nil {
		t.Fatal("test did not create the rollback-journal transaction")
	}
}

func TestSnapshotChecksRollbackJournalBetweenRetries(t *testing.T) {
	path, writer := newRollbackJournalTestDatabase(t)
	defer func() { _ = writer.Close() }() //nolint:errcheck // cleanup is best-effort
	destination := filepath.Join(t.TempDir(), "snapshot.db")
	var pending *sql.Tx
	copyAttempts := 0
	err := copyDatabaseSnapshotWithHooks(path, destination, snapshotHooks{
		afterCopy: func(attempt int) error {
			copyAttempts++
			if attempt == 0 {
				info, err := os.Stat(path)
				if err != nil {
					return err
				}
				return os.Chtimes(path, info.ModTime().Add(time.Second), info.ModTime().Add(time.Second))
			}
			return nil
		},
		afterUnstableAttempt: func(attempt int) error {
			if attempt == 0 {
				pending = spillUncommittedUpdate(t, writer, path)
			}
			return nil
		},
	})
	if pending != nil {
		defer func() { _ = pending.Rollback() }() //nolint:errcheck // cleanup is best-effort
	}
	if err == nil {
		t.Fatal("retry accepted a rollback-journal database")
	}
	if pending == nil {
		t.Fatal("rollback journal was not created between attempts")
	}
	if copyAttempts != 1 {
		t.Fatalf("snapshot copied %d attempts; expected rejection before retry copy", copyAttempts)
	}
}

func newRollbackJournalTestDatabase(t *testing.T) (string, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".tidydots.db")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 32; i++ {
		if err := store.SaveRender(context.Background(), fmt.Sprintf("./spill-%02d.tmpl", i), bytes.Repeat([]byte("a"), 16*1024), "hash", "linux", "host"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	writer.SetMaxOpenConns(1)
	for _, statement := range []string{"PRAGMA journal_mode=DELETE", "PRAGMA cache_size=1", "PRAGMA cache_spill=ON"} {
		if _, err := writer.ExecContext(context.Background(), statement); err != nil {
			_ = writer.Close() //nolint:errcheck // preserve setup error
			t.Fatalf("%s: %v", statement, err)
		}
	}
	t.Cleanup(func() { _ = writer.Close() }) //nolint:errcheck // cleanup is best-effort
	return path, writer
}

func spillUncommittedUpdate(t *testing.T, writer *sql.DB, path string) *sql.Tx {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := writer.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(context.Background(), `UPDATE template_renders SET pure_render = ?`, []byte(strings.Repeat("UNCOMMITTED-PRIVATE-RENDER", 4096))); err != nil {
		_ = tx.Rollback() //nolint:errcheck // preserve setup error
		t.Fatal(err)
	}
	if _, err := os.Stat(path + "-journal"); err != nil {
		_ = tx.Rollback() //nolint:errcheck // preserve setup error
		t.Fatalf("expected rollback journal: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		_ = tx.Rollback() //nolint:errcheck // preserve setup error
		t.Fatal(err)
	}
	if bytes.Equal(before, after) {
		_ = tx.Rollback() //nolint:errcheck // preserve setup error
		t.Fatal("uncommitted update did not spill to the database file")
	}
	return tx
}
