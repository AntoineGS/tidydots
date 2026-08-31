package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

func newEntryTypeChooserTestModel() Model {
	cfg := &config.Config{Applications: []config.Application{{
		Name: "tool",
		Entries: []config.SubEntry{{
			Name: "existing", Backup: "./existing",
			Targets: map[string]string{"linux": "~/.config/existing"},
		}},
	}}}
	m := NewModel(cfg, &platform.Platform{OS: OSLinux}, false)
	m.Operation = OpList
	m.Screen = ScreenResults
	// The chooser tests exercise navigation, not the asynchronous state gate.
	m.pendingStateChecks = 0
	return m
}

func TestAddEntryOpensTypeChooser(t *testing.T) {
	m := newEntryTypeChooserTestModel()
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	got := updated.(Model)
	if got.Screen != ScreenEntryTypeChooser || got.entryTypeChooser == nil {
		t.Fatalf("a did not open entry type chooser")
	}
	if got.entryTypeChooser.appName != "tool" || got.entryTypeChooser.cursor != 0 {
		t.Fatalf("chooser state = %#v", got.entryTypeChooser)
	}
}

func TestEntryTypeChooserNavigationWraps(t *testing.T) {
	m := newEntryTypeChooserTestModel()
	m.startEntryTypeChooser(0)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = updated.(Model)
	if m.entryTypeChooser.cursor != len(entryTypeOptions)-1 {
		t.Fatalf("up from first = %d", m.entryTypeChooser.cursor)
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = updated.(Model)
	if m.entryTypeChooser.cursor != 0 {
		t.Fatalf("j did not wrap to first option")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	if m.entryTypeChooser.cursor != 1 {
		t.Fatalf("down did not move to second option")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	m = updated.(Model)
	if m.entryTypeChooser.cursor != 0 {
		t.Fatalf("k did not move to first option")
	}
}

func TestEntryTypeChooserCancelReturnsToManage(t *testing.T) {
	m := newEntryTypeChooserTestModel()
	m.startEntryTypeChooser(0)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := updated.(Model)
	if got.Screen != ScreenResults || got.entryTypeChooser != nil || got.subEntryForm != nil {
		t.Fatalf("cancel left chooser or form state behind")
	}
	if len(got.Config.Applications[0].Entries) != 1 {
		t.Fatal("cancel modified configuration entries")
	}
}

func TestEntryTypeChooserSelectsAllPresets(t *testing.T) {
	expected := []struct {
		label     string
		entryType newSubEntryType
	}{
		{"Folder symlink", newSubEntryFolderSymlink},
		{"File symlink", newSubEntryFileSymlink},
		{"File copy", newSubEntryFileCopy},
		{"Setup command", newSubEntrySetup},
	}

	for cursor, want := range expected {
		t.Run(want.label, func(t *testing.T) {
			m := newEntryTypeChooserTestModel()
			m.startEntryTypeChooser(0)
			m.entryTypeChooser.cursor = cursor
			updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			got := updated.(Model)
			if got.Screen != ScreenAddForm || got.activeForm != FormSubEntry || got.subEntryForm == nil {
				t.Fatalf("selection did not open sub-entry form")
			}
			if got.entryTypeChooser != nil {
				t.Fatal("selection left chooser state behind")
			}
			if got.subEntryForm.IsSetup != (want.entryType == newSubEntrySetup) ||
				got.subEntryForm.IsFolder != (want.entryType == newSubEntryFolderSymlink) ||
				got.subEntryForm.IsCopy != (want.entryType == newSubEntryFileCopy) {
				t.Fatalf("wrong form preset for %q", want.label)
			}
		})
	}
}

func TestEntryTypeChooserOptionsMatchContract(t *testing.T) {
	expected := []struct {
		label     string
		entryType newSubEntryType
	}{
		{"Folder symlink", newSubEntryFolderSymlink},
		{"File symlink", newSubEntryFileSymlink},
		{"File copy", newSubEntryFileCopy},
		{"Setup command", newSubEntrySetup},
	}

	if len(entryTypeOptions) != len(expected) {
		t.Fatalf("option count = %d, want %d", len(entryTypeOptions), len(expected))
	}
	for i, want := range expected {
		if entryTypeOptions[i].label != want.label {
			t.Errorf("option %d label = %q, want %q", i, entryTypeOptions[i].label, want.label)
		}
		if entryTypeOptions[i].entryType != want.entryType {
			t.Errorf("option %d type = %d, want %d", i, entryTypeOptions[i].entryType, want.entryType)
		}
	}
}

func TestEntryTypeChooserInvalidStartClearsState(t *testing.T) {
	m := newEntryTypeChooserTestModel()
	m.startEntryTypeChooser(0)
	if m.entryTypeChooser == nil || m.Screen != ScreenEntryTypeChooser {
		t.Fatal("valid chooser did not start")
	}

	m.startEntryTypeChooser(-1)

	if m.Screen != ScreenResults || m.entryTypeChooser != nil || m.subEntryForm != nil {
		t.Fatalf("invalid chooser start left state behind: screen=%v chooser=%#v form=%#v", m.Screen, m.entryTypeChooser, m.subEntryForm)
	}
}

func TestEntryTypeChooserViewListsEveryOption(t *testing.T) {
	m := newEntryTypeChooserTestModel()
	m.startEntryTypeChooser(0)
	view := m.viewEntryTypeChooser()
	for _, option := range entryTypeOptions {
		if !strings.Contains(view, option.label) || !strings.Contains(view, option.description) {
			t.Fatalf("view does not contain option %q and its description", option.label)
		}
	}
}

func TestEntryTypeChooserInvalidTargetReturnsToManage(t *testing.T) {
	m := newEntryTypeChooserTestModel()
	m.startEntryTypeChooser(0)
	m.Applications = nil

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)
	if got.Screen != ScreenResults || got.entryTypeChooser != nil || got.subEntryForm != nil {
		t.Fatalf("invalid target did not return to clean Manage state")
	}
}

func TestEntryTypeChooserMissingConfigApplicationReturnsToManage(t *testing.T) {
	m := newEntryTypeChooserTestModel()
	m.startEntryTypeChooser(0)
	m.Config.Applications = nil

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := updated.(Model)
	if got.Screen != ScreenResults || got.entryTypeChooser != nil || got.subEntryForm != nil {
		t.Fatalf("missing config application did not return to clean Manage state")
	}
}

func TestEditExistingEntryBypassesTypeChooser(t *testing.T) {
	m := newEntryTypeChooserTestModel()
	m.Applications[0].Expanded = true
	m.rebuildTable()
	m.tableCursor = 1

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	got := updated.(Model)
	if got.Screen != ScreenAddForm || got.activeForm != FormSubEntry || got.subEntryForm == nil {
		t.Fatalf("edit did not open the existing sub-entry form")
	}
	if got.entryTypeChooser != nil {
		t.Fatal("edit unexpectedly opened the entry type chooser")
	}
}
