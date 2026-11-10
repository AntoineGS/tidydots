package tui

import (
	"bytes"
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

func TestSubEntryRepositoryDraft_TogglesForConfigAndSetup(t *testing.T) {
	tests := []struct {
		name  string
		entry config.SubEntry
	}{
		{name: "config", entry: configSubEntry()},
		{name: "setup", entry: setupSubEntry()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(setupOnlyConfig(tt.entry), linuxPlatform(), false)
			m.initSubEntryForm(0, 0)
			form := m.subEntryForm
			if form == nil {
				t.Fatal("form did not open")
			}
			form.PrivateRepositoryEnabled = true
			form.FocusIndex = form.MaxIndex()
			m.updateSubEntryFormFocus()
			if got := m.getSubEntryFieldType(); got != subFieldRepository {
				t.Fatalf("focus maps to %v, want repository field", got)
			}
			if form.RepositoryRole != workspace.Public {
				t.Fatalf("new form RepositoryRole = %q, want Public", form.RepositoryRole)
			}

			updateSubEntryForm(t, &m, tea.KeyPressMsg{Code: tea.KeySpace})
			if form.RepositoryRole != workspace.Private {
				t.Fatalf("space selected %q, want Private", form.RepositoryRole)
			}
			updateSubEntryForm(t, &m, tea.KeyPressMsg{Code: tea.KeyLeft})
			if form.RepositoryRole != workspace.Public {
				t.Fatalf("left selected %q, want Public", form.RepositoryRole)
			}
			updateSubEntryForm(t, &m, tea.KeyPressMsg{Code: tea.KeyRight})
			if form.RepositoryRole != workspace.Private {
				t.Fatalf("right selected %q, want Private", form.RepositoryRole)
			}
			updateSubEntryForm(t, &m, tea.KeyPressMsg{Code: tea.KeyEnter})
			if form.RepositoryRole != workspace.Public {
				t.Fatalf("enter toggled to %q, want Public", form.RepositoryRole)
			}
			updateSubEntryForm(t, &m, tea.KeyPressMsg{Code: tea.KeyRight})
		})
	}
}

func TestSubEntryRepositoryDraft_PrivateUnavailableAndCancel(t *testing.T) {
	tests := []struct {
		name  string
		entry config.SubEntry
	}{
		{name: "config", entry: configSubEntry()},
		{name: "setup", entry: setupSubEntry()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, path := modelOnDisk(t, setupOnlyConfig(tt.entry))
			beforeYAML, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read initial config: %v", err)
			}
			before := m.Config.Applications[0].Entries[0]
			subIdx := 0
			m.initSubEntryForm(0, subIdx)
			form := m.subEntryForm
			form.FocusIndex = form.MaxIndex()
			m.updateSubEntryFormFocus()

			updateSubEntryForm(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
			updateSubEntryForm(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
			if form.RepositoryRole != workspace.Public {
				t.Fatalf("unavailable private repository selected role %q", form.RepositoryRole)
			}
			if got := m.viewSubEntryForm(); !containsText(got, "Private unavailable") {
				t.Fatalf("view does not explain unavailable Private repository: %q", got)
			}

			form.PrivateRepositoryEnabled = true
			updateSubEntryForm(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
			if form.RepositoryRole != workspace.Private {
				t.Fatalf("right selected %q, want Private before cancel", form.RepositoryRole)
			}
			updateSubEntryForm(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
			if m.subEntryForm != nil {
				t.Fatal("esc left the sub-entry form open")
			}
			if got := m.Config.Applications[0].Entries[0]; got.Name != before.Name || got.Check["linux"] != before.Check["linux"] {
				t.Fatalf("cancel changed the in-memory configuration: got %+v, want %+v", got, before)
			}
			afterYAML, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read config after cancel: %v", err)
			}
			if !bytes.Equal(beforeYAML, afterYAML) {
				t.Fatal("cancel persisted the pending repository ownership selection")
			}
		})
	}
}

func TestSubEntryRepositoryDraft_RejectsPrivateWithoutRegisteredWorkspace(t *testing.T) {
	m, path := modelOnDisk(t, setupOnlyConfig(configSubEntry()))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read initial config: %v", err)
	}
	m.initSubEntryForm(0, 0)
	m.subEntryForm.PrivateRepositoryEnabled = true
	m.subEntryForm.FocusIndex = m.subEntryForm.MaxIndex()
	m.updateSubEntryFormFocus()
	updateSubEntryForm(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	if err := m.saveSubEntryForm(); err == nil || !containsText(err.Error(), "without a registered private repository") {
		t.Fatalf("save with a changed owner error = %v, want missing-private-repository error", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config after rejected move: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("form persisted configuration despite lacking a workspace move API")
	}
}

func updateSubEntryForm(t *testing.T, m *Model, msg tea.KeyPressMsg) {
	t.Helper()
	updated, _ := m.updateSubEntryForm(msg)
	updatedModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("updateSubEntryForm returned %T, want Model", updated)
	}
	*m = updatedModel
}
