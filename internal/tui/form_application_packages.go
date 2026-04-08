package tui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// updateApplicationGitFields handles navigation within git sub-fields (gitFieldCursor >= 0)
func (m Model) updateApplicationGitFields(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		if m.applicationForm.GitFieldCursor > 0 {
			m.applicationForm.GitFieldCursor--
		} else {
			// Back to git label (will route to updateApplicationPackagesList on next keypress)
			m.applicationForm.GitFieldCursor = -1
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Down):
		if m.applicationForm.GitFieldCursor < GitFieldCount-1 {
			m.applicationForm.GitFieldCursor++
		} else {
			// Move to installer item (next in packages list)
			m.applicationForm.PackagesCursor = len(displayPackageManagers) + 1
			m.applicationForm.GitFieldCursor = -1
			m.applicationForm.InstallerFieldCursor = -1
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Edit):
		if m.applicationForm.GitFieldCursor == GitFieldSudo {
			m.applicationForm.GitSudo = !m.applicationForm.GitSudo
			return m, nil
		}
		// Enter edit mode for text fields
		input := m.getGitFieldInput()
		if input != nil {
			m.applicationForm.EditingGitField = true
			m.applicationForm.OriginalValue = input.Value()
			input.Focus()
			input.SetCursor(len(input.Value()))
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Toggle):
		if m.applicationForm.GitFieldCursor == GitFieldSudo {
			m.applicationForm.GitSudo = !m.applicationForm.GitSudo
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.TabNext):
		m.applicationForm.FocusIndex++
		if m.applicationForm.FocusIndex > 3 {
			m.applicationForm.FocusIndex = 0
		}
		m.applicationForm.PackagesCursor = 0
		m.applicationForm.GitFieldCursor = -1
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.TabPrev):
		m.applicationForm.FocusIndex--
		m.applicationForm.GitFieldCursor = -1
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.Save):
		if err := m.saveApplicationForm(); err != nil {
			m.applicationForm.Err = err.Error()
			return m, nil
		}
		m.activeForm = FormNone
		m.applicationForm = nil
		m.Screen = ScreenResults
		return m, m.dispatchUncheckedPackageStates()
	}

	return m, nil
}

// updateApplicationGitFieldInput handles text input when editing a git field
func (m Model) updateApplicationGitFieldInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
			input.SetValue(m.applicationForm.OriginalValue)
		}
		m.applicationForm.EditingGitField = false
		return m, nil

	case key.Matches(msg, TextEditKeys.Confirm) || key.Matches(msg, TextEditKeys.SaveForm):
		// Save current value and exit edit mode
		m.applicationForm.EditingGitField = false
		return m, nil
	}

	// Pass to the focused text input
	input := m.getGitFieldInput()
	if input != nil {
		*input, cmd = input.Update(msg)
	}

	m.applicationForm.Err = ""
	return m, cmd
}

// getGitFieldInput returns a pointer to the current git text input based on gitFieldCursor
func (m *Model) getGitFieldInput() *textinput.Model {
	if m.applicationForm == nil {
		return nil
	}
	return m.applicationForm.GetGitFieldInput()
}

// updateApplicationInstallerFields handles navigation within installer sub-fields (installerFieldCursor >= 0)
func (m Model) updateApplicationInstallerFields(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
		if m.applicationForm.InstallerFieldCursor > 0 {
			m.applicationForm.InstallerFieldCursor--
		} else {
			// Back to installer label (will route to updateApplicationPackagesList on next keypress)
			m.applicationForm.InstallerFieldCursor = -1
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Down):
		if m.applicationForm.InstallerFieldCursor < InstallerFieldCount-1 {
			m.applicationForm.InstallerFieldCursor++
		} else {
			// Move to When section
			m.applicationForm.FocusIndex++
			if m.applicationForm.FocusIndex > 3 {
				m.applicationForm.FocusIndex = 0
			}
			m.applicationForm.PackagesCursor = 0
			m.applicationForm.InstallerFieldCursor = -1
			m.updateApplicationFormFocus()
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Edit):
		// Enter edit mode for text fields
		input := m.getInstallerFieldInput()
		if input != nil {
			m.applicationForm.EditingInstallerField = true
			m.applicationForm.OriginalValue = input.Value()
			input.Focus()
			input.SetCursor(len(input.Value()))
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.TabNext):
		m.applicationForm.FocusIndex++
		if m.applicationForm.FocusIndex > 3 {
			m.applicationForm.FocusIndex = 0
		}
		m.applicationForm.PackagesCursor = 0
		m.applicationForm.InstallerFieldCursor = -1
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.TabPrev):
		m.applicationForm.FocusIndex--
		m.applicationForm.InstallerFieldCursor = -1
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.Save):
		if err := m.saveApplicationForm(); err != nil {
			m.applicationForm.Err = err.Error()
			return m, nil
		}
		m.activeForm = FormNone
		m.applicationForm = nil
		m.Screen = ScreenResults
		return m, m.dispatchUncheckedPackageStates()
	}

	return m, nil
}

// updateApplicationInstallerFieldInput handles text input when editing an installer field
func (m Model) updateApplicationInstallerFieldInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
			input.SetValue(m.applicationForm.OriginalValue)
		}
		m.applicationForm.EditingInstallerField = false
		return m, nil

	case key.Matches(msg, TextEditKeys.Confirm) || key.Matches(msg, TextEditKeys.SaveForm):
		// Save current value and exit edit mode
		m.applicationForm.EditingInstallerField = false
		return m, nil
	}

	// Pass to the focused text input
	input := m.getInstallerFieldInput()
	if input != nil {
		*input, cmd = input.Update(msg)
	}

	m.applicationForm.Err = ""
	return m, cmd
}

// getInstallerFieldInput returns a pointer to the current installer text input based on installerFieldCursor
func (m *Model) getInstallerFieldInput() *textinput.Model {
	if m.applicationForm == nil {
		return nil
	}
	return m.applicationForm.GetInstallerFieldInput()
}
