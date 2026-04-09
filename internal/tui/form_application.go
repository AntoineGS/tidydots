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
type applicationFieldType = forms.ApplicationFieldType

const (
	appFieldName        = forms.AppFieldName
	appFieldDescription = forms.AppFieldDescription
	appFieldPackages    = forms.AppFieldPackages
	appFieldWhen        = forms.AppFieldWhen
)

// initApplicationForm initializes the application form.
// If appIdx >= 0, loads data from the existing application at that index (edit mode).
// If appIdx < 0, creates an empty form (new mode).
func (m *Model) initApplicationForm(appIdx int) {
	// Resolve config index and app data (edit mode only)
	configAppIdx := -1
	var app *config.Application

	if appIdx >= 0 {
		if appIdx >= len(m.Applications) {
			return
		}
		appName := m.Applications[appIdx].Application.Name
		configAppIdx = m.findConfigApplicationIndex(appName)
		if configAppIdx < 0 {
			return
		}
		app = &m.Config.Applications[configAppIdx]
	}

	nameInput := newFormInput(PlaceholderNeovim, CharLimitName, InputWidthNarrow)
	nameInput.Focus()

	descriptionInput := newFormInput("e.g., Neovim text editor", CharLimitDesc, InputWidthNarrow)
	packageNameInput := newFormInput(PlaceholderNeovim, CharLimitPkgName, InputWidthNarrow)
	whenInput := newFormInput(PlaceholderWhen, CharLimitWhen, InputWidthWide)

	gitURLInput, gitBranchInput, gitLinuxInput, gitWindowsInput := newGitTextInputs()
	installerLinuxInput, installerWindowsInput, installerBinaryInput := newInstallerTextInputs()

	depInput := newFormInput(PlaceholderDep, CharLimitDep, InputWidthNarrow)

	packageManagers := make(map[string]string)
	packageDeps := make(map[string][]string)
	hasGitPackage := false
	gitSudo := false
	hasInstallerPackage := false

	if app != nil {
		nameInput.SetValue(app.Name)
		descriptionInput.SetValue(app.Description)
		whenInput.SetValue(app.When)

		// Load package managers (only string-based managers, skip git and installer)
		if app.Package != nil && len(app.Package.Managers) > 0 {
			for k, v := range app.Package.Managers {
				if k == TypeGit || k == TypeInstaller {
					continue
				}
				if !v.IsGit() && !v.IsInstaller() {
					packageManagers[k] = v.PackageName
				}
			}
		}

		// Load git package if present
		if app.Package != nil {
			if gitVal, ok := app.Package.Managers[TypeGit]; ok && gitVal.IsGit() {
				hasGitPackage = true
				gitURLInput.SetValue(gitVal.Git.URL)
				gitBranchInput.SetValue(gitVal.Git.Branch)
				gitSudo = gitVal.Git.Sudo

				if target, ok := gitVal.Git.Targets[OSLinux]; ok {
					gitLinuxInput.SetValue(target)
				}
				if target, ok := gitVal.Git.Targets[OSWindows]; ok {
					gitWindowsInput.SetValue(target)
				}
			}
		}

		// Load installer package if present
		if app.Package != nil {
			if installerVal, ok := app.Package.Managers[TypeInstaller]; ok && installerVal.IsInstaller() {
				hasInstallerPackage = true
				if cmd, ok := installerVal.Installer.Command[OSLinux]; ok {
					installerLinuxInput.SetValue(cmd)
				}
				if cmd, ok := installerVal.Installer.Command[OSWindows]; ok {
					installerWindowsInput.SetValue(cmd)
				}
				installerBinaryInput.SetValue(installerVal.Installer.Binary)
			}
		}

		// Load package deps
		if app.Package != nil && len(app.Package.Managers) > 0 {
			for k, v := range app.Package.Managers {
				if k == TypeGit || k == TypeInstaller {
					continue
				}
				if len(v.Deps) > 0 {
					packageDeps[k] = append([]string{}, v.Deps...)
				}
			}
		}
	}

	m.applicationForm = &ApplicationForm{
		NameInput:             nameInput,
		DescriptionInput:      descriptionInput,
		PackageManagers:       packageManagers,
		PackagesCursor:        0,
		EditingPackage:        false,
		PackageNameInput:      packageNameInput,
		LastPackageName:       "",
		WhenInput:             whenInput,
		FocusIndex:            0,
		EditingField:          false,
		OriginalValue:         "",
		EditAppIdx:            configAppIdx,
		Err:                   "",
		GitURLInput:           gitURLInput,
		GitBranchInput:        gitBranchInput,
		GitLinuxInput:         gitLinuxInput,
		GitWindowsInput:       gitWindowsInput,
		GitFieldCursor:        -1,
		HasGitPackage:         hasGitPackage,
		GitSudo:               gitSudo,
		InstallerLinuxInput:   installerLinuxInput,
		InstallerWindowsInput: installerWindowsInput,
		InstallerBinaryInput:  installerBinaryInput,
		InstallerFieldCursor:  -1,
		HasInstallerPackage:   hasInstallerPackage,
		PackageDeps:           packageDeps,
		DepsCursor:            0,
		EditingDeps:           false,
		EditingDepItem:        false,
		DepsManagerKey:        "",
		DepInput:              depInput,
	}

	m.activeForm = FormApplication
	m.Screen = ScreenAddForm
}

// getApplicationFieldType returns the field type at the current focus index
func (m *Model) getApplicationFieldType() applicationFieldType {
	if m.applicationForm == nil {
		return appFieldName
	}

	return m.applicationForm.GetFieldType()
}

// updateApplicationForm handles key events for the application form
func (m Model) updateApplicationForm(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}

	// Handle deps list editing
	if m.applicationForm.EditingDeps {
		if m.applicationForm.EditingDepItem {
			return m.updateDepsItemInput(msg)
		}
		return m.updateDepsList(msg)
	}

	// Handle editing a git text field
	if m.applicationForm.EditingGitField {
		return m.updateApplicationGitFieldInput(msg)
	}

	// Handle editing an installer text field
	if m.applicationForm.EditingInstallerField {
		return m.updateApplicationInstallerFieldInput(msg)
	}

	// Handle editing a text field
	if m.applicationForm.EditingField {
		return m.updateApplicationFieldInput(msg)
	}

	// Handle editing package name
	if m.applicationForm.EditingPackage {
		return m.updateApplicationPackageInput(msg)
	}

	// Handle editing when expression
	if m.applicationForm.EditingWhen {
		return m.updateApplicationWhenInput(msg)
	}

	// Handle packages list navigation
	if m.getApplicationFieldType() == appFieldPackages {
		if m.applicationForm.PackagesCursor == len(displayPackageManagers) && m.applicationForm.GitFieldCursor >= 0 {
			return m.updateApplicationGitFields(msg)
		}
		if m.applicationForm.PackagesCursor == len(displayPackageManagers)+1 && m.applicationForm.InstallerFieldCursor >= 0 {
			return m.updateApplicationInstallerFields(msg)
		}
		return m.updateApplicationPackagesList(msg)
	}

	if m, cmd, handled := m.handleCommonKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, FormNavKeys.Cancel):
		// Return to list view
		m.activeForm = FormNone
		m.applicationForm = nil
		m.Screen = ScreenResults
		return m, nil

	case key.Matches(msg, FormNavKeys.Down):
		m.applicationForm.FocusIndex++
		if m.applicationForm.FocusIndex > 3 {
			m.applicationForm.FocusIndex = 0
		}
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.Up):
		m.applicationForm.FocusIndex--
		if m.applicationForm.FocusIndex < 0 {
			m.applicationForm.FocusIndex = 3
		}
		if m.getApplicationFieldType() == appFieldPackages {
			m.applicationForm.PackagesCursor = len(displayPackageManagers) + 1
			m.applicationForm.GitFieldCursor = -1
			m.applicationForm.InstallerFieldCursor = -1
		}
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.TabNext):
		m.applicationForm.FocusIndex++
		if m.applicationForm.FocusIndex > 3 {
			m.applicationForm.FocusIndex = 0
		}
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.TabPrev):
		m.applicationForm.FocusIndex--
		if m.applicationForm.FocusIndex < 0 {
			m.applicationForm.FocusIndex = 3
		}
		if m.getApplicationFieldType() == appFieldPackages {
			m.applicationForm.PackagesCursor = len(displayPackageManagers) + 1
			m.applicationForm.GitFieldCursor = -1
			m.applicationForm.InstallerFieldCursor = -1
		}
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.Edit):
		// Enter edit mode for text fields
		ft := m.getApplicationFieldType()
		if ft == appFieldName || ft == appFieldDescription {
			m.enterApplicationFieldEditMode()
			return m, nil
		}
		if ft == appFieldWhen {
			m.applicationForm.EditingWhen = true
			m.applicationForm.OriginalValue = m.applicationForm.WhenInput.Value()
			m.applicationForm.WhenInput.Focus()
			m.applicationForm.WhenInput.SetCursor(len(m.applicationForm.WhenInput.Value()))
			return m, nil
		}

	case key.Matches(msg, FormNavKeys.Save):
		// Save the form
		if err := m.saveApplicationForm(); err != nil {
			m.applicationForm.Err = err.Error()
			return m, nil
		}
		// Success - go back to list
		m.activeForm = FormNone
		m.applicationForm = nil
		m.Screen = ScreenResults
		return m, m.dispatchUncheckedPackageStates()
	}

	// Clear error when navigating
	m.applicationForm.Err = ""

	return m, nil
}

// updateApplicationFieldInput handles key events when editing a text field
func (m Model) updateApplicationFieldInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}

	var cmd tea.Cmd
	ft := m.getApplicationFieldType()

	if m, cmd, handled := m.handleTextEditKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, TextEditKeys.Cancel):
		// Cancel editing and restore original value
		m.cancelApplicationFieldEdit()
		return m, nil

	case key.Matches(msg, TextEditKeys.Confirm) || key.Matches(msg, TextEditKeys.SaveForm):
		// Save and exit edit mode
		m.applicationForm.EditingField = false
		m.updateApplicationFormFocus()
		return m, nil
	}

	// Handle text input for the focused field
	switch ft {
	case appFieldName:
		m.applicationForm.NameInput, cmd = m.applicationForm.NameInput.Update(msg)
	case appFieldDescription:
		m.applicationForm.DescriptionInput, cmd = m.applicationForm.DescriptionInput.Update(msg)
	case appFieldPackages, appFieldWhen:
		// List/when fields don't need text input updates here
	}

	// Clear error when typing
	m.applicationForm.Err = ""

	return m, cmd
}

// updateApplicationPackagesList handles key events when packages list is focused
func (m Model) updateApplicationPackagesList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}

	gitItemIdx := len(displayPackageManagers)
	installerItemIdx := len(displayPackageManagers) + 1
	maxCursor := installerItemIdx // includes git and installer items at the end

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
		if m.applicationForm.PackagesCursor > 0 {
			m.applicationForm.PackagesCursor--
			// Reset field cursors when moving between items
			m.applicationForm.GitFieldCursor = -1
			m.applicationForm.InstallerFieldCursor = -1
		} else {
			// Move to previous field
			m.applicationForm.FocusIndex--
			m.updateApplicationFormFocus()
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Down):
		switch {
		case m.applicationForm.PackagesCursor < maxCursor:
			// Moving to next item - handle git sub-field entry
			if m.applicationForm.PackagesCursor == gitItemIdx && m.applicationForm.HasGitPackage && m.applicationForm.GitFieldCursor == -1 {
				m.applicationForm.GitFieldCursor = 0
				return m, nil
			}
			m.applicationForm.PackagesCursor++
			m.applicationForm.GitFieldCursor = -1
			m.applicationForm.InstallerFieldCursor = -1
		case m.applicationForm.PackagesCursor == installerItemIdx && m.applicationForm.HasInstallerPackage && m.applicationForm.InstallerFieldCursor == -1:
			// Enter installer sub-fields (handled separately since installer is the last item, so packagesCursor == maxCursor)
			m.applicationForm.InstallerFieldCursor = 0
		default:
			// Move to next field
			m.applicationForm.FocusIndex++
			if m.applicationForm.FocusIndex > 3 {
				m.applicationForm.FocusIndex = 0
			}
			m.applicationForm.ResetCursors()
			m.updateApplicationFormFocus()
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.TabNext):
		m.applicationForm.FocusIndex++
		if m.applicationForm.FocusIndex > 3 {
			m.applicationForm.FocusIndex = 0
		}
		m.applicationForm.ResetCursors()
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FormNavKeys.TabPrev):
		m.applicationForm.FocusIndex--
		m.applicationForm.GitFieldCursor = -1
		m.applicationForm.InstallerFieldCursor = -1
		m.updateApplicationFormFocus()
		return m, nil

	case key.Matches(msg, FilesListKeys.Edit):
		return m.handlePackagesListActivate(gitItemIdx, installerItemIdx)

	case key.Matches(msg, FormNavKeys.Delete):
		return m.handlePackagesListDelete(gitItemIdx, installerItemIdx)

	case key.Matches(msg, key.NewBinding(key.WithKeys("p"))):
		// Enter deps editing for the current native manager
		if m.applicationForm.PackagesCursor >= 0 && m.applicationForm.PackagesCursor < len(displayPackageManagers) {
			manager := displayPackageManagers[m.applicationForm.PackagesCursor]
			m.applicationForm.DepsManagerKey = manager
			m.applicationForm.EditingDeps = true
			m.applicationForm.DepsCursor = 0
			return m, nil
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Save):
		// Save the form
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

// handlePackagesListActivate handles enter/space on the packages list
func (m Model) handlePackagesListActivate(gitItemIdx, installerItemIdx int) (tea.Model, tea.Cmd) {
	// Handle git item
	if m.applicationForm.PackagesCursor == gitItemIdx {
		if !m.applicationForm.HasGitPackage {
			m.applicationForm.HasGitPackage = true
		}
		m.applicationForm.GitFieldCursor = GitFieldURL
		return m, nil
	}
	// Handle installer item
	if m.applicationForm.PackagesCursor == installerItemIdx {
		if !m.applicationForm.HasInstallerPackage {
			m.applicationForm.HasInstallerPackage = true
		}
		m.applicationForm.InstallerFieldCursor = InstallerFieldLinux
		return m, nil
	}
	// Edit the selected package manager's package name
	if m.applicationForm.PackagesCursor < 0 || m.applicationForm.PackagesCursor >= len(displayPackageManagers) {
		return m, nil
	}
	manager := displayPackageManagers[m.applicationForm.PackagesCursor]
	currentValue := m.applicationForm.PackageManagers[manager]

	// Auto-populate with last package name if empty
	if currentValue == "" && m.applicationForm.LastPackageName != "" {
		currentValue = m.applicationForm.LastPackageName
	}

	m.applicationForm.EditingPackage = true
	m.applicationForm.PackageNameInput.SetValue(currentValue)
	m.applicationForm.PackageNameInput.Focus()
	m.applicationForm.PackageNameInput.SetCursor(len(currentValue))
	return m, nil
}

// handlePackagesListDelete handles delete/backspace on the packages list
func (m Model) handlePackagesListDelete(gitItemIdx, installerItemIdx int) (tea.Model, tea.Cmd) {
	// Handle git item deletion
	if m.applicationForm.PackagesCursor == gitItemIdx && m.applicationForm.GitFieldCursor == -1 {
		m.applicationForm.HasGitPackage = false
		m.applicationForm.GitFieldCursor = -1
		m.applicationForm.GitSudo = false
		m.applicationForm.GitURLInput.SetValue("")
		m.applicationForm.GitBranchInput.SetValue("")
		m.applicationForm.GitLinuxInput.SetValue("")
		m.applicationForm.GitWindowsInput.SetValue("")
		m.applicationForm.Err = ""
		return m, nil
	}
	// Handle installer item deletion
	if m.applicationForm.PackagesCursor == installerItemIdx && m.applicationForm.InstallerFieldCursor == -1 {
		m.applicationForm.HasInstallerPackage = false
		m.applicationForm.InstallerFieldCursor = -1
		m.applicationForm.InstallerLinuxInput.SetValue("")
		m.applicationForm.InstallerWindowsInput.SetValue("")
		m.applicationForm.InstallerBinaryInput.SetValue("")
		m.applicationForm.Err = ""
		return m, nil
	}
	// Clear the package name for the selected manager
	if m.applicationForm.PackagesCursor < 0 || m.applicationForm.PackagesCursor >= len(displayPackageManagers) {
		return m, nil
	}
	manager := displayPackageManagers[m.applicationForm.PackagesCursor]
	delete(m.applicationForm.PackageManagers, manager)
	m.applicationForm.Err = ""
	return m, nil
}

// updateDepsList handles navigation within the deps list
func (m Model) updateDepsList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}

	manager := m.applicationForm.DepsManagerKey
	deps := m.applicationForm.PackageDeps[manager]
	maxIdx := len(deps) // last position is "Add" button

	switch {
	case key.Matches(msg, FormNavKeys.Cancel):
		m.applicationForm.EditingDeps = false
		m.applicationForm.DepsManagerKey = ""
		return m, nil

	case key.Matches(msg, FormNavKeys.Up):
		if m.applicationForm.DepsCursor > 0 {
			m.applicationForm.DepsCursor--
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Down):
		if m.applicationForm.DepsCursor < maxIdx {
			m.applicationForm.DepsCursor++
		}
		return m, nil

	case key.Matches(msg, FilesListKeys.Edit):
		if m.applicationForm.DepsCursor == maxIdx {
			// Add new dep
			m.applicationForm.EditingDepItem = true
			m.applicationForm.DepInput.SetValue("")
			m.applicationForm.DepInput.Focus()
			return m, nil
		}
		if m.applicationForm.DepsCursor < len(deps) {
			// Edit existing dep
			m.applicationForm.EditingDepItem = true
			m.applicationForm.DepInput.SetValue(deps[m.applicationForm.DepsCursor])
			m.applicationForm.DepInput.Focus()
			m.applicationForm.DepInput.SetCursor(len(deps[m.applicationForm.DepsCursor]))
			return m, nil
		}
		return m, nil

	case key.Matches(msg, FormNavKeys.Delete):
		if m.applicationForm.DepsCursor < len(deps) {
			deps = append(deps[:m.applicationForm.DepsCursor], deps[m.applicationForm.DepsCursor+1:]...)
			if len(deps) == 0 {
				delete(m.applicationForm.PackageDeps, manager)
			} else {
				m.applicationForm.PackageDeps[manager] = deps
			}
			if m.applicationForm.DepsCursor > 0 && m.applicationForm.DepsCursor >= len(deps) {
				m.applicationForm.DepsCursor--
			}
		}
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

// updateDepsItemInput handles text input when editing a dep item
func (m Model) updateDepsItemInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}

	var cmd tea.Cmd

	if m, cmd, handled := m.handleTextEditKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, TextEditKeys.Cancel):
		m.applicationForm.EditingDepItem = false
		m.applicationForm.DepInput.SetValue("")
		return m, nil

	case key.Matches(msg, TextEditKeys.Confirm):
		value := strings.TrimSpace(m.applicationForm.DepInput.Value())
		if value != "" {
			manager := m.applicationForm.DepsManagerKey
			deps := m.applicationForm.PackageDeps[manager]
			if m.applicationForm.DepsCursor < len(deps) {
				// Edit existing
				deps[m.applicationForm.DepsCursor] = value
				m.applicationForm.PackageDeps[manager] = deps
			} else {
				// Add new
				m.applicationForm.PackageDeps[manager] = append(deps, value)
			}
		}
		m.applicationForm.EditingDepItem = false
		m.applicationForm.DepInput.SetValue("")
		return m, nil
	}

	m.applicationForm.DepInput, cmd = m.applicationForm.DepInput.Update(msg)
	return m, cmd
}

// updateApplicationPackageInput handles key events when editing a package name
func (m Model) updateApplicationPackageInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}

	var cmd tea.Cmd

	if m, cmd, handled := m.handleTextEditKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, TextEditKeys.Cancel):
		// Cancel editing package name
		m.applicationForm.EditingPackage = false
		m.applicationForm.PackageNameInput.SetValue("")
		m.applicationForm.Err = ""
		return m, nil

	case key.Matches(msg, SearchKeys.Confirm) || key.Matches(msg, TextEditKeys.SaveForm):
		pkgName := strings.TrimSpace(m.applicationForm.PackageNameInput.Value())
		if m.applicationForm.PackagesCursor < 0 || m.applicationForm.PackagesCursor >= len(displayPackageManagers) {
			m.applicationForm.EditingPackage = false
			m.applicationForm.PackageNameInput.SetValue("")
			return m, nil
		}
		manager := displayPackageManagers[m.applicationForm.PackagesCursor]

		if pkgName != "" {
			m.applicationForm.PackageManagers[manager] = pkgName
			m.applicationForm.LastPackageName = pkgName // Remember for auto-populate
		} else {
			// Clear if empty
			delete(m.applicationForm.PackageManagers, manager)
		}

		m.applicationForm.EditingPackage = false
		m.applicationForm.PackageNameInput.SetValue("")
		return m, nil
	}

	// Handle text input
	m.applicationForm.PackageNameInput, cmd = m.applicationForm.PackageNameInput.Update(msg)
	m.applicationForm.Err = ""
	return m, cmd
}

// updateApplicationWhenInput handles key events when editing the when expression
func (m Model) updateApplicationWhenInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.applicationForm == nil {
		return m, nil
	}

	var cmd tea.Cmd

	if m, cmd, handled := m.handleTextEditKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, TextEditKeys.Cancel):
		// Cancel editing and restore original value
		m.applicationForm.WhenInput.SetValue(m.applicationForm.OriginalValue)
		m.applicationForm.EditingWhen = false
		m.applicationForm.WhenInput.Blur()
		m.applicationForm.Err = ""
		return m, nil

	case key.Matches(msg, TextEditKeys.Confirm) || key.Matches(msg, TextEditKeys.SaveForm):
		// Save and exit edit mode
		m.applicationForm.EditingWhen = false
		m.applicationForm.WhenInput.Blur()
		return m, nil
	}

	// Handle text input
	m.applicationForm.WhenInput, cmd = m.applicationForm.WhenInput.Update(msg)
	m.applicationForm.Err = ""

	return m, cmd
}

// viewApplicationForm renders the application form
func (m Model) viewApplicationForm() string {
	if m.applicationForm == nil {
		return ""
	}

	var b strings.Builder
	ft := m.getApplicationFieldType()

	// Name field
	nameLabel := "Name:"
	if ft == appFieldName {
		nameLabel = HelpKeyStyle.Render("Name:")
	}
	fmt.Fprintf(&b, "  %s\n", nameLabel)
	fmt.Fprintf(&b, "  %s\n\n", m.renderApplicationFieldValue(appFieldName, "(empty)"))

	// Description field
	descLabel := "Description:"
	if ft == appFieldDescription {
		descLabel = HelpKeyStyle.Render("Description:")
	}
	fmt.Fprintf(&b, "  %s\n", descLabel)
	fmt.Fprintf(&b, "  %s\n\n", m.renderApplicationFieldValue(appFieldDescription, "(optional)"))

	// Packages section
	packagesLabel := "Packages:"
	if ft == appFieldPackages {
		packagesLabel = HelpKeyStyle.Render("Packages:")
	}
	fmt.Fprintf(&b, "  %s\n", packagesLabel)

	if m.applicationForm.EditingDeps {
		// Show deps editing view instead of regular packages section
		b.WriteString(renderDepsSection(
			m.applicationForm.DepsManagerKey,
			m.applicationForm.PackageDeps[m.applicationForm.DepsManagerKey],
			m.applicationForm.DepsCursor,
			m.applicationForm.EditingDepItem,
			m.applicationForm.DepInput,
		))
	} else {
		b.WriteString(renderPackagesSection(
			ft == appFieldPackages,
			m.applicationForm.PackageManagers,
			m.applicationForm.PackagesCursor,
			m.applicationForm.EditingPackage,
			m.applicationForm.PackageNameInput,
			m.applicationForm.PackageDeps,
		))
		onGitItem := ft == appFieldPackages && m.applicationForm.PackagesCursor == len(displayPackageManagers)
		b.WriteString(renderGitPackageSection(
			ft == appFieldPackages,
			onGitItem,
			m.applicationForm.HasGitPackage,
			m.applicationForm.GitFieldCursor,
			m.applicationForm.EditingGitField,
			m.applicationForm.GitURLInput,
			m.applicationForm.GitBranchInput,
			m.applicationForm.GitLinuxInput,
			m.applicationForm.GitWindowsInput,
			m.applicationForm.GitSudo,
		))
		onInstallerItem := ft == appFieldPackages && m.applicationForm.PackagesCursor == len(displayPackageManagers)+1
		b.WriteString(renderInstallerPackageSection(
			ft == appFieldPackages,
			onInstallerItem,
			m.applicationForm.HasInstallerPackage,
			m.applicationForm.InstallerFieldCursor,
			m.applicationForm.EditingInstallerField,
			m.applicationForm.InstallerLinuxInput,
			m.applicationForm.InstallerWindowsInput,
			m.applicationForm.InstallerBinaryInput,
		))
	}
	b.WriteString("\n")

	// When section
	whenLabel := "When:"
	if ft == appFieldWhen {
		whenLabel = HelpKeyStyle.Render("When:")
	}
	fmt.Fprintf(&b, "  %s\n", whenLabel)
	b.WriteString(renderWhenField(
		ft == appFieldWhen,
		m.applicationForm.EditingWhen,
		m.applicationForm.WhenInput,
	))
	b.WriteString("\n")

	// Error message
	if m.applicationForm.Err != "" {
		b.WriteString(ErrorStyle.Render("  Error: " + m.applicationForm.Err))
		b.WriteString("\n\n")
	}

	// Help
	b.WriteString(m.renderApplicationFormHelp())

	return BaseStyle.Render(b.String())
}

// renderApplicationFieldValue renders a field value with appropriate styling
func (m Model) renderApplicationFieldValue(fieldType applicationFieldType, placeholder string) string {
	if m.applicationForm == nil {
		return placeholder
	}

	currentFt := m.getApplicationFieldType()
	isEditing := m.applicationForm.EditingField && currentFt == fieldType
	isFocused := currentFt == fieldType

	var input textinput.Model
	switch fieldType {
	case appFieldName:
		input = m.applicationForm.NameInput
	case appFieldDescription:
		input = m.applicationForm.DescriptionInput
	case appFieldPackages, appFieldWhen:
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

// renderApplicationFormHelp renders context-sensitive help for the application form
func (m Model) renderApplicationFormHelp() string {
	if m.applicationForm == nil {
		return ""
	}

	ft := m.getApplicationFieldType()

	if m.applicationForm.EditingDeps {
		if m.applicationForm.EditingDepItem {
			return RenderHelpFromBindings(m.width,
				TextEditKeys.Confirm,
				TextEditKeys.Cancel,
			)
		}
		depsBinding := key.NewBinding(key.WithKeys("enter", "e"), key.WithHelp("enter/e", "edit"))
		return RenderHelpFromBindings(m.width,
			depsBinding,
			FormNavKeys.Delete,
			FormNavKeys.Cancel,
		)
	}

	if m.applicationForm.EditingGitField || m.applicationForm.EditingInstallerField {
		return RenderHelpFromBindings(m.width,
			TextEditKeys.Confirm,
			TextEditKeys.SaveForm,
			TextEditKeys.Cancel,
		)
	}

	if m.applicationForm.EditingPackage {
		return RenderHelpFromBindings(m.width,
			TextEditKeys.Confirm,
			TextEditKeys.SaveForm,
			TextEditKeys.Cancel,
		)
	}

	if m.applicationForm.EditingWhen {
		return RenderHelpFromBindings(m.width,
			TextEditKeys.Confirm,
			TextEditKeys.SaveForm,
			TextEditKeys.Cancel,
		)
	}

	if m.applicationForm.EditingField {
		return RenderHelpFromBindings(m.width,
			TextEditKeys.Confirm,
			TextEditKeys.SaveForm,
			TextEditKeys.Cancel,
		)
	}

	if ft == appFieldPackages {
		// Git package states
		if m.applicationForm.PackagesCursor == len(displayPackageManagers) {
			if !m.applicationForm.HasGitPackage {
				addBinding := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "add"))
				return RenderHelpFromBindings(m.width, addBinding, FormNavKeys.Save)
			}
			if m.applicationForm.GitFieldCursor == -1 {
				return RenderHelpFromBindings(m.width, FormNavKeys.Delete, FormNavKeys.Save)
			}
			if m.applicationForm.GitFieldCursor == GitFieldSudo {
				return RenderHelpFromBindings(m.width, FormNavKeys.Toggle, FormNavKeys.Save)
			}
			return RenderHelpFromBindings(m.width, FormNavKeys.Edit, FormNavKeys.Save)
		}
		// Installer package states
		if m.applicationForm.PackagesCursor == len(displayPackageManagers)+1 {
			if !m.applicationForm.HasInstallerPackage {
				addBinding := key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "add"))
				return RenderHelpFromBindings(m.width, addBinding, FormNavKeys.Save)
			}
			if m.applicationForm.InstallerFieldCursor == -1 {
				return RenderHelpFromBindings(m.width, FormNavKeys.Delete, FormNavKeys.Save)
			}
			return RenderHelpFromBindings(m.width, FormNavKeys.Edit, FormNavKeys.Save)
		}
		// Bounds check for packagesCursor
		depsBinding := key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "deps"))
		if m.applicationForm.PackagesCursor >= 0 && m.applicationForm.PackagesCursor < len(displayPackageManagers) {
			manager := displayPackageManagers[m.applicationForm.PackagesCursor]
			if m.applicationForm.PackageManagers[manager] != "" {
				return RenderHelpFromBindings(m.width,
					FormNavKeys.Edit,
					depsBinding,
					FormNavKeys.Delete,
					FormNavKeys.Save,
				)
			}
		}
		return RenderHelpFromBindings(m.width,
			FormNavKeys.Edit,
			depsBinding,
			FormNavKeys.Save,
		)
	}

	if ft == appFieldWhen {
		return RenderHelpFromBindings(m.width,
			FormNavKeys.Edit,
			FormNavKeys.Save,
		)
	}

	// Text field focused (not editing)
	return RenderHelpFromBindings(m.width,
		FormNavKeys.Edit,
		FormNavKeys.Save,
	)
}

// saveApplicationForm validates and saves the application form
func (m *Model) saveApplicationForm() error {
	if m.applicationForm == nil {
		return errors.New("no form data")
	}

	name, description, when, pkg, err := m.applicationForm.BuildApplication()
	if err != nil {
		return err
	}

	// Save based on edit mode
	if m.applicationForm.EditAppIdx >= 0 {
		return m.saveEditedApplication(m.applicationForm.EditAppIdx, name, description, when, pkg)
	}
	return m.saveNewApplication(config.Application{
		Name:        name,
		Description: description,
		When:        when,
		Package:     pkg,
		Entries:     []config.SubEntry{}, // Empty entries initially
	})
}

// updateApplicationFormFocus updates which input field is focused
func (m *Model) updateApplicationFormFocus() {
	if m.applicationForm == nil {
		return
	}
	m.applicationForm.UpdateFocus()
}

// enterApplicationFieldEditMode enters edit mode for the current text field
func (m *Model) enterApplicationFieldEditMode() {
	if m.applicationForm == nil {
		return
	}
	m.applicationForm.EnterFieldEditMode()
}

// cancelApplicationFieldEdit cancels editing and restores the original value
func (m *Model) cancelApplicationFieldEdit() {
	if m.applicationForm == nil {
		return
	}
	m.applicationForm.CancelFieldEdit()
}

// NewApplicationForm delegates to forms.NewApplicationForm.
var NewApplicationForm = forms.NewApplicationForm
