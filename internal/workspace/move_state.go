package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/state"
)

const stateRemapPrefix = "move-state-remap:"

// planRenderTransfer attributes history by inspecting the stored keys. Copy
// tuple targets are retained exactly: they encode the path used by each host,
// platform, and target when that render was produced.
//
//nolint:gocyclo // ordered attribution handles exact, tuple, and legacy keys.
func planRenderTransfer(ctx context.Context, d *moveData, _ *platform.Platform) error {
	var templates []fileSnapshot
	for _, file := range d.sourceFiles {
		if strings.HasSuffix(file.rel, ".tmpl") && !strings.HasSuffix(file.rel, ".tmpl.rendered") && !strings.HasSuffix(file.rel, ".tmpl.conflict") {
			templates = append(templates, file)
		}
	}
	if len(templates) == 0 {
		return nil
	}
	selected := make(map[string]string, len(templates))
	legacyCandidates := make(map[string][]string, len(templates))
	backup := filepath.Join(d.source.Root, d.backupRel)
	for _, file := range templates {
		abs := filepath.Join(backup, file.rel)
		rel, err := filepath.Rel(d.source.Root, abs)
		if err != nil {
			return fmt.Errorf("resolve moved template source key: %w", err)
		}
		sourceKey := "./" + filepath.ToSlash(rel)
		selected[sourceKey] = sourceKey
		legacyCandidates[filepath.ToSlash(file.rel)] = append(legacyCandidates[filepath.ToSlash(file.rel)], sourceKey)
	}
	for sourceKey := range selected {
		d.stateKeys = append(d.stateKeys, sourceKey)
	}
	store, err := state.OpenReadOnly(ctx, filepath.Join(d.source.Root, ".tidydots.db"))
	if err != nil {
		return fmt.Errorf("inspect source render history: %w", err)
	}
	if store == nil {
		d.stateKeys = uniqueStrings(d.stateKeys)
		return nil
	}
	defer func() { _ = store.Close() }() //nolint:errcheck,gosec // best-effort close after snapshot

	all, err := store.EnumerateRenders(ctx)
	if err != nil {
		return fmt.Errorf("enumerate source render history: %w", err)
	}
	for _, record := range all {
		if _, ok := selected[record.TemplatePath]; ok {
			d.stateRecords = append(d.stateRecords, record)
			d.stateKeys = append(d.stateKeys, record.TemplatePath)
			continue
		}
		if sourceKey, target, ok := parseCopyRenderKey(record.TemplatePath); ok {
			if _, selectedSource := selected[sourceKey]; selectedSource {
				d.stateRecords = append(d.stateRecords, record)
				d.stateKeys = append(d.stateKeys, record.TemplatePath)
				continue
			}
			canonicalSource, matched, err := attributeLegacySource(d, sourceKey, legacyCandidates)
			if err != nil {
				return err
			}
			if matched {
				remappedKey := fmt.Sprintf("copy:%q:%q", canonicalSource, target)
				d.stateRecords = append(d.stateRecords, record)
				d.stateKeys = append(d.stateKeys, record.TemplatePath, encodeStateRemap(record.TemplatePath, remappedKey))
				continue
			}
		}
		if sourceKey, matched, err := attributeLegacySource(d, record.TemplatePath, legacyCandidates); err != nil {
			return err
		} else if matched {
			d.stateRecords = append(d.stateRecords, record)
			d.stateKeys = append(d.stateKeys, record.TemplatePath, encodeStateRemap(record.TemplatePath, sourceKey))
		}
	}
	d.stateKeys = uniqueStrings(d.stateKeys)
	return nil
}

func legacyKeyHasOtherSource(d *moveData, legacyKey, selectedSourceKey string) bool {
	if d.source.Config == nil {
		return false
	}
	selectedPath := filepath.Join(d.source.Root, filepath.FromSlash(strings.TrimPrefix(selectedSourceKey, "./")))
	for _, app := range d.source.Config.Applications {
		for _, entry := range app.Entries {
			if entryHasOtherLegacyTemplate(d.source, entry, legacyKey, selectedPath) {
				return true
			}
		}
	}
	return false
}

func entryHasOtherLegacyTemplate(repo *Repository, entry config.SubEntry, legacyKey, selectedPath string) bool {
	if entry.IsSetup() || entry.Backup == "" {
		return false
	}
	backup, err := staticBackup(repo, entry.Backup)
	if err != nil {
		// A non-static backup that selects the same legacy relative key could
		// have produced it, but cannot be ruled in or out from this snapshot.
		return (entry.IsFolder() || entrySelectsRelativeFile(entry, legacyKey)) && strings.HasSuffix(legacyKey, ".tmpl")
	}
	candidate := filepath.Join(backup, filepath.FromSlash(legacyKey))
	rel, err := filepath.Rel(backup, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	if filepath.Clean(candidate) == filepath.Clean(selectedPath) {
		return false
	}
	if !entry.IsFolder() && !entrySelectsRelativeFile(entry, legacyKey) {
		return false
	}
	return strings.HasSuffix(candidate, ".tmpl")
}

func entrySelectsRelativeFile(entry config.SubEntry, relative string) bool {
	for _, file := range entry.Files {
		if filepath.ToSlash(filepath.Clean(file)) == filepath.ToSlash(filepath.Clean(filepath.FromSlash(relative))) {
			return true
		}
	}
	return false
}

func attributeLegacySource(d *moveData, key string, candidates map[string][]string) (string, bool, error) {
	values := candidates[key]
	if len(values) == 0 {
		return "", false, nil
	}
	if len(values) != 1 || legacyKeyHasOtherSource(d, key, values[0]) {
		return "", false, fmt.Errorf("ambiguous legacy render history key %q may refer to multiple repository sources", key)
	}
	return values[0], true, nil
}

func parseCopyRenderKey(key string) (string, string, bool) {
	if !strings.HasPrefix(key, "copy:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(key, "copy:")
	source, rest, ok := parseQuotedField(rest)
	if !ok || !strings.HasPrefix(rest, ":") {
		return "", "", false
	}
	target, rest, ok := parseQuotedField(strings.TrimPrefix(rest, ":"))
	if !ok || rest != "" || source == "" || target == "" {
		return "", "", false
	}
	return source, target, true
}

func parseQuotedField(value string) (string, string, bool) {
	if len(value) == 0 || value[0] != '"' {
		return "", "", false
	}
	escaped := false
	for i := 1; i < len(value); i++ {
		if escaped {
			escaped = false
			continue
		}
		if value[i] == '\\' {
			escaped = true
			continue
		}
		if value[i] == '"' {
			field, err := strconv.Unquote(value[:i+1])
			if err != nil {
				return "", "", false
			}
			return field, value[i+1:], true
		}
	}
	return "", "", false
}

func encodeStateRemap(from, to string) string {
	return stateRemapPrefix + strconv.Quote(from) + ":" + strconv.Quote(to)
}

func decodeStateRemap(value string) (from, to string, ok bool) {
	if !strings.HasPrefix(value, stateRemapPrefix) {
		return "", "", false
	}
	from, rest, ok := parseQuotedField(strings.TrimPrefix(value, stateRemapPrefix))
	if !ok || !strings.HasPrefix(rest, ":") {
		return "", "", false
	}
	to, rest, ok = parseQuotedField(strings.TrimPrefix(rest, ":"))
	return from, to, ok && rest == ""
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func cloneRenderRecords(records []state.RenderRecord) []state.RenderRecord {
	out := make([]state.RenderRecord, len(records))
	for i, record := range records {
		out[i] = record
		out[i].PureRender = append([]byte(nil), record.PureRender...)
	}
	return out
}

func destinationMoveStateRecords(j *moveJournal) []state.RenderRecord {
	records := cloneRenderRecords(j.StateRecords)
	remaps := stateRemaps(j.StateKeys)
	for i := range records {
		if destinationKey, ok := remaps[records[i].TemplatePath]; ok {
			records[i].TemplatePath = destinationKey
		}
	}
	return records
}

func stateRemaps(keys []string) map[string]string {
	remaps := make(map[string]string)
	for _, key := range keys {
		from, to, ok := decodeStateRemap(key)
		if ok {
			remaps[from] = to
		}
	}
	return remaps
}

func importMoveState(ctx context.Context, journalID string, j *moveJournal) error {
	if len(j.StateRecords) == 0 {
		return nil
	}
	path := filepath.Join(j.DestinationRoot, ".tidydots.db")
	store, err := state.Open(ctx, path)
	if err != nil {
		return fmt.Errorf("open destination render history: %w", err)
	}
	defer func() { _ = store.Close() }() //nolint:errcheck,gosec // best-effort close after import
	if _, err := store.ImportRenders(ctx, journalID, destinationMoveStateRecords(j)); err != nil {
		return fmt.Errorf("transfer render history: %w", err)
	}
	return nil
}

func rollbackMoveState(ctx context.Context, j *moveJournal) error {
	if len(j.StateRecords) == 0 {
		return nil
	}
	path := filepath.Join(j.DestinationRoot, ".tidydots.db")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	store, err := state.Open(ctx, path)
	if err != nil {
		return fmt.Errorf("open destination render history for rollback: %w", err)
	}
	defer func() { _ = store.Close() }() //nolint:errcheck,gosec // preserve rollback result
	if err := store.DeleteImportedRenders(ctx, j.ID); err != nil {
		return err
	}
	// Keep the database and all SQLite sidecars. Once created, the database may
	// contain unrelated concurrent history or be open in another process.
	return nil
}

// revalidateMoveSourceState detects additions, deletions, or edits to any
// selected source/copy history after planning. Call before commit publication.
//
//nolint:gocyclo // source-attribution rules differ for canonical and legacy persisted keys.
func revalidateMoveSourceState(ctx context.Context, j *moveJournal) error {
	if len(j.StateKeys) == 0 && len(j.StateRecords) == 0 {
		return nil
	}
	store, err := state.OpenReadOnly(ctx, filepath.Join(j.SourceRoot, ".tidydots.db"))
	if err != nil {
		return fmt.Errorf("revalidate source render history: %w", err)
	}
	if store == nil {
		if len(j.StateRecords) == 0 {
			return nil
		}
		return fmt.Errorf("source render history disappeared after planning")
	}
	defer func() { _ = store.Close() }() //nolint:errcheck,gosec // read-only snapshot cleanup
	all, err := store.EnumerateRenders(ctx)
	if err != nil {
		return fmt.Errorf("enumerate current source render history: %w", err)
	}
	selected := selectedSourceKeys(j.StateKeys)
	legacy := legacyRemaps(j.StateKeys, j.SourceRel)
	current := make([]state.RenderRecord, 0)
	for _, record := range all {
		if _, ok := selected[record.TemplatePath]; ok {
			current = append(current, record)
			continue
		}
		if source, _, ok := parseCopyRenderKey(record.TemplatePath); ok {
			_, selectedSource := selected[source]
			_, legacySource := legacy[source]
			if selectedSource || legacySource {
				current = append(current, record)
				continue
			}
		}
		if _, ok := legacy[record.TemplatePath]; ok {
			current = append(current, record)
		}
	}
	expected := j.StateRecords
	if j.StateCleaned {
		expected = nil
	}
	if !sameRenderSnapshots(current, expected) {
		// A crash may occur after the atomic source-row deletion commits but
		// before the journal records StateCleaned. The cleanup intent identifies
		// that replay window; accept only a fully empty attributed set here.
		if j.Intent == "cleanup-state" && len(current) == 0 {
			return nil
		}
		return fmt.Errorf("source render history changed after planning; refusing move commit")
	}
	return nil
}

func selectedSourceKeys(keys []string) map[string]struct{} {
	selected := make(map[string]struct{})
	for _, key := range keys {
		if _, _, remap := decodeStateRemap(key); remap {
			continue
		}
		if strings.HasPrefix(key, "./") {
			if _, _, isCopy := parseCopyRenderKey(key); !isCopy {
				selected[key] = struct{}{}
			}
		}
	}
	return selected
}

func legacyRemaps(keys []string, backupRel string) map[string]string {
	remaps := make(map[string]string)
	for _, key := range keys {
		from, to, ok := decodeStateRemap(key)
		if ok {
			remaps[from] = to
			continue
		}
		if !strings.HasPrefix(key, "./") {
			continue
		}
		sourcePath := strings.TrimPrefix(key, "./")
		legacy, err := filepath.Rel(filepath.FromSlash(backupRel), filepath.FromSlash(sourcePath))
		if err == nil && legacy != ".." && !strings.HasPrefix(legacy, ".."+string(filepath.Separator)) {
			remaps[filepath.ToSlash(legacy)] = key
		}
	}
	return remaps
}

func sameRenderSnapshots(left, right []state.RenderRecord) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		a, b := left[i], right[i]
		if a.ID != b.ID || a.TemplatePath != b.TemplatePath || !bytes.Equal(a.PureRender, b.PureRender) ||
			a.TemplateHash != b.TemplateHash || !a.RenderedAt.Equal(b.RenderedAt) ||
			a.PlatformOS != b.PlatformOS || a.PlatformHost != b.PlatformHost {
			return false
		}
	}
	return true
}

// verifyCommittedMoveState checks the destination's durable fingerprint receipt
// and imported rows before committed cleanup may delete any source material.
func verifyCommittedMoveState(ctx context.Context, j *moveJournal) error {
	if len(j.StateRecords) == 0 {
		return nil
	}
	path := filepath.Join(j.DestinationRoot, ".tidydots.db")
	store, err := state.OpenReadOnly(ctx, path)
	if err != nil {
		return fmt.Errorf("inspect committed destination render history: %w", err)
	}
	if store == nil {
		return fmt.Errorf("committed destination render history is missing")
	}
	defer func() { _ = store.Close() }() //nolint:errcheck,gosec // read-only snapshot cleanup
	if err := store.VerifyImportedRenders(ctx, j.ID, destinationMoveStateRecords(j)); err != nil {
		return fmt.Errorf("verify committed destination render history: %w", err)
	}
	return nil
}

func cleanupMoveState(ctx context.Context, j *moveJournal) error {
	if len(j.StateRecords) == 0 {
		return nil
	}
	if err := verifyCommittedMoveState(ctx, j); err != nil {
		return err
	}
	path := filepath.Join(j.SourceRoot, ".tidydots.db")
	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return nil
	}
	store, err := state.Open(ctx, path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }() //nolint:errcheck,gosec // preserve cleanup result
	if err := store.DeleteRenders(ctx, j.ID, j.StateRecords); err != nil {
		return err
	}
	return nil
}
