package tui

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// updateApplicationGitFields handles navigation within git sub-fields (gitFieldCursor >= 0)
func (m Model) updateApplicationGitFields(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}

	if m, cmd, handled := m.handleCommonKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, FormNavKeys.Cancel):
		m.activeForm = FormNone
		m.applicationForm = nil
		m.Screen = ScreenResults
		return m, nil

	case key.Matches(msg, FormNavKeys.Up):
		if m.applicationForm.gitFieldCursor > 0 {
			m.applicationForm.gitFieldCursor--
		} else {
			// Back to git label (will route to updateApplicationPackagesList on next keypress)
			m.applicationForm.gitFieldCursor = -1
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Down):
		if m.applicationForm.gitFieldCursor < GitFieldCount-1 {
			m.applicationForm.gitFieldCursor++
		} else {
			// Move to installer item (next in packages list)
			m.applicationForm.packagesCursor = len(displayPackageManagers) + 1
			m.applicationForm.gitFieldCursor = -1
			m.applicationForm.installerFieldCursor = -1
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Edit):
		if m.applicationForm.gitFieldCursor == GitFieldSudo {
			m.applicationForm.gitSudo = !m.applicationForm.gitSudo
			return m, nil
		}
		// Enter edit mode for text fields
		input := m.getGitFieldInput()
		if input != nil {
			m.applicationForm.editingGitField = true
			m.applicationForm.originalValue = input.Value()
			input.Focus()
			input.SetCursor(len(input.Value()))
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Toggle):
		if m.applicationForm.gitFieldCursor == GitFieldSudo {
			m.applicationForm.gitSudo = !m.applicationForm.gitSudo
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.TabNext):
		m.applicationForm.focusIndex++
		if m.applicationForm.focusIndex > 3 {
			m.applicationForm.focusIndex = 0
		}
		m.applicationForm.packagesCursor = 0
		m.applicationForm.gitFieldCursor = -1
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.TabPrev):
		m.applicationForm.focusIndex--
		m.applicationForm.gitFieldCursor = -1
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.Save):
		if err := m.saveApplicationForm(); err != nil {
			m.applicationForm.err = err.Error()
			return m, nil
		}
		m.activeForm = FormNone
		m.applicationForm = nil
		m.Screen = ScreenResults
		return m, m.checkUncheckedPackageStatesCmd()
	}

	return m, nil
}

// updateApplicationGitFieldInput handles text input when editing a git field
func (m Model) updateApplicationGitFieldInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}

	var cmd tea.Cmd

	if m, cmd, handled := m.handleTextEditKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, TextEditKeys.Cancel):
		// Restore original value and exit edit mode
		input := m.getGitFieldInput()
		if input != nil {
			input.SetValue(m.applicationForm.originalValue)
		}
		m.applicationForm.editingGitField = false
		return m, nil

	case key.Matches(msg, TextEditKeys.Confirm) || key.Matches(msg, TextEditKeys.SaveForm):
		// Save current value and exit edit mode
		m.applicationForm.editingGitField = false
		return m, nil
	}

	// Pass to the focused text input
	input := m.getGitFieldInput()
	if input != nil {
		*input, cmd = input.Update(msg)
	}

	m.applicationForm.err = ""
	return m, cmd
}

// getGitFieldInput returns a pointer to the current git text input based on gitFieldCursor
func (m *Model) getGitFieldInput() *textinput.Model {
	if m.applicationForm == nil {
		return nil
	}

	switch m.applicationForm.gitFieldCursor {
	case GitFieldURL:
		return &m.applicationForm.gitURLInput
	case GitFieldBranch:
		return &m.applicationForm.gitBranchInput
	case GitFieldLinux:
		return &m.applicationForm.gitLinuxInput
	case GitFieldWindows:
		return &m.applicationForm.gitWindowsInput
	default:
		return nil
	}
}

// updateApplicationInstallerFields handles navigation within installer sub-fields (installerFieldCursor >= 0)
func (m Model) updateApplicationInstallerFields(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}

	if m, cmd, handled := m.handleCommonKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, FormNavKeys.Cancel):
		m.activeForm = FormNone
		m.applicationForm = nil
		m.Screen = ScreenResults
		return m, nil

	case key.Matches(msg, FormNavKeys.Up):
		if m.applicationForm.installerFieldCursor > 0 {
			m.applicationForm.installerFieldCursor--
		} else {
			// Back to installer label (will route to updateApplicationPackagesList on next keypress)
			m.applicationForm.installerFieldCursor = -1
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Down):
		if m.applicationForm.installerFieldCursor < InstallerFieldCount-1 {
			m.applicationForm.installerFieldCursor++
		} else {
			// Move to When section
			m.applicationForm.focusIndex++
			if m.applicationForm.focusIndex > 3 {
				m.applicationForm.focusIndex = 0
			}
			m.applicationForm.packagesCursor = 0
			m.applicationForm.installerFieldCursor = -1
			m.updateApplicationFormFocus()
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Edit):
		// Enter edit mode for text fields
		input := m.getInstallerFieldInput()
		if input != nil {
			m.applicationForm.editingInstallerField = true
			m.applicationForm.originalValue = input.Value()
			input.Focus()
			input.SetCursor(len(input.Value()))
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.TabNext):
		m.applicationForm.focusIndex++
		if m.applicationForm.focusIndex > 3 {
			m.applicationForm.focusIndex = 0
		}
		m.applicationForm.packagesCursor = 0
		m.applicationForm.installerFieldCursor = -1
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.TabPrev):
		m.applicationForm.focusIndex--
		m.applicationForm.installerFieldCursor = -1
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.Save):
		if err := m.saveApplicationForm(); err != nil {
			m.applicationForm.err = err.Error()
			return m, nil
		}
		m.activeForm = FormNone
		m.applicationForm = nil
		m.Screen = ScreenResults
		return m, m.checkUncheckedPackageStatesCmd()
	}

	return m, nil
}

// updateApplicationInstallerFieldInput handles text input when editing an installer field
func (m Model) updateApplicationInstallerFieldInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}

	var cmd tea.Cmd

	if m, cmd, handled := m.handleTextEditKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, TextEditKeys.Cancel):
		// Restore original value and exit edit mode
		input := m.getInstallerFieldInput()
		if input != nil {
			input.SetValue(m.applicationForm.originalValue)
		}
		m.applicationForm.editingInstallerField = false
		return m, nil

	case key.Matches(msg, TextEditKeys.Confirm) || key.Matches(msg, TextEditKeys.SaveForm):
		// Save current value and exit edit mode
		m.applicationForm.editingInstallerField = false
		return m, nil
	}

	// Pass to the focused text input
	input := m.getInstallerFieldInput()
	if input != nil {
		*input, cmd = input.Update(msg)
	}

	m.applicationForm.err = ""
	return m, cmd
}

// getInstallerFieldInput returns a pointer to the current installer text input based on installerFieldCursor
func (m *Model) getInstallerFieldInput() *textinput.Model {
	if m.applicationForm == nil {
		return nil
	}

	switch m.applicationForm.installerFieldCursor {
	case InstallerFieldLinux:
		return &m.applicationForm.installerLinuxInput
	case InstallerFieldWindows:
		return &m.applicationForm.installerWindowsInput
	case InstallerFieldBinary:
		return &m.applicationForm.installerBinaryInput
	default:
		return nil
	}
}
