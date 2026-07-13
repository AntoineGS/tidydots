package tui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/tui/forms"
)

// Field type aliases from forms package for use in root tui methods.
type subEntryFieldType = forms.SubEntryFieldType

const (
	subFieldName     = forms.SubFieldName
	subFieldLinux    = forms.SubFieldLinux
	subFieldWindows  = forms.SubFieldWindows
	subFieldBackup   = forms.SubFieldBackup
	subFieldIsFolder = forms.SubFieldIsFolder
	subFieldFiles    = forms.SubFieldFiles
	subFieldIsSudo   = forms.SubFieldIsSudo
)

// Mode constants from forms package.
const (
	ModeNone      = forms.ModeNone
	ModeChoosing  = forms.ModeChoosing
	ModePicker    = forms.ModePicker
	ModeTextInput = forms.ModeTextInput
)

// initSubEntryForm initializes the sub-entry form.
// appIdx is the index in m.Applications (sorted).
// If subIdx >= 0, loads data from the existing sub-entry (edit mode).
// If subIdx < 0, creates an empty form for adding to the app (new mode).
func (m *Model) initSubEntryForm(appIdx, subIdx int) {
	// appIdx is an index into m.Applications (sorted), not m.Config.Applications (unsorted)
	// We need to find the correct index in m.Config.Applications by application name
	if appIdx < 0 || appIdx >= len(m.Applications) {
		return
	}

	appName := m.Applications[appIdx].Application.Name
	configAppIdx := m.findConfigApplicationIndex(appName)
	if configAppIdx < 0 {
		return
	}

	// Resolve sub-entry data (edit mode only)
	configSubIdx := -1
	hasSub := false
	var sub config.SubEntry

	if subIdx >= 0 {
		app := m.Config.Applications[configAppIdx]

		// subIdx is an index into m.Applications[appIdx].SubItems, which may be filtered
		// We need to find the correct index in app.Entries by sub-entry name
		if subIdx >= len(m.Applications[appIdx].SubItems) {
			return
		}

		subEntryName := m.Applications[appIdx].SubItems[subIdx].SubEntry.Name
		for i, entry := range app.Entries {
			if entry.Name == subEntryName {
				configSubIdx = i
				break
			}
		}

		if configSubIdx < 0 {
			return
		}

		sub = app.Entries[configSubIdx]
		hasSub = true
	}

	nameInput := newFormInput("e.g., nvim-config", CharLimitName, InputWidthNarrow)
	nameInput.Focus()

	linuxTargetInput := newFormInput("e.g., ~/.config/nvim", CharLimitPath, InputWidthNarrow)
	windowsTargetInput := newFormInput("e.g., ~/AppData/Local/nvim", CharLimitPath, InputWidthNarrow)
	backupInput := newFormInput("e.g., ./nvim", CharLimitPath, InputWidthNarrow)
	newFileInput := newFormInput("e.g., .bashrc", CharLimitFile, InputWidthNarrow)

	isSudo := false
	isFolder := true
	var files []string

	if hasSub {
		nameInput.SetValue(sub.Name)

		if target, ok := sub.Targets["linux"]; ok {
			linuxTargetInput.SetValue(target)
		}
		if target, ok := sub.Targets["windows"]; ok {
			windowsTargetInput.SetValue(target)
		}

		backupInput.SetValue(sub.Backup)
		isSudo = sub.Sudo
		isFolder = sub.IsFolder()

		if !isFolder && len(sub.Files) > 0 {
			files = make([]string, len(sub.Files))
			copy(files, sub.Files)
		}
	}

	// New mode: targetAppIdx = configAppIdx, editAppIdx = -1, editSubIdx = -1
	// Edit mode: targetAppIdx = -1, editAppIdx = configAppIdx, editSubIdx = configSubIdx
	targetAppIdx := configAppIdx
	editAppIdx := -1
	editSubEntryIdx := -1

	if hasSub {
		targetAppIdx = -1
		editAppIdx = configAppIdx
		editSubEntryIdx = configSubIdx
	}

	m.subEntryForm = &SubEntryForm{
		NameInput:          nameInput,
		LinuxTargetInput:   linuxTargetInput,
		WindowsTargetInput: windowsTargetInput,
		IsSudo:             isSudo,
		BackupInput:        backupInput,
		IsFolder:           isFolder,
		Files:              files,
		FilesCursor:        0,
		NewFileInput:       newFileInput,
		AddingFile:         false,
		EditingFile:        false,
		EditingFileIndex:   -1,
		FocusIndex:         0,
		EditingField:       false,
		OriginalValue:      "",
		Suggestions:        nil,
		SuggestionCursor:   -1,
		ShowSuggestions:    false,
		TargetAppIdx:       targetAppIdx,
		EditAppIdx:         editAppIdx,
		EditSubIdx:         editSubEntryIdx,
		Err:                "",
		AddFileMode:        ModeNone,
		ModeMenuCursor:     0,
		SelectedFiles:      make(map[string]bool),
	}

	m.activeForm = FormSubEntry
	m.Screen = ScreenAddForm
}

// getSubEntryFieldType returns the field type at the current focus index
func (m *Model) getSubEntryFieldType() subEntryFieldType {
	if m.subEntryForm == nil {
		return subFieldName
	}
	return m.subEntryForm.GetFieldType()
}

// subEntryFormMaxIndex returns the maximum focus index based on state
func (m *Model) subEntryFormMaxIndex() int {
	if m.subEntryForm == nil {
		return 0
	}
	return m.subEntryForm.MaxIndex()
}

// updateSubEntryForm handles key events for the sub-entry form
func (m Model) updateSubEntryForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.subEntryForm == nil {
		return m, nil
	}

	// Handle mode selection menu
	if m.subEntryForm.AddFileMode == ModeChoosing {
		return m.updateFileAddModeChoice(msg)
	}

	// Handle file picker
	if m.subEntryForm.AddFileMode == ModePicker {
		return m.updateSubEntryFilePicker(msg)
	}

	// Handle manual text input mode (from "Type Path" menu option)
	if m.subEntryForm.AddFileMode == ModeTextInput {
		return m.updateSubEntryFileInput(msg)
	}

	// Handle editing a text field
	if m.subEntryForm.EditingField {
		return m.updateSubEntryFieldInput(msg)
	}

	// Handle adding/editing file mode
	if m.subEntryForm.AddingFile || m.subEntryForm.EditingFile {
		return m.updateSubEntryFileInput(msg)
	}

	// Handle files list navigation
	if m.getSubEntryFieldType() == subFieldFiles {
		return m.updateSubEntryFilesList(msg)
	}

	if m, cmd, handled := m.handleCommonKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, FormNavKeys.Cancel):
		// Return to list view
		m.activeForm = FormNone
		m.subEntryForm = nil
		m.Screen = ScreenResults

		return m, nil

	case key.Matches(msg, FormNavKeys.Down):
		m.subEntryForm.FocusIndex++

		maxIndex := m.subEntryFormMaxIndex()
		if m.subEntryForm.FocusIndex > maxIndex {
			m.subEntryForm.FocusIndex = 0
		}

		m.updateSubEntryFormFocus()
		m.subEntryForm.Err = ""            // Clear error on navigation
		m.subEntryForm.SuccessMessage = "" // Clear success message on navigation

		return m, nil

	case key.Matches(msg, FormNavKeys.Up):
		m.subEntryForm.FocusIndex--
		if m.subEntryForm.FocusIndex < 0 {
			m.subEntryForm.FocusIndex = m.subEntryFormMaxIndex()
		}
		if m.getSubEntryFieldType() == subFieldFiles {
			m.subEntryForm.FilesCursor = len(m.subEntryForm.Files)
		}

		m.updateSubEntryFormFocus()
		m.subEntryForm.Err = ""            // Clear error on navigation
		m.subEntryForm.SuccessMessage = "" // Clear success message on navigation

		return m, nil

	case key.Matches(msg, FormNavKeys.TabNext):
		m.subEntryForm.FocusIndex++

		maxIndex := m.subEntryFormMaxIndex()
		if m.subEntryForm.FocusIndex > maxIndex {
			m.subEntryForm.FocusIndex = 0
		}

		m.updateSubEntryFormFocus()
		m.subEntryForm.Err = ""            // Clear error on navigation
		m.subEntryForm.SuccessMessage = "" // Clear success message on navigation

		return m, nil

	case key.Matches(msg, FormNavKeys.TabPrev):
		m.subEntryForm.FocusIndex--
		if m.subEntryForm.FocusIndex < 0 {
			m.subEntryForm.FocusIndex = m.subEntryFormMaxIndex()
		}
		if m.getSubEntryFieldType() == subFieldFiles {
			m.subEntryForm.FilesCursor = len(m.subEntryForm.Files)
		}

		m.updateSubEntryFormFocus()
		m.subEntryForm.Err = ""            // Clear error on navigation
		m.subEntryForm.SuccessMessage = "" // Clear success message on navigation

		return m, nil

	case key.Matches(msg, FormNavKeys.Toggle):
		// Handle toggles
		ft := m.getSubEntryFieldType()
		switch ft {
		case subFieldIsFolder:
			m.subEntryForm.IsFolder = !m.subEntryForm.IsFolder
			return m, nil
		case subFieldIsSudo:
			m.subEntryForm.IsSudo = !m.subEntryForm.IsSudo
			return m, nil
		case subFieldName, subFieldLinux, subFieldWindows, subFieldBackup, subFieldFiles:
			// Text and list fields don't toggle
		}

	case key.Matches(msg, FormNavKeys.Edit):
		// Enter edit mode for text fields
		ft := m.getSubEntryFieldType()

		if m.isSubEntryTextInputField() {
			m.enterSubEntryFieldEditMode()
			return m, nil
		}
		// Handle toggles on enter
		switch ft {
		case subFieldIsFolder:
			m.subEntryForm.IsFolder = !m.subEntryForm.IsFolder
			return m, nil
		case subFieldIsSudo:
			m.subEntryForm.IsSudo = !m.subEntryForm.IsSudo
			return m, nil
		case subFieldName, subFieldLinux, subFieldWindows, subFieldBackup, subFieldFiles:
			// Text and list fields don't toggle
		}

	case key.Matches(msg, FormNavKeys.Save):
		// Save the form
		if err := m.saveSubEntryForm(); err != nil {
			m.subEntryForm.Err = err.Error()
			return m, nil
		}
		// Success - go back to list
		m.activeForm = FormNone
		m.subEntryForm = nil
		m.Screen = ScreenResults

		return m, m.dispatchLoadingSubEntryStates()
	}

	// Clear error when navigating
	m.subEntryForm.Err = ""

	return m, nil
}

// updateSubEntryFilesList handles key events when the files list is focused
func (m Model) updateSubEntryFilesList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.subEntryForm == nil {
		return m, nil
	}

	// filesCursor: 0 to len(files)-1 for file items, len(files) for "Add File" button
	maxCursor := len(m.subEntryForm.Files)

	if m, cmd, handled := m.handleCommonKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, FormNavKeys.Cancel):
		m.activeForm = FormNone
		m.subEntryForm = nil
		m.Screen = ScreenResults

		return m, nil

	case key.Matches(msg, FilesListKeys.Up):
		if m.subEntryForm.FilesCursor > 0 {
			m.subEntryForm.FilesCursor--
		} else {
			// Move to previous field
			m.subEntryForm.FocusIndex--
			m.updateSubEntryFormFocus()
		}

		return m, nil

	case key.Matches(msg, FilesListKeys.Down):
		if m.subEntryForm.FilesCursor < maxCursor {
			m.subEntryForm.FilesCursor++
		} else {
			// Move to next field
			m.subEntryForm.FocusIndex++

			maxIndex := m.subEntryFormMaxIndex()
			if m.subEntryForm.FocusIndex > maxIndex {
				m.subEntryForm.FocusIndex = 0
			}
			m.subEntryForm.FilesCursor = 0
			m.updateSubEntryFormFocus()
		}

		return m, nil

	case key.Matches(msg, FormNavKeys.TabNext):
		// Move to next field
		m.subEntryForm.FocusIndex++

		maxIndex := m.subEntryFormMaxIndex()
		if m.subEntryForm.FocusIndex > maxIndex {
			m.subEntryForm.FocusIndex = 0
		}
		m.subEntryForm.FilesCursor = 0
		m.updateSubEntryFormFocus()

		return m, nil

	case key.Matches(msg, FormNavKeys.TabPrev):
		// Move to previous field
		m.subEntryForm.FocusIndex--
		m.subEntryForm.FilesCursor = 0
		m.updateSubEntryFormFocus()

		return m, nil

	case key.Matches(msg, FilesListKeys.Edit):
		// If on "Add File" button, start mode selection
		if m.subEntryForm.FilesCursor == len(m.subEntryForm.Files) {
			m.subEntryForm.AddFileMode = ModeChoosing
			m.subEntryForm.ModeMenuCursor = 0

			return m, nil
		}
		// Edit the selected file
		if m.subEntryForm.FilesCursor < len(m.subEntryForm.Files) {
			m.subEntryForm.EditingFile = true
			m.subEntryForm.EditingFileIndex = m.subEntryForm.FilesCursor
			m.subEntryForm.NewFileInput.SetValue(m.subEntryForm.Files[m.subEntryForm.FilesCursor])
			m.subEntryForm.NewFileInput.Focus()
			m.subEntryForm.NewFileInput.SetCursor(len(m.subEntryForm.Files[m.subEntryForm.FilesCursor]))
		}

		return m, nil

	case key.Matches(msg, FilesListKeys.Delete):
		// Delete the selected file
		if m.subEntryForm.FilesCursor < len(m.subEntryForm.Files) && len(m.subEntryForm.Files) > 0 {
			// Remove file at cursor
			m.subEntryForm.Files = append(
				m.subEntryForm.Files[:m.subEntryForm.FilesCursor],
				m.subEntryForm.Files[m.subEntryForm.FilesCursor+1:]...,
			)
			// Adjust cursor if needed
			if m.subEntryForm.FilesCursor >= len(m.subEntryForm.Files) && m.subEntryForm.FilesCursor > 0 {
				m.subEntryForm.FilesCursor--
			}
		}

		return m, nil

	case key.Matches(msg, FilesListKeys.Save):
		// Save the form
		if err := m.saveSubEntryForm(); err != nil {
			m.subEntryForm.Err = err.Error()
			return m, nil
		}
		m.activeForm = FormNone
		m.subEntryForm = nil
		m.Screen = ScreenResults

		return m, m.dispatchLoadingSubEntryStates()
	}

	return m, nil
}

// updateSubEntryFileInput handles key events when adding or editing a file
func (m Model) updateSubEntryFileInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.subEntryForm == nil {
		return m, nil
	}

	var cmd tea.Cmd

	if m, cmd, handled := m.handleTextEditKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, TextEditKeys.Cancel):
		// Cancel adding/editing file
		m.subEntryForm.AddingFile = false
		m.subEntryForm.EditingFile = false
		m.subEntryForm.EditingFileIndex = -1
		m.subEntryForm.AddFileMode = ModeNone
		m.subEntryForm.NewFileInput.SetValue("")

		return m, nil

	case key.Matches(msg, SearchKeys.Confirm) || key.Matches(msg, TextEditKeys.SaveForm):
		fileName := strings.TrimSpace(m.subEntryForm.NewFileInput.Value())
		if m.subEntryForm.EditingFile {
			// Update existing file if not empty
			if fileName != "" && m.subEntryForm.EditingFileIndex >= 0 && m.subEntryForm.EditingFileIndex < len(m.subEntryForm.Files) {
				m.subEntryForm.Files[m.subEntryForm.EditingFileIndex] = fileName
			}
			m.subEntryForm.EditingFile = false
			m.subEntryForm.EditingFileIndex = -1
		} else {
			// Add new file if not empty
			if fileName != "" {
				m.subEntryForm.Files = append(m.subEntryForm.Files, fileName)
				m.subEntryForm.FilesCursor = len(m.subEntryForm.Files) // Move cursor to "Add File" button
			}
			m.subEntryForm.AddingFile = false
		}

		m.subEntryForm.AddFileMode = ModeNone
		m.subEntryForm.NewFileInput.SetValue("")

		return m, nil
	}

	// Handle text input
	m.subEntryForm.NewFileInput, cmd = m.subEntryForm.NewFileInput.Update(msg)

	return m, cmd
}

// viewSubEntryForm renders the sub-entry form
//
//nolint:gocyclo // UI rendering with many states
func (m Model) viewSubEntryForm() string {
	if m.subEntryForm == nil {
		return ""
	}

	// Show mode selection menu if in ModeChoosing
	if m.subEntryForm.AddFileMode == ModeChoosing {
		return m.viewFileAddModeMenu()
	}

	// Show file picker if in ModePicker
	if m.subEntryForm.AddFileMode == ModePicker {
		return m.viewFilePicker()
	}

	var b strings.Builder
	ft := m.getSubEntryFieldType()

	// Name field
	nameLabel := "Name:"
	if ft == subFieldName {
		nameLabel = HelpKeyStyle.Render("Name:")
	}

	fmt.Fprintf(&b, "  %s\n", nameLabel)
	fmt.Fprintf(&b, "  %s\n\n", m.renderSubEntryFieldValue(subFieldName, "(empty)"))

	// Linux target field
	linuxTargetLabel := "Target (linux):"
	if ft == subFieldLinux {
		linuxTargetLabel = HelpKeyStyle.Render(linuxTargetLabel)
	}

	fmt.Fprintf(&b, "  %s\n", linuxTargetLabel)
	fmt.Fprintf(&b, "  %s\n", m.renderSubEntryFieldValue(subFieldLinux, "(empty)"))

	if m.subEntryForm.EditingField && ft == subFieldLinux && m.subEntryForm.ShowSuggestions {
		b.WriteString(m.renderSubEntrySuggestions())
	}

	b.WriteString("\n")

	// Windows target field
	windowsTargetLabel := "Target (windows):"
	if ft == subFieldWindows {
		windowsTargetLabel = HelpKeyStyle.Render(windowsTargetLabel)
	}

	fmt.Fprintf(&b, "  %s\n", windowsTargetLabel)
	fmt.Fprintf(&b, "  %s\n", m.renderSubEntryFieldValue(subFieldWindows, "(empty)"))

	if m.subEntryForm.EditingField && ft == subFieldWindows && m.subEntryForm.ShowSuggestions {
		b.WriteString(m.renderSubEntrySuggestions())
	}

	b.WriteString("\n")

	// Backup field
	backupLabel := "Backup path:"
	if ft == subFieldBackup {
		backupLabel = HelpKeyStyle.Render("Backup path:")
	}

	fmt.Fprintf(&b, "  %s\n", backupLabel)
	fmt.Fprintf(&b, "  %s\n", m.renderSubEntryFieldValue(subFieldBackup, "(empty)"))

	if m.subEntryForm.EditingField && ft == subFieldBackup && m.subEntryForm.ShowSuggestions {
		b.WriteString(m.renderSubEntrySuggestions())
	}

	b.WriteString("\n")

	// Is folder toggle
	toggleLabel := "Backup type:"
	if ft == subFieldIsFolder {
		toggleLabel = HelpKeyStyle.Render("Backup type:")
	}
	folderCheck := CheckboxUnchecked
	filesCheck := CheckboxChecked

	if m.subEntryForm.IsFolder {
		folderCheck = CheckboxChecked
		filesCheck = CheckboxUnchecked
	}

	fmt.Fprintf(&b, "  %s  %s Folder  %s Files\n\n", toggleLabel, folderCheck, filesCheck)

	// Files list (only shown when Files mode is selected)
	if !m.subEntryForm.IsFolder {
		filesLabel := "Files:"
		if ft == subFieldFiles {
			filesLabel = HelpKeyStyle.Render("Files:")
		}

		fmt.Fprintf(&b, "  %s\n", filesLabel)

		// Render file list
		if len(m.subEntryForm.Files) == 0 && !m.subEntryForm.AddingFile {
			b.WriteString(MutedTextStyle.Render("    (no files added)"))
			b.WriteString("\n")
		} else {
			for i, file := range m.subEntryForm.Files {
				prefix := IndentSpaces
				// Show input if editing this file
				switch {
				case m.subEntryForm.EditingFile && m.subEntryForm.EditingFileIndex == i:
					fmt.Fprintf(&b, "%s%s\n", prefix, m.subEntryForm.NewFileInput.View())
				case ft == subFieldFiles && !m.subEntryForm.AddingFile && !m.subEntryForm.EditingFile && m.subEntryForm.FilesCursor == i:
					fmt.Fprintf(&b, "%s%s\n", prefix, SelectedMenuItemStyle.Render("• "+file))
				default:
					fmt.Fprintf(&b, "%s• %s\n", prefix, file)
				}
			}
		}

		// Add File button or input
		if m.subEntryForm.AddingFile {
			fmt.Fprintf(&b, "    %s\n", m.subEntryForm.NewFileInput.View())
		} else if !m.subEntryForm.EditingFile {
			addFileText := "[+ Add File]"
			if ft == subFieldFiles && m.subEntryForm.FilesCursor == len(m.subEntryForm.Files) {
				fmt.Fprintf(&b, "    %s\n", SelectedMenuItemStyle.Render(addFileText))
			} else {
				fmt.Fprintf(&b, "    %s\n", MutedTextStyle.Render(addFileText))
			}
		}

		b.WriteString("\n")
	}

	// Root toggle
	rootLabel := "Root only:"
	if ft == subFieldIsSudo {
		rootLabel = HelpKeyStyle.Render("Root only:")
	}

	rootCheck := CheckboxUnchecked
	if m.subEntryForm.IsSudo {
		rootCheck = CheckboxChecked
	}

	fmt.Fprintf(&b, "  %s  %s Yes\n\n", rootLabel, rootCheck)

	// Error message
	if m.subEntryForm.Err != "" {
		b.WriteString(ErrorStyle.Render("  Error: " + m.subEntryForm.Err))
		b.WriteString("\n\n")
	}

	// Success message
	if m.subEntryForm.SuccessMessage != "" {
		b.WriteString(SuccessStyle.Render("  " + m.subEntryForm.SuccessMessage))
		b.WriteString("\n\n")
	}

	// Help
	b.WriteString(m.renderSubEntryFormHelp())

	return BaseStyle.Render(b.String())
}

// renderSubEntryFieldValue renders a field value with appropriate styling
//
//nolint:unparam // placeholder parameter kept for consistency and future extensibility
func (m Model) renderSubEntryFieldValue(fieldType subEntryFieldType, placeholder string) string {
	if m.subEntryForm == nil {
		return placeholder
	}

	currentFt := m.getSubEntryFieldType()
	isEditing := m.subEntryForm.EditingField && currentFt == fieldType
	isFocused := currentFt == fieldType

	var input textinput.Model

	switch fieldType {
	case subFieldName:
		input = m.subEntryForm.NameInput
	case subFieldLinux:
		input = m.subEntryForm.LinuxTargetInput
	case subFieldWindows:
		input = m.subEntryForm.WindowsTargetInput
	case subFieldBackup:
		input = m.subEntryForm.BackupInput
	case subFieldIsFolder, subFieldFiles, subFieldIsSudo:
		return placeholder
	default:
		return placeholder
	}

	if isEditing {
		return input.View()
	}

	value := input.Value()
	if value == "" {
		value = MutedTextStyle.Render(placeholder)
	}

	if isFocused {
		return SelectedMenuItemStyle.Render(value)
	}

	return value
}

// renderSubEntryFormHelp renders context-sensitive help for the sub-entry form
func (m Model) renderSubEntryFormHelp() string {
	if m.subEntryForm == nil {
		return ""
	}

	ft := m.getSubEntryFieldType()

	if m.subEntryForm.AddingFile {
		return RenderHelpFromBindings(m.width,
			SearchKeys.Confirm,
			TextEditKeys.SaveForm,
			TextEditKeys.Cancel,
		)
	}

	if m.subEntryForm.EditingFile {
		return RenderHelpFromBindings(m.width,
			SearchKeys.Confirm,
			TextEditKeys.SaveForm,
			TextEditKeys.Cancel,
		)
	}

	if m.subEntryForm.EditingField {
		// Editing a text field
		if m.subEntryForm.ShowSuggestions && len(m.subEntryForm.Suggestions) > 0 && m.subEntryForm.SuggestionCursor >= 0 {
			return RenderHelpFromBindings(m.width,
				SuggestionKeys.Up,
				SuggestionKeys.Accept,
				TextEditKeys.SaveForm,
				TextEditKeys.Cancel,
			)
		}

		if m.subEntryForm.ShowSuggestions && len(m.subEntryForm.Suggestions) > 0 {
			return RenderHelpFromBindings(m.width,
				SuggestionKeys.Up,
				TextEditKeys.Confirm,
				TextEditKeys.SaveForm,
				TextEditKeys.Cancel,
			)
		}

		return RenderHelpFromBindings(m.width,
			TextEditKeys.Confirm,
			TextEditKeys.SaveForm,
			TextEditKeys.Cancel,
		)
	}

	if ft == subFieldFiles {
		// Files list focused
		if m.subEntryForm.FilesCursor < len(m.subEntryForm.Files) {
			return RenderHelpFromBindings(m.width,
				FilesListKeys.Edit,
				FilesListKeys.Delete,
				FilesListKeys.Save,
			)
		}

		return RenderHelpFromBindings(m.width,
			FilesListKeys.Edit,
			FilesListKeys.Save,
		)
	}

	if m.isSubEntryTextInputField() {
		// Text field focused (not editing)
		return RenderHelpFromBindings(m.width,
			FormNavKeys.Edit,
			FormNavKeys.Save,
		)
	}

	if m.isSubEntryToggleField() {
		// Toggle field focused
		return RenderHelpFromBindings(m.width,
			FormNavKeys.Toggle,
			FormNavKeys.Save,
		)
	}

	return RenderHelpFromBindings(m.width,
		FormNavKeys.Edit,
		FormNavKeys.Save,
	)
}

// renderSubEntrySuggestions renders the autocomplete dropdown
func (m Model) renderSubEntrySuggestions() string {
	if m.subEntryForm == nil || len(m.subEntryForm.Suggestions) == 0 {
		return ""
	}

	var b strings.Builder

	for i, suggestion := range m.subEntryForm.Suggestions {
		if i == m.subEntryForm.SuggestionCursor {
			fmt.Fprintf(&b, "  %s\n", SelectedMenuItemStyle.Render(suggestion))
		} else {
			fmt.Fprintf(&b, "  %s\n", MutedTextStyle.Render(suggestion))
		}
	}

	return b.String()
}

// saveSubEntryForm validates and saves the sub-entry form
func (m *Model) saveSubEntryForm() error {
	if m.subEntryForm == nil {
		return errors.New("no form data")
	}

	subEntry, err := m.subEntryForm.BuildSubEntry()
	if err != nil {
		return err
	}

	// Route to correct save operation
	if m.subEntryForm.EditAppIdx >= 0 && m.subEntryForm.EditSubIdx >= 0 {
		// Editing existing SubEntry
		return m.updateSubEntry(m.subEntryForm.EditAppIdx, m.subEntryForm.EditSubIdx, subEntry)
	} else if m.subEntryForm.TargetAppIdx >= 0 {
		// Adding SubEntry to existing Application
		return m.addSubEntryToApp(m.subEntryForm.TargetAppIdx, subEntry)
	}

	return fmt.Errorf("invalid form state")
}

// Helper functions

// updateSubEntryFormFocus updates which input field is focused
func (m *Model) updateSubEntryFormFocus() {
	if m.subEntryForm == nil {
		return
	}
	m.subEntryForm.UpdateFocus()
}

// enterSubEntryFieldEditMode enters edit mode for the current text field
func (m *Model) enterSubEntryFieldEditMode() {
	if m.subEntryForm == nil {
		return
	}
	m.subEntryForm.EnterFieldEditMode()
}

// cancelSubEntryFieldEdit cancels editing and restores the original value
func (m *Model) cancelSubEntryFieldEdit() {
	if m.subEntryForm == nil {
		return
	}
	m.subEntryForm.CancelFieldEdit()
}

// isSubEntryTextInputField returns true if the current field is a text input
func (m *Model) isSubEntryTextInputField() bool {
	if m.subEntryForm == nil {
		return false
	}
	return m.subEntryForm.IsTextInputField()
}

// isSubEntryToggleField returns true if the current field is a toggle
func (m *Model) isSubEntryToggleField() bool {
	if m.subEntryForm == nil {
		return false
	}
	return m.subEntryForm.IsToggleField()
}

// addSubEntryToApp adds a SubEntry to an existing Application
func (m *Model) addSubEntryToApp(appIdx int, subEntry config.SubEntry) error {
	if appIdx < 0 || appIdx >= len(m.Config.Applications) {
		return fmt.Errorf("invalid application index")
	}

	app := &m.Config.Applications[appIdx]

	// Check for duplicate SubEntry names within this Application
	for _, existing := range app.Entries {
		if existing.Name == subEntry.Name {
			return fmt.Errorf("a sub-entry with name '%s' already exists in this application", subEntry.Name)
		}
	}

	app.Entries = append(app.Entries, subEntry)

	if err := config.Save(m.Config, m.ConfigPath); err != nil {
		// Rollback
		app.Entries = app.Entries[:len(app.Entries)-1]
		return fmt.Errorf("failed to save config: %w", err)
	}

	m.reinitPreservingState(app.Name)

	return nil
}

// updateSubEntry updates an existing SubEntry
func (m *Model) updateSubEntry(appIdx, subIdx int, subEntry config.SubEntry) error {
	if appIdx < 0 || appIdx >= len(m.Config.Applications) {
		return fmt.Errorf("invalid application index")
	}

	app := &m.Config.Applications[appIdx]

	if subIdx < 0 || subIdx >= len(app.Entries) {
		return fmt.Errorf("invalid sub-entry index")
	}

	// Check for duplicate names (skip the one being edited)
	for i, existing := range app.Entries {
		if i != subIdx && existing.Name == subEntry.Name {
			return fmt.Errorf("a sub-entry with name '%s' already exists in this application", subEntry.Name)
		}
	}

	// Update SubEntry
	original := app.Entries[subIdx]
	app.Entries[subIdx] = subEntry

	if err := config.Save(m.Config, m.ConfigPath); err != nil {
		app.Entries[subIdx] = original // Rollback
		return fmt.Errorf("failed to save config: %w", err)
	}

	m.reinitPreservingState(app.Name)

	return nil
}

// NewSubEntryForm delegates to forms.NewSubEntryForm.
var NewSubEntryForm = forms.NewSubEntryForm
