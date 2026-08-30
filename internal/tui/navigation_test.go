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
}
