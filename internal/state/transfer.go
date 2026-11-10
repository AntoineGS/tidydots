package state

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"time"
)

// RenderImport maps one source database-local row ID to its destination-local
// row ID. The mapping is durably stored with the imported record.
type RenderImport struct {
	SourceID      int64
	DestinationID int64
}

// ExportRenders returns every record whose template_path exactly matches one
// of keys, in ascending source database ID order. An empty key list exports
// nothing. Rows are snapshots: pass these exact records to DeleteRenders so
// later rows appended under an exported key are not deleted.
func (s *Store) ExportRenders(ctx context.Context, keys []string) ([]RenderRecord, error) {
	if len(keys) == 0 {
		return []RenderRecord{}, nil
	}
	unique := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		unique[key] = struct{}{}
	}
	uniqueKeys := make([]string, 0, len(unique))
	for key := range unique {
		uniqueKeys = append(uniqueKeys, key)
	}
	sort.Strings(uniqueKeys)

	records := make([]RenderRecord, 0)
	for _, key := range uniqueKeys {
		rows, err := s.db.QueryContext(ctx, `SELECT id, template_path, pure_render, template_hash, rendered_at, platform_os, platform_host
			FROM template_renders WHERE template_path = ? ORDER BY id`, key)
		if err != nil {
			return nil, fmt.Errorf("exporting render history: %w", err)
		}
		for rows.Next() {
			var record RenderRecord
			var renderedAt string
			if err := rows.Scan(&record.ID, &record.TemplatePath, &record.PureRender, &record.TemplateHash, &renderedAt, &record.PlatformOS, &record.PlatformHost); err != nil {
				_ = rows.Close() //nolint:errcheck,gosec // preserve the scan error
				return nil, fmt.Errorf("scanning render history: %w", err)
			}
			record.RenderedAt, err = parseTime(renderedAt)
			if err != nil {
				_ = rows.Close() //nolint:errcheck,gosec // preserve the parse error
				return nil, fmt.Errorf("parsing render timestamp: %w", err)
			}
			records = append(records, record)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close() //nolint:errcheck,gosec // preserve the iteration error
			return nil, fmt.Errorf("reading render history: %w", err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("closing render history query: %w", err)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records, nil
}

// ImportRenders imports one journaled transfer atomically. transactionID must
// be a stable, unique journal identity. Source records are inserted in source
// ID order with destination-local IDs; the receipt maps source IDs to those
// IDs in the same SQLite transaction as the rows. Replaying the same identity
// and records returns the original mapping without duplicating history. A
// replay with different data fails. Any pre-existing destination history for
// an imported template_path is rejected, conservatively avoiding key merges.
func (s *Store) ImportRenders(ctx context.Context, transactionID string, records []RenderRecord) ([]RenderImport, error) {
	if transactionID == "" {
		return nil, fmt.Errorf("render import transaction ID must not be empty")
	}
	ordered := append([]RenderRecord(nil), records...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	if err := validateImportRecords(ordered); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning render import: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck,gosec // commit makes rollback a no-op

	var receiptCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM render_imports WHERE transaction_id = ?`, transactionID).Scan(&receiptCount); err != nil {
		return nil, fmt.Errorf("checking render import receipt: %w", err)
	}
	if receiptCount > 0 || len(ordered) == 0 {
		mapping, err := replayImport(ctx, tx, transactionID, ordered, receiptCount)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("committing render import replay: %w", err)
		}
		return mapping, nil
	}

	mapping, err := importFreshRenders(ctx, tx, transactionID, ordered)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing render import: %w", err)
	}
	return mapping, nil
}

func validateImportRecords(records []RenderRecord) error {
	for i, record := range records {
		if record.ID < 1 || record.RenderedAt.IsZero() || record.TemplatePath == "" {
			return fmt.Errorf("invalid render record for import: source ID %d", record.ID)
		}
		if i > 0 && records[i-1].ID == record.ID {
			return fmt.Errorf("duplicate source render ID %d", record.ID)
		}
	}
	return nil
}

func importFreshRenders(ctx context.Context, tx *sql.Tx, transactionID string, records []RenderRecord) ([]RenderImport, error) {
	seenKeys := make(map[string]struct{})
	for _, record := range records {
		seenKeys[record.TemplatePath] = struct{}{}
	}
	for key := range seenKeys {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM template_renders WHERE template_path = ?`, key).Scan(&count); err != nil {
			return nil, fmt.Errorf("checking destination render key %q: %w", key, err)
		}
		if count != 0 {
			return nil, fmt.Errorf("destination already has render history for %q", key)
		}
	}
	mapping := make([]RenderImport, 0, len(records))
	for _, record := range records {
		mappingItem, err := insertImportedRender(ctx, tx, transactionID, record)
		if err != nil {
			return nil, err
		}
		mapping = append(mapping, mappingItem)
	}
	return mapping, nil
}

func insertImportedRender(ctx context.Context, tx *sql.Tx, transactionID string, record RenderRecord) (RenderImport, error) {
	timestamp := record.RenderedAt.Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `INSERT INTO template_renders
		(template_path, pure_render, template_hash, rendered_at, platform_os, platform_host)
		VALUES (?, ?, ?, ?, ?, ?)`, record.TemplatePath, record.PureRender, record.TemplateHash,
		timestamp, record.PlatformOS, record.PlatformHost)
	if err != nil {
		return RenderImport{}, fmt.Errorf("importing source render ID %d: %w", record.ID, err)
	}
	destinationID, err := result.LastInsertId()
	if err != nil {
		return RenderImport{}, fmt.Errorf("reading imported render ID: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO render_imports
		(transaction_id, source_id, destination_id, fingerprint)
		VALUES (?, ?, ?, ?)`, transactionID, record.ID, destinationID, renderFingerprint(record))
	if err != nil {
		return RenderImport{}, fmt.Errorf("recording imported render ID %d: %w", record.ID, err)
	}
	return RenderImport{SourceID: record.ID, DestinationID: destinationID}, nil
}

func replayImport(ctx context.Context, tx *sql.Tx, transactionID string, records []RenderRecord, receiptCount int) ([]RenderImport, error) {
	if receiptCount != len(records) {
		return nil, fmt.Errorf("render import %q replay does not match its durable receipt", transactionID)
	}
	mapping := make([]RenderImport, 0, len(records))
	for _, record := range records {
		var destinationID int64
		var fingerprint []byte
		if err := tx.QueryRowContext(ctx, `SELECT destination_id, fingerprint
			FROM render_imports WHERE transaction_id = ? AND source_id = ?`, transactionID, record.ID).
			Scan(&destinationID, &fingerprint); err != nil {
			return nil, fmt.Errorf("reading render import receipt for source ID %d: %w", record.ID, err)
		}
		if !bytes.Equal(fingerprint, renderFingerprint(record)) {
			return nil, fmt.Errorf("render import %q replay differs at source ID %d", transactionID, record.ID)
		}
		imported, err := queryRenderByID(ctx, tx, destinationID)
		if err != nil {
			return nil, fmt.Errorf("checking imported render ID %d: %w", destinationID, err)
		}
		if imported == nil || !sameRenderPayload(*imported, record) || !bytes.Equal(fingerprint, renderFingerprint(*imported)) {
			return nil, fmt.Errorf("imported render ID %d changed after transaction", destinationID)
		}
		mapping = append(mapping, RenderImport{SourceID: record.ID, DestinationID: destinationID})
	}
	return mapping, nil
}

// DeleteRenders removes only the exact rows in a previously exported snapshot.
// transactionID is the stable journal identity. Deletion and its durable
// receipt commit atomically, making replay after a SQLite commit a no-op.
// Missing or changed rows abort the atomic delete; rows appended later under a
// key are untouched. It is intended for committed source cleanup.
func (s *Store) DeleteRenders(ctx context.Context, transactionID string, snapshot []RenderRecord) error {
	if len(snapshot) == 0 {
		return nil
	}
	if transactionID == "" {
		return fmt.Errorf("render deletion transaction ID must not be empty")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning render deletion: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck,gosec // commit makes rollback a no-op
	var receiptCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM render_deletions WHERE transaction_id = ?`, transactionID).Scan(&receiptCount); err != nil {
		return fmt.Errorf("checking render deletion receipt: %w", err)
	}
	if receiptCount > 0 {
		if err := verifyDeletionReplay(ctx, tx, transactionID, snapshot, receiptCount); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err := deleteSnapshotRows(ctx, tx, transactionID, snapshot); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing render deletion: %w", err)
	}
	return nil
}

func verifyDeletionReplay(ctx context.Context, tx *sql.Tx, transactionID string, snapshot []RenderRecord, receiptCount int) error {
	if receiptCount != len(snapshot) {
		return fmt.Errorf("render deletion %q replay does not match its durable receipt", transactionID)
	}
	for _, expected := range snapshot {
		var fingerprint []byte
		var sourceID int64
		if err := tx.QueryRowContext(ctx, `SELECT source_id, fingerprint
			FROM render_deletions WHERE transaction_id = ? AND source_id = ?`, transactionID, expected.ID).
			Scan(&sourceID, &fingerprint); err != nil {
			return fmt.Errorf("reading render deletion receipt for ID %d: %w", expected.ID, err)
		}
		if sourceID != expected.ID || !bytes.Equal(fingerprint, renderFingerprint(expected)) {
			return fmt.Errorf("render deletion %q replay differs at ID %d", transactionID, expected.ID)
		}
	}
	return nil
}

func deleteSnapshotRows(ctx context.Context, tx *sql.Tx, transactionID string, snapshot []RenderRecord) error {
	seen := make(map[int64]struct{}, len(snapshot))
	for _, expected := range snapshot {
		if _, duplicate := seen[expected.ID]; duplicate {
			return fmt.Errorf("duplicate source render ID %d in deletion snapshot", expected.ID)
		}
		seen[expected.ID] = struct{}{}
		actual, err := queryRenderByID(ctx, tx, expected.ID)
		if err != nil {
			return fmt.Errorf("revalidating render ID %d: %w", expected.ID, err)
		}
		if actual == nil || !sameRender(*actual, expected) {
			return fmt.Errorf("render ID %d changed since it was snapshotted", expected.ID)
		}
	}
	for _, expected := range snapshot {
		_, err := tx.ExecContext(ctx, `INSERT INTO render_deletions
			(transaction_id, source_id, fingerprint)
			VALUES (?, ?, ?)`, transactionID, expected.ID, renderFingerprint(expected))
		if err != nil {
			return fmt.Errorf("recording render deletion ID %d: %w", expected.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM template_renders WHERE id = ?`, expected.ID); err != nil {
			return fmt.Errorf("deleting render ID %d: %w", expected.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM render_imports WHERE destination_id = ?`, expected.ID); err != nil {
			return fmt.Errorf("removing prior import receipts for render ID %d: %w", expected.ID, err)
		}
	}
	return nil
}

// DeleteImportedRenders removes exactly the rows named by a transfer receipt.
// It is a pre-commit rollback operation; expected content is checked before any
// row is removed. Repeating rollback after the receipt was removed is a no-op.
func (s *Store) DeleteImportedRenders(ctx context.Context, transactionID string) error {
	if transactionID == "" {
		return fmt.Errorf("render import transaction ID must not be empty")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning imported render rollback: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck,gosec // commit makes rollback a no-op
	imports, err := readImportedRows(ctx, tx, transactionID)
	if err != nil {
		return err
	}
	if err := verifyImportedRows(ctx, tx, imports); err != nil {
		return err
	}
	for _, item := range imports {
		if _, err := tx.ExecContext(ctx, `DELETE FROM template_renders WHERE id = ?`, item.destinationID); err != nil {
			return fmt.Errorf("removing imported render ID %d: %w", item.destinationID, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM render_imports WHERE transaction_id = ?`, transactionID); err != nil {
		return fmt.Errorf("removing render import receipt: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing imported render rollback: %w", err)
	}
	return nil
}

type importedRow struct {
	destinationID int64
	sourceID      int64
	fingerprint   []byte
}

func readImportedRows(ctx context.Context, tx *sql.Tx, transactionID string) ([]importedRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT source_id, destination_id, fingerprint
		FROM render_imports WHERE transaction_id = ? ORDER BY source_id`, transactionID)
	if err != nil {
		return nil, fmt.Errorf("reading render import receipt: %w", err)
	}
	imports := make([]importedRow, 0)
	for rows.Next() {
		var item importedRow
		if err := rows.Scan(&item.sourceID, &item.destinationID, &item.fingerprint); err != nil {
			_ = rows.Close() //nolint:errcheck,gosec // preserve scan error
			return nil, fmt.Errorf("scanning render import receipt: %w", err)
		}
		imports = append(imports, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close() //nolint:errcheck,gosec // preserve iteration error
		return nil, fmt.Errorf("reading render import receipt: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing render import receipt: %w", err)
	}
	return imports, nil
}

func verifyImportedRows(ctx context.Context, tx *sql.Tx, imports []importedRow) error {
	for _, item := range imports {
		actual, err := queryRenderByID(ctx, tx, item.destinationID)
		if err != nil {
			return fmt.Errorf("revalidating imported render ID %d: %w", item.destinationID, err)
		}
		if actual == nil || !bytes.Equal(renderFingerprint(*actual), item.fingerprint) {
			return fmt.Errorf("imported render ID %d changed; refusing rollback", item.destinationID)
		}
	}
	return nil
}

type renderQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func queryRenderByID(ctx context.Context, queryer renderQuery, id int64) (*RenderRecord, error) {
	var record RenderRecord
	var renderedAt string
	err := queryer.QueryRowContext(ctx, `SELECT id, template_path, pure_render, template_hash, rendered_at, platform_os, platform_host
		FROM template_renders WHERE id = ?`, id).
		Scan(&record.ID, &record.TemplatePath, &record.PureRender, &record.TemplateHash, &renderedAt, &record.PlatformOS, &record.PlatformHost)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // nil denotes a missing expected row
	}
	if err != nil {
		return nil, err
	}
	record.RenderedAt, err = parseTime(renderedAt)
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func sameRender(left, right RenderRecord) bool {
	return left.ID == right.ID && sameRenderPayload(left, right)
}

func sameRenderPayload(left, right RenderRecord) bool {
	return left.TemplatePath == right.TemplatePath &&
		bytes.Equal(left.PureRender, right.PureRender) && left.TemplateHash == right.TemplateHash &&
		left.RenderedAt.Equal(right.RenderedAt) && left.PlatformOS == right.PlatformOS &&
		left.PlatformHost == right.PlatformHost
}

// renderFingerprint hashes every non-ID row field as length-delimited bytes.
// IDs are stored independently in the receipt because database row IDs are
// local to each database. UTC RFC3339Nano gives timestamps one canonical form.
func renderFingerprint(record RenderRecord) []byte {
	hash := sha256.New()
	writeFingerprintField(hash, []byte(record.TemplatePath))
	writeFingerprintField(hash, record.PureRender)
	writeFingerprintField(hash, []byte(record.TemplateHash))
	writeFingerprintField(hash, []byte(record.RenderedAt.UTC().Format(time.RFC3339Nano)))
	writeFingerprintField(hash, []byte(record.PlatformOS))
	writeFingerprintField(hash, []byte(record.PlatformHost))
	return hash.Sum(nil)
}

func writeFingerprintField(hash interface{ Write([]byte) (int, error) }, value []byte) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = hash.Write(length[:]) // hash.Hash writes cannot fail
	_, _ = hash.Write(value)     // hash.Hash writes cannot fail
}
