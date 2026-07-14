package tui

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/AntoineGS/tidydots/internal/config"
)

// deleteProbeConfig lists zebra BEFORE alpha, so the config-file order is the
// reverse of m.Applications' alphabetical order. Any index that crosses from
// the model to m.Config.Applications without a name lookup lands on the wrong
// application.
func deleteProbeConfig() *config.Config {
	return &config.Config{
		Version:    3,
		BackupRoot: "/repo",
		Applications: []config.Application{
			{
				Name: "zebra",
				Entries: []config.SubEntry{
					{Name: "conf-z", Backup: "./z", Targets: map[string]string{"linux": "~/.z"}},
				},
			},
			{
				Name: "alpha",
				Entries: []config.SubEntry{
					{Name: "conf-a", Backup: "./a", Targets: map[string]string{"linux": "~/.a"}},
				},
			},
		},
	}
}

// osSkewedEntriesConfig puts a windows-only entry first, so SubItems (which
// skips entries that do not apply to the running OS) is offset by one from
// app.Entries. A SubItems index used against app.Entries deletes the wrong
// entry even when app order matches.
func osSkewedEntriesConfig() *config.Config {
	return &config.Config{
		Version:    3,
		BackupRoot: "/repo",
		Applications: []config.Application{
			{
				Name: "editor",
				Entries: []config.SubEntry{
					{Name: "win-only", Backup: "./w", Targets: map[string]string{"windows": "~/w"}},
					{Name: "one", Backup: "./1", Targets: map[string]string{"linux": "~/.one"}},
					{Name: "two", Backup: "./2", Targets: map[string]string{"linux": "~/.two"}},
				},
			},
		},
	}
}

// confirmDelete sends the "y" keypress that the delete-confirmation dialog is
// waiting on and returns the updated model.
func confirmDelete(t *testing.T, m *Model) *Model {
	t.Helper()

	updated, _ := m.updateResults(tea.KeyPressMsg{Code: 'y', Text: "y"})

	got, ok := updated.(Model)
	if !ok {
		t.Fatalf("updateResults returned %T, want Model", updated)
	}

	if got.err != nil {
		t.Fatalf("delete failed: %v", got.err)
	}

	return &got
}

func configApplicationNames(cfg *config.Config) []string {
	names := make([]string, len(cfg.Applications))
	for i, app := range cfg.Applications {
		names[i] = app.Name
	}

	return names
}

// TestDeleteApp_NonAlphabeticalConfig_DeletesCursorApp is the regression guard
// for a destructive mis-targeting bug: the single-item delete confirmation
// passed the cursor's index into the alphabetically sorted m.Applications
// straight to deleteApplication, which slices m.Config.Applications — the
// unsorted config-file order. With zebra listed before alpha, deleting zebra
// (model index 1) removed config index 1: alpha.
func TestDeleteApp_NonAlphabeticalConfig_DeletesCursorApp(t *testing.T) {
	m, _ := modelOnDisk(t, deleteProbeConfig())

	cursorToRow(t, m, "zebra")
	m.confirmingDeleteApp = true

	got := confirmDelete(t, m)

	want := []string{"alpha"}
	if names := configApplicationNames(got.Config); !slices.Equal(names, want) {
		t.Fatalf("config after deleting zebra = %v, want %v (wrong application deleted)",
			names, want)
	}
}

// TestDeleteSubEntry_OSSkippedEntry_DeletesCursorEntry covers the sub-entry
// facet of the same bug: SubItems omits entries that do not apply to the
// running OS, so a SubItems index is NOT an app.Entries index. With a
// windows-only entry first, deleting "two" (SubItems index 1) removed
// Entries[1]: "one".
func TestDeleteSubEntry_OSSkippedEntry_DeletesCursorEntry(t *testing.T) {
	m, _ := modelOnDisk(t, osSkewedEntriesConfig())

	m.Applications[0].Expanded = true
	m.rebuildTable()

	cursorToRow(t, m, "two")
	m.confirmingDeleteSubEntry = true

	got := confirmDelete(t, m)

	entries := got.Config.Applications[0].Entries
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name
	}

	want := []string{"win-only", "one"}
	if !slices.Equal(names, want) {
		t.Fatalf("entries after deleting two = %v, want %v (wrong entry deleted)",
			names, want)
	}
}
