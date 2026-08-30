package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
)

func TestVimMotionPaging(t *testing.T) {
	entries := make([]config.SubEntry, 12)
	for i := range entries {
		entries[i] = config.SubEntry{
			Name:    string(rune('a' + i)),
			Backup:  "./backup",
			Targets: map[string]string{"linux": "~/target"},
		}
	}
	m := NewModel(&config.Config{Version: 3, Applications: []config.Application{{Name: "app", Entries: entries}}}, linuxPlatform(), false)
	m.height = 20
	m.viewHeight = 10
	m.Applications[0].Expanded = true
	m.selectedApps = map[string]bool{"app": true}
	m.selectedSubEntries = map[subEntryKey]bool{{app: "app", sub: "a"}: true}
	m.multiSelectActive = true
	m.rebuildTable()
	maxVisible := m.computeMaxVisibleRows()
	if len(m.tableRows) <= maxVisible {
		t.Fatalf("test requires more rows than viewport: rows=%d viewport=%d", len(m.tableRows), maxVisible)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	wantHalf := maxVisible / 2
	if wantHalf < 1 {
		wantHalf = 1
	}
	if m.tableCursor != wantHalf {
		t.Errorf("ctrl+d cursor = %d, want %d", m.tableCursor, wantHalf)
	}
	if m.scrollOffset != 0 {
		t.Errorf("ctrl+d scroll = %d, want 0", m.scrollOffset)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m = updated.(Model)
	if m.tableCursor != 0 || m.scrollOffset != 0 {
		t.Errorf("ctrl+u = cursor %d, scroll %d, want 0, 0", m.tableCursor, m.scrollOffset)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if !m.pendingG {
		t.Fatal("first g should arm pendingG")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	m = updated.(Model)
	if m.pendingG {
		t.Fatal("non-g key should clear pendingG")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if !m.pendingG {
		t.Fatal("g should arm pendingG again")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if m.tableCursor != 0 || m.scrollOffset != 0 || m.pendingG {
		t.Errorf("gg = cursor %d, scroll %d, pendingG %t, want 0, 0, false", m.tableCursor, m.scrollOffset, m.pendingG)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	if m.tableCursor != len(m.tableRows)-1 {
		t.Errorf("G cursor = %d, want %d", m.tableCursor, len(m.tableRows)-1)
	}
	if want := len(m.tableRows) - maxVisible; m.scrollOffset != want {
		t.Errorf("G scroll = %d, want %d", m.scrollOffset, want)
	}
	if !m.selectedApps["app"] || !m.selectedSubEntries[subEntryKey{app: "app", sub: "a"}] {
		t.Error("navigation motions must not mutate multi-select state")
	}
}

func TestVimMotionPaging_EmptyTableKeepsCursorAndScrollZero(t *testing.T) {
	m := Model{Operation: OpList, Screen: ScreenResults, height: 24, tableRows: nil}

	for _, msg := range []tea.KeyPressMsg{
		{Code: 'd', Mod: tea.ModCtrl},
		{Code: 'u', Mod: tea.ModCtrl},
		{Code: 'G', Text: "G"},
		{Code: 'g', Text: "g"},
		{Code: 'g', Text: "g"},
	} {
		updated, _ := m.Update(msg)
		m = updated.(Model)
		if m.tableCursor != 0 || m.scrollOffset != 0 {
			t.Fatalf("key %q moved empty table to cursor=%d scroll=%d", msg.Text, m.tableCursor, m.scrollOffset)
		}
	}
}

func TestVimMotionPaging_OneRowAndRepeatedPagingClamp(t *testing.T) {
	m := Model{
		Operation:   OpList,
		Screen:      ScreenResults,
		height:      24,
		tableRows:   []TableRow{{AppName: "one"}},
		tableCursor: 0,
	}

	for i := 0; i < 3; i++ {
		updated, _ := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
		m = updated.(Model)
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
		m = updated.(Model)
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if m.tableCursor != 0 || m.scrollOffset != 0 {
		t.Errorf("one-row motions ended at cursor=%d scroll=%d", m.tableCursor, m.scrollOffset)
	}
}

func TestVimMotionPaging_ModalAndSearchIsolation(t *testing.T) {
	m := Model{
		Operation:              OpList,
		Screen:                 ScreenResults,
		height:                 24,
		tableRows:              []TableRow{{AppName: "one"}, {AppName: "two"}},
		Platform:               linuxPlatform(),
		confirmingFilterToggle: true,
		tableCursor:            1,
	}
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'g', Text: "g"})
	m = updated.(Model)
	if m.tableCursor != 1 || m.pendingG {
		t.Errorf("motion entered confirmation modal: cursor=%d pendingG=%t", m.tableCursor, m.pendingG)
	}

	m.confirmingFilterToggle = false
	m.searching = true
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	m = updated.(Model)
	if m.pendingG {
		t.Errorf("motion entered search input: pendingG=%t", m.pendingG)
	}
}

func actionFilterSelectionModel() Model {
	app := ApplicationItem{
		Application: config.Application{Name: "selected"},
		SubItems: []SubEntryItem{{
			AppName:  "selected",
			SubEntry: config.SubEntry{Name: "linked", Targets: map[string]string{"linux": "~/linked"}},
			State:    StateLinked,
			Index:    0,
		}},
	}
	m := Model{
		Applications:       []ApplicationItem{app},
		Platform:           linuxPlatform(),
		Operation:          OpList,
		Screen:             ScreenResults,
		filterEnabled:      false,
		selectedApps:       map[string]bool{"selected": true},
		selectedSubEntries: make(map[subEntryKey]bool),
		multiSelectActive:  true,
		height:             24,
		width:              100,
	}
	m.rebuildTable()
	return m
}

func TestActionFilterConfirmationCancelKeepsFilterDisabled(t *testing.T) {
	m := actionFilterSelectionModel()
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = updated.(Model)
	if !m.confirmingActionFilter || m.actionFilterEnabled {
		t.Fatalf("x should open action-filter confirmation: confirming=%t enabled=%t", m.confirmingActionFilter, m.actionFilterEnabled)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = updated.(Model)
	if m.confirmingActionFilter || m.actionFilterEnabled || !m.selectedApps["selected"] {
		t.Errorf("cancel changed state: confirming=%t enabled=%t selected=%t", m.confirmingActionFilter, m.actionFilterEnabled, m.selectedApps["selected"])
	}
}

func TestActionFilterConfirmationPreservesBatchSelections(t *testing.T) {
	m := actionFilterSelectionModel()
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = updated.(Model)

	if m.confirmingActionFilter || !m.actionFilterEnabled || !m.selectedApps["selected"] || !m.multiSelectActive {
		t.Errorf("confirm did not preserve batch selection: confirming=%t enabled=%t selected=%t active=%t", m.confirmingActionFilter, m.actionFilterEnabled, m.selectedApps["selected"], m.multiSelectActive)
	}
	if len(m.tableRows) != 0 {
		t.Errorf("action filter should hide the linked-only selected app, got %d rows", len(m.tableRows))
	}
}

func TestActionFilterConfirmationBlocksMotions(t *testing.T) {
	m := actionFilterSelectionModel()
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = updated.(Model)
	if !m.confirmingActionFilter || m.tableCursor != 0 || m.pendingG {
		t.Errorf("motion escaped action-filter modal: confirming=%t cursor=%d pendingG=%t", m.confirmingActionFilter, m.tableCursor, m.pendingG)
	}
}
