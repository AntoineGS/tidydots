package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func TestForceRestoreKeyBindingsMatchUppercaseOnly(t *testing.T) {
	upper := tea.KeyPressMsg{Code: 'R'}
	lower := tea.KeyPressMsg{Code: 'r'}

	if !key.Matches(upper, ListKeys.ForceRestore) || key.Matches(lower, ListKeys.ForceRestore) {
		t.Fatal("List Force Restore must match uppercase R only")
	}
	if !key.Matches(upper, MultiSelectKeys.ForceRestore) || key.Matches(lower, MultiSelectKeys.ForceRestore) {
		t.Fatal("multi-select Force Restore must match uppercase R only")
	}
	if !key.Matches(lower, ListKeys.Restore) || key.Matches(upper, ListKeys.Restore) {
		t.Fatal("List Restore must match lowercase r only")
	}
	if !key.Matches(lower, MultiSelectKeys.Restore) || key.Matches(upper, MultiSelectKeys.Restore) {
		t.Fatal("multi-select Restore must match lowercase r only")
	}
}

func TestForceRestoreAppearsInListHelp(t *testing.T) {
	m := Model{width: 120}
	help := m.renderHelpForCurrentState()
	if !strings.Contains(help, "fo") || !strings.Contains(help, "restore") {
		t.Fatalf("help = %q, want force restore binding", help)
	}

	m.multiSelectActive = true
	help = m.renderHelpForCurrentState()
	if !strings.Contains(help, "fo") || !strings.Contains(help, "restore") {
		t.Fatalf("multi-select help = %q, want force restore binding", help)
	}
}
