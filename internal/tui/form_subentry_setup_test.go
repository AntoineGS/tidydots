package tui

import (
	"strings"
	"testing"
)

// setupSubItemIndex returns the index of the application's setup sub-entry row.
func setupSubItemIndex(t *testing.T, m *Model, appIdx int) int {
	t.Helper()

	for i := range m.Applications[appIdx].SubItems {
		if m.Applications[appIdx].SubItems[i].SubEntry.IsSetup() {
			return i
		}
	}

	t.Fatalf("application %q has no setup sub-entry", m.Applications[appIdx].Application.Name)

	return -1
}

// TestInitSubEntryForm_SetupEntry_RefusesToOpenTheForm guards a data-loss path.
// Setup entries became cursorable rows in the manage view, so `e` now reaches
// them — but the sub-entry form has no fields for check/run. Opening it and
// saving rewrites the entry to tidydots.yaml *without* them: the entry stops
// being a setup entry, re-validates clean, and the user's setup step silently
// ceases to exist. The form must refuse rather than destroy.
func TestInitSubEntryForm_SetupEntry_RefusesToOpenTheForm(t *testing.T) {
	cfg := setupOnlyConfig(configSubEntry(), setupSubEntry())
	m := NewModel(cfg, linuxPlatform(), false)

	subIdx := setupSubItemIndex(t, &m, 0)

	m.initSubEntryForm(0, subIdx)

	if m.subEntryForm != nil {
		t.Fatal("the form opened on a setup entry: saving it would drop check/run from tidydots.yaml")
	}

	if m.Screen == ScreenAddForm {
		t.Error("Screen = ScreenAddForm; the editable form must not be shown for a setup entry")
	}

	if !m.showingResults || len(m.results) != 1 {
		t.Fatalf("results = %+v (showing = %v), want exactly one message explaining the refusal",
			m.results, m.showingResults)
	}

	if m.results[0].Success {
		t.Error("the refusal is reported as a success; the edit did not happen")
	}

	if !strings.Contains(m.results[0].Message, "tidydots.yaml") {
		t.Errorf("message = %q, want it to tell the user where setup entries are edited", m.results[0].Message)
	}

	// The config in memory is untouched: nothing was rewritten on the way out.
	entry := cfg.Applications[0].Entries[1]
	if !entry.IsSetup() {
		t.Errorf("the entry is no longer a setup entry after the refused edit: %+v", entry)
	}
}

// TestInitSubEntryForm_ConfigEntry_StillOpens proves the guard is narrow: config
// entries are still editable.
func TestInitSubEntryForm_ConfigEntry_StillOpens(t *testing.T) {
	cfg := setupOnlyConfig(configSubEntry(), setupSubEntry())
	m := NewModel(cfg, linuxPlatform(), false)

	setupIdx := setupSubItemIndex(t, &m, 0)
	configIdx := 1 - setupIdx

	m.initSubEntryForm(0, configIdx)

	if m.subEntryForm == nil {
		t.Fatal("the form did not open on a config entry")
	}

	if m.subEntryForm.NameInput.Value() != "config-file" {
		t.Errorf("form opened on %q, want %q", m.subEntryForm.NameInput.Value(), "config-file")
	}

	if m.Screen != ScreenAddForm {
		t.Errorf("Screen = %v, want ScreenAddForm", m.Screen)
	}
}
