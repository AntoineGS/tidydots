package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/tui/forms"
)

func (m *Model) startSubEntryWhenChooser() {
	f := m.subEntryForm
	f.WhenMode = forms.WhenModeChooser
	f.EditingWhen = false
	f.HostnameCursor = 0
	f.SelectedHostnames = make(map[string]bool)
}

func (m *Model) startSubEntryWhenTextEdit() {
	f := m.subEntryForm
	f.WhenMode = forms.WhenModeNone
	f.SelectedHostnames = nil
	f.EditingWhen = true
	f.WhenInput.Focus()
	f.WhenInput.SetCursor(len(f.WhenInput.Value()))
}

func (m Model) updateSubEntryWhenChooser(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := m.subEntryForm
	optionCount := len(m.HostnameChoices)
	if key.Matches(msg, FormNavKeys.Cancel) {
		f.WhenInput.SetValue(f.OriginalValue)
		f.WhenMode = forms.WhenModeNone
		f.SelectedHostnames = nil
		return m, nil
	}
	if key.Matches(msg, FormNavKeys.Down) {
		f.HostnameCursor = (f.HostnameCursor + 1) % (optionCount + 1)
		return m, nil
	}
	if key.Matches(msg, FormNavKeys.Up) || key.Matches(msg, FormNavKeys.TabPrev) {
		f.HostnameCursor = (f.HostnameCursor + optionCount) % (optionCount + 1)
		return m, nil
	}
	if (key.Matches(msg, FormNavKeys.Toggle) || key.Matches(msg, FormNavKeys.TabNext)) && f.HostnameCursor < optionCount {
		host := m.HostnameChoices[f.HostnameCursor]
		f.SelectedHostnames[host] = !f.SelectedHostnames[host]
		return m, nil
	}
	saveForm := key.Matches(msg, FormNavKeys.Save)
	if key.Matches(msg, FormNavKeys.Edit) || saveForm {
		if f.HostnameCursor == optionCount {
			m.startSubEntryWhenTextEdit()
			return m, nil
		}
		selected := make([]string, 0, len(f.SelectedHostnames))
		for _, hostname := range m.HostnameChoices {
			if f.SelectedHostnames[hostname] {
				selected = append(selected, hostname)
			}
		}
		f.WhenInput.SetValue(buildHostnameWhen(selected))
		f.WhenMode = forms.WhenModeNone
		f.SelectedHostnames = nil
		if saveForm {
			return m.updateSubEntryForm(msg)
		}
	}
	return m, nil
}

func (m Model) updateSubEntryWhenInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := m.subEntryForm
	if m, cmd, handled := m.handleTextEditKeys(msg); handled {
		return m, cmd
	}
	switch {
	case key.Matches(msg, TextEditKeys.Cancel):
		f.WhenInput.SetValue(f.OriginalValue)
		f.WhenInput.Blur()
		f.EditingWhen = false
		f.Err = ""
	case key.Matches(msg, TextEditKeys.Confirm) || key.Matches(msg, TextEditKeys.SaveForm):
		f.WhenInput.Blur()
		f.EditingWhen = false
	default:
		var cmd tea.Cmd
		f.WhenInput, cmd = f.WhenInput.Update(msg)
		f.Err = ""
		return m, cmd
	}
	return m, nil
}

func (m Model) renderSubEntryWhenChooser() string {
	if m.subEntryForm == nil {
		return ""
	}
	return renderHostnameOptions(m.HostnameChoices, m.subEntryForm.HostnameCursor, m.subEntryForm.SelectedHostnames)
}

func renderHostnameOptions(choices []string, cursor int, selected map[string]bool) string {
	var b strings.Builder
	for i, hostname := range choices {
		prefix := "  "
		if selected[hostname] {
			prefix = "✓ "
		}
		line := prefix + hostname
		if i == cursor {
			line = SelectedMenuItemStyle.Render(line)
		}
		fmt.Fprintf(&b, "  %s\n", line)
	}
	line := "  Type expression"
	if cursor == len(choices) {
		line = SelectedMenuItemStyle.Render(line)
	}
	fmt.Fprintf(&b, "  %s\n", line)
	return b.String()
}
