package tui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/tui/forms"
)

// buildHostnameWhen creates a Go template expression for one or more hosts.
func buildHostnameWhen(hostnames []string) string {
	if len(hostnames) == 0 {
		return ""
	}
	parts := make([]string, 0, len(hostnames))
	for _, hostname := range hostnames {
		parts = append(parts, "(eq .Hostname "+strconv.Quote(hostname)+")")
	}
	expression := strings.TrimPrefix(parts[0], "(")
	expression = strings.TrimSuffix(expression, ")")
	if len(parts) > 1 {
		expression = "or " + strings.Join(parts, " ")
	}
	return "{{ " + expression + " }}"
}

func (m *Model) startWhenChooser() {
	f := m.applicationForm
	f.WhenMode = forms.WhenModeChooser
	f.EditingWhen = false
	f.HostnameCursor = 0
	f.SelectedHostnames = make(map[string]bool)
}

func (m *Model) startWhenTextEdit() {
	f := m.applicationForm
	f.WhenMode = 0
	f.SelectedHostnames = nil
	f.EditingWhen = true
	f.WhenInput.Focus()
	f.WhenInput.SetCursor(len(f.WhenInput.Value()))
}

func (m Model) updateWhenChooser(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	f := m.applicationForm
	if f == nil {
		return m, nil
	}
	optionCount := len(m.HostnameChoices)
	if key.Matches(msg, FormNavKeys.Cancel) {
		f.WhenInput.SetValue(f.OriginalValue)
		f.WhenMode = 0
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
	if key.Matches(msg, FormNavKeys.Edit) || key.Matches(msg, FormNavKeys.Save) {
		if f.HostnameCursor == optionCount {
			m.startWhenTextEdit()
			return m, nil
		}
		selected := make([]string, 0, len(f.SelectedHostnames))
		for _, hostname := range m.HostnameChoices {
			if f.SelectedHostnames[hostname] {
				selected = append(selected, hostname)
			}
		}
		if len(selected) > 0 {
			f.WhenInput.SetValue(buildHostnameWhen(selected))
			f.WhenMode = 0
			f.SelectedHostnames = nil
		}
	}
	return m, nil
}

func (m Model) renderWhenChooser() string {
	f := m.applicationForm
	var b strings.Builder
	for i, hostname := range m.HostnameChoices {
		prefix := "  "
		if f.SelectedHostnames[hostname] {
			prefix = "✓ "
		}
		line := prefix + hostname
		if i == f.HostnameCursor {
			line = SelectedMenuItemStyle.Render(line)
		}
		b.WriteString("  " + line + "\n")
	}
	line := "  Type expression"
	if f.HostnameCursor == len(m.HostnameChoices) {
		line = SelectedMenuItemStyle.Render(line)
	}
	b.WriteString("  " + line + "\n")
	return b.String()
}
