package tui

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
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

func TestInitSubEntryForm_SetupEntry_OpensEditableForm(t *testing.T) {
	entry := setupSubEntry()
	entry.Sudo = true
	cfg := setupOnlyConfig(entry)
	m := NewModel(cfg, linuxPlatform(), false)

	subIdx := setupSubItemIndex(t, &m, 0)

	m.initSubEntryForm(0, subIdx)

	if m.subEntryForm == nil {
		t.Fatal("the form did not open on a setup entry")
	}
	form := m.subEntryForm
	if !form.IsSetup {
		t.Error("IsSetup = false, want true")
	}
	if got := form.LinuxCheckInput.Value(); got != entry.Check["linux"] {
		t.Errorf("LinuxCheckInput = %q, want %q", got, entry.Check["linux"])
	}
	if got := form.LinuxRunInput.Value(); got != entry.Run["linux"] {
		t.Errorf("LinuxRunInput = %q, want %q", got, entry.Run["linux"])
	}
	if got := form.WindowsCheckInput.Value(); got != entry.Check["windows"] {
		t.Errorf("WindowsCheckInput = %q, want %q", got, entry.Check["windows"])
	}
	if got := form.WindowsRunInput.Value(); got != entry.Run["windows"] {
		t.Errorf("WindowsRunInput = %q, want %q", got, entry.Run["windows"])
	}
	if !form.IsSudo {
		t.Error("IsSudo = false, want true")
	}
}

// TestInitSubEntryForm_ConfigEntry_StillOpens proves the guard is narrow: config
// entries are still editable.
func TestSaveSubEntryForm_SetupEntry_RoundTripsEdits(t *testing.T) {
	entry := setupSubEntry()
	entry.Sudo = true
	cfg := setupOnlyConfig(entry)
	m, path := modelOnDisk(t, cfg)
	subIdx := setupSubItemIndex(t, m, 0)
	m.initSubEntryForm(0, subIdx)
	form := m.subEntryForm
	form.NameInput.SetValue("edited-setup")
	form.LinuxCheckInput.SetValue("test -x /usr/bin/edited")
	form.LinuxRunInput.SetValue("install-edited")
	form.WindowsCheckInput.SetValue("where edited")
	form.WindowsRunInput.SetValue("install-edited.exe")

	if err := m.saveSubEntryForm(); err != nil {
		t.Fatalf("saveSubEntryForm() error = %v", err)
	}

	saved, err := config.Load(path)
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	got := saved.Applications[0].Entries[0]
	if got.Name != "edited-setup" || got.Backup != "" || len(got.Targets) != 0 || len(got.Files) != 0 || got.Method != "" {
		t.Fatalf("saved setup entry has config fields: %+v", got)
	}
	if got.Check["linux"] != "test -x /usr/bin/edited" || got.Run["linux"] != "install-edited" {
		t.Errorf("saved Linux commands = %v/%v", got.Check["linux"], got.Run["linux"])
	}
	if got.Check["windows"] != "where edited" || got.Run["windows"] != "install-edited.exe" {
		t.Errorf("saved Windows commands = %v/%v", got.Check["windows"], got.Run["windows"])
	}
	if !got.Sudo {
		t.Error("saved Sudo = false, want true")
	}
}

func TestInitSubEntryForm_ConfigEntry_StillOpens(t *testing.T) {
	cfg := setupOnlyConfig(configSubEntry())
	m := NewModel(cfg, linuxPlatform(), false)

	m.initSubEntryForm(0, 0)

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
