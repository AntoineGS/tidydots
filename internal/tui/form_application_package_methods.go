package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

const (
	customFieldCount = 2
	urlFieldCount    = 4
)

func (m Model) updateApplicationCustomFields(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}
	if m, cmd, handled := m.handleCommonKeys(msg); handled {
		return m, cmd
	}
	switch {
	case key.Matches(msg, FormNavKeys.Cancel):
		m.activeForm, m.applicationForm, m.Screen = FormNone, nil, ScreenResults
	case key.Matches(msg, FormNavKeys.Up):
		if m.applicationForm.CustomFieldCursor > 0 {
			m.applicationForm.CustomFieldCursor--
		} else {
			m.applicationForm.CustomFieldCursor = -1
		}
	case key.Matches(msg, FormNavKeys.Down):
		if m.applicationForm.CustomFieldCursor < customFieldCount-1 {
			m.applicationForm.CustomFieldCursor++
		} else {
			m.applicationForm.PackagesCursor = len(displayPackageManagers) + 3
			m.applicationForm.CustomFieldCursor = -1
			m.applicationForm.URLFieldCursor = -1
		}
	case key.Matches(msg, FormNavKeys.Edit):
		input := m.applicationForm.GetCustomFieldInput()
		if input != nil {
			m.applicationForm.EditingCustomField = true
			m.applicationForm.OriginalValue = input.Value()
			input.Focus()
			input.SetCursor(len(input.Value()))
		}
	case key.Matches(msg, FormNavKeys.Save):
		return m.saveApplicationFormAndExit()
	}
	return m, nil
}

func (m Model) updateApplicationURLFields(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}
	if m, cmd, handled := m.handleCommonKeys(msg); handled {
		return m, cmd
	}
	switch {
	case key.Matches(msg, FormNavKeys.Cancel):
		m.activeForm, m.applicationForm, m.Screen = FormNone, nil, ScreenResults
	case key.Matches(msg, FormNavKeys.Up):
		if m.applicationForm.URLFieldCursor > 0 {
			m.applicationForm.URLFieldCursor--
		} else {
			m.applicationForm.URLFieldCursor = -1
		}
	case key.Matches(msg, FormNavKeys.Down):
		if m.applicationForm.URLFieldCursor < urlFieldCount-1 {
			m.applicationForm.URLFieldCursor++
		} else {
			m.applicationForm.FocusIndex++
			if m.applicationForm.FocusIndex > 3 {
				m.applicationForm.FocusIndex = 0
			}
			m.applicationForm.PackagesCursor = 0
			m.applicationForm.URLFieldCursor = -1
			m.updateApplicationFormFocus()
		}
	case key.Matches(msg, FormNavKeys.Edit):
		input := m.applicationForm.GetURLFieldInput()
		if input != nil {
			m.applicationForm.EditingURLField = true
			m.applicationForm.OriginalValue = input.Value()
			input.Focus()
			input.SetCursor(len(input.Value()))
		}
	case key.Matches(msg, FormNavKeys.Save):
		return m.saveApplicationFormAndExit()
	}
	return m, nil
}

func (m Model) updateApplicationPackageMethodInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}
	var cmd tea.Cmd
	if m, cmd, handled := m.handleTextEditKeys(msg); handled {
		return m, cmd
	}
	input := m.applicationForm.GetCustomFieldInput()
	if m.applicationForm.EditingURLField {
		input = m.applicationForm.GetURLFieldInput()
	}
	switch {
	case key.Matches(msg, TextEditKeys.Cancel):
		if input != nil {
			input.SetValue(m.applicationForm.OriginalValue)
		}
		m.applicationForm.EditingCustomField = false
		m.applicationForm.EditingURLField = false
	case key.Matches(msg, TextEditKeys.Confirm) || key.Matches(msg, TextEditKeys.SaveForm):
		m.applicationForm.EditingCustomField = false
		m.applicationForm.EditingURLField = false
	default:
		if input != nil {
			*input, cmd = input.Update(msg)
		}
		m.applicationForm.Err = ""
	}
	return m, cmd
}

func (m Model) saveApplicationFormAndExit() (tea.Model, tea.Cmd) {
	if err := m.saveApplicationForm(); err != nil {
		m.applicationForm.Err = err.Error()
		return m, nil
	}
	m.activeForm, m.applicationForm, m.Screen = FormNone, nil, ScreenResults
	return m, tea.Batch(m.dispatchUncheckedPackageStates(), m.dispatchLoadingSubEntryStates())
}
