package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type entryTypeChooserState struct {
	appName string
	cursor  int
}

type entryTypeOption struct {
	label       string
	description string
	entryType   newSubEntryType
}

var entryTypeOptions = []entryTypeOption{
	{"Folder symlink", "Link an entire backup directory", newSubEntryFolderSymlink},
	{"File symlink", "Link selected files from a backup directory", newSubEntryFileSymlink},
	{"File copy", "Copy selected files instead of linking them", newSubEntryFileCopy},
	{"Setup command", "Run commands when a read-only check fails", newSubEntrySetup},
}

func (m *Model) startEntryTypeChooser(appIdx int) {
	if appIdx < 0 || appIdx >= len(m.Applications) {
		m.closeEntryTypeChooser()
		return
	}
	m.entryTypeChooser = &entryTypeChooserState{appName: m.Applications[appIdx].Application.Name}
	m.Screen = ScreenEntryTypeChooser
}

func (m *Model) closeEntryTypeChooser() {
	m.entryTypeChooser = nil
	m.Screen = ScreenResults
}

func (m Model) updateEntryTypeChooser(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.entryTypeChooser == nil {
		m.Screen = ScreenResults
		return m, nil
	}
	if m, cmd, handled := m.handleCommonKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, ModeChooserKeys.Cancel):
		m.closeEntryTypeChooser()
	case key.Matches(msg, ModeChooserKeys.Up):
		m.entryTypeChooser.cursor = (m.entryTypeChooser.cursor - 1 + len(entryTypeOptions)) % len(entryTypeOptions)
	case key.Matches(msg, ModeChooserKeys.Down):
		m.entryTypeChooser.cursor = (m.entryTypeChooser.cursor + 1) % len(entryTypeOptions)
	case key.Matches(msg, ModeChooserKeys.Select):
		appIdx := -1
		for i := range m.Applications {
			if m.Applications[i].Application.Name == m.entryTypeChooser.appName {
				appIdx = i
				break
			}
		}
		if appIdx < 0 {
			m.closeEntryTypeChooser()
			return m, nil
		}
		if m.findConfigApplicationIndex(m.entryTypeChooser.appName) < 0 {
			m.closeEntryTypeChooser()
			return m, nil
		}
		entryType := entryTypeOptions[m.entryTypeChooser.cursor].entryType
		m.entryTypeChooser = nil
		m.initSubEntryFormWithType(appIdx, -1, entryType)
	}
	return m, nil
}

func (m Model) viewEntryTypeChooser() string {
	if m.entryTypeChooser == nil {
		return ""
	}

	var b strings.Builder
	b.WriteString(TitleStyle.Render("  Choose entry type:"))
	b.WriteString("\n\n")
	for i, option := range entryTypeOptions {
		line := fmt.Sprintf("%s  %s", option.label, MutedTextStyle.Render(option.description))
		if i == m.entryTypeChooser.cursor {
			fmt.Fprintf(&b, "  %s\n", SelectedMenuItemStyle.Render("→ "+line))
		} else {
			fmt.Fprintf(&b, "    %s\n", line)
		}
	}
	b.WriteString("\n")
	b.WriteString(RenderHelpFromBindings(m.width, ModeChooserKeys.Select, ModeChooserKeys.Cancel))
	return BaseStyle.Render(b.String())
}
