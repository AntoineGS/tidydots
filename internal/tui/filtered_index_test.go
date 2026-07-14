package tui

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
)

// logindLikeConfig mirrors the real shape that exposed the bug: an application
// whose name does NOT contain the search term, holding a config entry first and
// a setup entry second.
func logindLikeConfig() *config.Config {
	return &config.Config{
		Version:    3,
		BackupRoot: "/repo",
		Applications: []config.Application{
			{
				Name: "logind-config",
				Entries: []config.SubEntry{
					{
						Name:    "drop-ins",
						Backup:  "./Linux/systemd/logind.conf.d",
						Targets: map[string]string{"linux": "/etc/systemd/logind.conf.d"},
						Files:   []string{"hibernate-on-lid.conf"},
					},
					{
						Name:  "reload-logind",
						Check: map[string]string{"linux": "busctl ... | grep -q hibernate"},
						Run:   map[string]string{"linux": "systemctl reload systemd-logind"},
					},
				},
			},
		},
	}
}

// cursorToRow moves the table cursor to the row whose first cell contains name.
func cursorToRow(t *testing.T, m *Model, name string) {
	t.Helper()

	for i, row := range m.tableRows {
		if len(row.Data) > 0 && contains(row.Data[0], name) {
			m.tableCursor = i
			return
		}
	}

	t.Fatalf("no table row for %q; rows: %v", name, m.tableRows)
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) > 0 && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}

	return -1
}

// TestCursorLookup_UnderSubEntryFilter_ResolvesRealIndex is the regression guard
// for a mis-targeting bug.
//
// getSearchedApplications compacts each app's SubItems down to the entries that
// match the search, and flattenApplications stamped each row with its index into
// that FILTERED slice. getApplicationAtCursorFromTable resolves the app by name
// against the full m.Applications, then hands that filtered index back — so every
// caller indexed the FULL SubItems with a FILTERED index.
//
// Filtering on "reload" drops "drop-ins" (index 0), leaving "reload-logind" alone
// at filtered index 0 — but its real index is 1. Pressing `r` therefore acted on
// SubItems[0], the config entry: the TUI reported "Restored: ..." and the setup
// entry never ran. `d` and `e` share this lookup, so they deleted and edited the
// wrong entry too.
func TestCursorLookup_UnderSubEntryFilter_ResolvesRealIndex(t *testing.T) {
	m := NewModel(logindLikeConfig(), linuxPlatform(), false)

	// Expand the app so its sub-entries get rows, then filter on the SETUP entry's
	// name. The app name ("logind-config") does not contain "reload", so the app
	// itself does not match and only the setup entry survives the filter.
	m.Applications[0].Expanded = true
	m.searchText = "reload"
	m.rebuildTable()

	cursorToRow(t, &m, "reload-logind")

	appIdx, subIdx := m.getApplicationAtCursorFromTable()
	if appIdx != 0 {
		t.Fatalf("appIdx = %d, want 0", appIdx)
	}

	got := m.Applications[appIdx].SubItems[subIdx]
	if got.SubEntry.Name != "reload-logind" {
		t.Errorf("cursor on 'reload-logind' resolved to SubItems[%d] = %q; the action would hit the wrong entry",
			subIdx, got.SubEntry.Name)
	}

	if !got.SubEntry.IsSetup() {
		t.Errorf("resolved entry is not the setup entry: %+v", got.SubEntry)
	}
}

// TestCursorLookup_NoFilter_StillResolves proves the fix does not regress the
// unfiltered path, where filtered and real indices coincide.
func TestCursorLookup_NoFilter_StillResolves(t *testing.T) {
	m := NewModel(logindLikeConfig(), linuxPlatform(), false)
	m.Applications[0].Expanded = true
	m.rebuildTable()

	for _, name := range []string{"drop-ins", "reload-logind"} {
		cursorToRow(t, &m, name)

		appIdx, subIdx := m.getApplicationAtCursorFromTable()
		if appIdx < 0 || subIdx < 0 {
			t.Fatalf("%s: lookup returned (%d, %d)", name, appIdx, subIdx)
		}

		if got := m.Applications[appIdx].SubItems[subIdx].SubEntry.Name; got != name {
			t.Errorf("cursor on %q resolved to %q", name, got)
		}
	}
}

// TestCursorLookup_AppNameFilter_ResolvesRealIndex covers the case that masked the
// bug: filtering on the APP name keeps every sub-entry, so filtered and real
// indices coincide and the lookup appeared to work.
func TestCursorLookup_AppNameFilter_ResolvesRealIndex(t *testing.T) {
	m := NewModel(logindLikeConfig(), linuxPlatform(), false)
	m.Applications[0].Expanded = true
	m.searchText = "logind-config"
	m.rebuildTable()

	cursorToRow(t, &m, "reload-logind")

	appIdx, subIdx := m.getApplicationAtCursorFromTable()
	if got := m.Applications[appIdx].SubItems[subIdx].SubEntry.Name; got != "reload-logind" {
		t.Errorf("cursor on 'reload-logind' resolved to %q", got)
	}
}

// TestSelectionHighlight_UnderFilter_TargetsRealEntry is the regression guard
// for the highlight half of the filtered-index bug: TableRow carried a
// position in the FILTERED slice as its app identifier, and the styling path
// handed it to selection lookups keyed by model position — so under a filter
// the wrong rows rendered as selected. Rows now carry names, and the lookups
// take names.
func TestSelectionHighlight_UnderFilter_TargetsRealEntry(t *testing.T) {
	m := NewModel(logindLikeConfig(), linuxPlatform(), false)
	m.Applications[0].Expanded = true
	m.searchText = "reload"
	m.rebuildTable()

	cursorToRow(t, &m, "reload-logind")

	appIdx, subIdx := m.getApplicationAtCursorFromTable()
	m.toggleSubEntrySelection(appIdx, subIdx)

	row := m.tableRows[m.tableCursor]
	if row.SubName != "reload-logind" {
		t.Fatalf("row.SubName = %q, want %q", row.SubName, "reload-logind")
	}

	if !m.isSubEntrySelected(row.AppName, row.SubName) {
		t.Error("the selected row must report selected via its own identity fields")
	}

	if m.isSubEntrySelected("logind-config", "drop-ins") {
		t.Error("the sibling entry must not report selected")
	}
}
