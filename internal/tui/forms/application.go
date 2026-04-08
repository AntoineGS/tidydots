package forms

import (
	"errors"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/tui/tuishared"
)

// ApplicationFieldType represents the type of field in the ApplicationForm
type ApplicationFieldType int

// ApplicationForm field type constants.
const (
	AppFieldName ApplicationFieldType = iota
	AppFieldDescription
	AppFieldPackages
	AppFieldWhen
)

// ApplicationForm holds state for editing Application metadata
type ApplicationForm struct {
	PackageManagers  map[string]string
	LastPackageName  string
	Err              string
	OriginalValue    string
	DescriptionInput textinput.Model
	PackageNameInput textinput.Model
	NameInput        textinput.Model
	WhenInput        textinput.Model
	EditAppIdx       int
	PackagesCursor   int
	FocusIndex       int
	EditingField     bool
	EditingPackage   bool
	EditingWhen      bool

	// Git package fields
	GitURLInput     textinput.Model
	GitBranchInput  textinput.Model
	GitLinuxInput   textinput.Model
	GitWindowsInput textinput.Model
	GitFieldCursor  int  // -1 = on git label/button, 0-4 = on sub-fields
	EditingGitField bool // true when editing a git text field
	HasGitPackage   bool // true when git package is configured/expanded
	GitSudo         bool // sudo toggle for git package

	// Installer package fields
	InstallerLinuxInput   textinput.Model
	InstallerWindowsInput textinput.Model
	InstallerBinaryInput  textinput.Model
	InstallerFieldCursor  int  // -1 = on installer label/button, 0-2 = on sub-fields
	EditingInstallerField bool // true when editing an installer text field
	HasInstallerPackage   bool // true when installer package is configured/expanded

	// Package dependency fields
	PackageDeps    map[string][]string // manager -> deps list
	DepsCursor     int                 // cursor within deps list
	EditingDeps    bool                // true when in deps editing mode
	EditingDepItem bool                // true when editing a dep text input
	DepsManagerKey string              // which manager's deps we're editing
	DepInput       textinput.Model     // text input for adding/editing deps
}

// ResetCursors resets all cursor and sub-field state on the ApplicationForm.
func (f *ApplicationForm) ResetCursors() {
	f.PackagesCursor = 0
	f.GitFieldCursor = -1
	f.InstallerFieldCursor = -1
	f.EditingGitField = false
	f.EditingInstallerField = false
	f.EditingPackage = false
	f.EditingDeps = false
	f.EditingDepItem = false
}

// GetFieldType returns the field type at the current focus index
func (f *ApplicationForm) GetFieldType() ApplicationFieldType {
	if f == nil {
		return AppFieldName
	}

	switch f.FocusIndex {
	case 0:
		return AppFieldName
	case 1:
		return AppFieldDescription
	case 2:
		return AppFieldPackages
	case 3:
		return AppFieldWhen
	default:
		return AppFieldName
	}
}

// Validate checks if the ApplicationForm has valid data
func (f *ApplicationForm) Validate() error {
	if strings.TrimSpace(f.NameInput.Value()) == "" {
		return errors.New("application name is required")
	}
	return nil
}

// GetGitFieldInput returns a pointer to the current git text input based on GitFieldCursor
func (f *ApplicationForm) GetGitFieldInput() *textinput.Model {
	if f == nil {
		return nil
	}

	switch f.GitFieldCursor {
	case tuishared.GitFieldURL:
		return &f.GitURLInput
	case tuishared.GitFieldBranch:
		return &f.GitBranchInput
	case tuishared.GitFieldLinux:
		return &f.GitLinuxInput
	case tuishared.GitFieldWindows:
		return &f.GitWindowsInput
	default:
		return nil
	}
}

// GetInstallerFieldInput returns a pointer to the current installer text input based on InstallerFieldCursor
func (f *ApplicationForm) GetInstallerFieldInput() *textinput.Model {
	if f == nil {
		return nil
	}

	switch f.InstallerFieldCursor {
	case tuishared.InstallerFieldLinux:
		return &f.InstallerLinuxInput
	case tuishared.InstallerFieldWindows:
		return &f.InstallerWindowsInput
	case tuishared.InstallerFieldBinary:
		return &f.InstallerBinaryInput
	default:
		return nil
	}
}

// UpdateFocus updates which input field is focused
func (f *ApplicationForm) UpdateFocus() {
	if f == nil {
		return
	}

	f.NameInput.Blur()
	f.DescriptionInput.Blur()

	ft := f.GetFieldType()
	switch ft {
	case AppFieldName:
		f.NameInput.Focus()
	case AppFieldDescription:
		f.DescriptionInput.Focus()
	case AppFieldPackages:
		// List fields don't use textinput focus
	case AppFieldWhen:
		// When field focus is handled separately
	}
}

// EnterFieldEditMode enters edit mode for the current text field
func (f *ApplicationForm) EnterFieldEditMode() {
	if f == nil {
		return
	}

	f.EditingField = true
	ft := f.GetFieldType()

	switch ft {
	case AppFieldName:
		f.OriginalValue = f.NameInput.Value()
		f.NameInput.Focus()
		f.NameInput.SetCursor(len(f.NameInput.Value()))
	case AppFieldDescription:
		f.OriginalValue = f.DescriptionInput.Value()
		f.DescriptionInput.Focus()
		f.DescriptionInput.SetCursor(len(f.DescriptionInput.Value()))
	case AppFieldPackages:
		// List fields don't use text input editing
	case AppFieldWhen:
		// When field has its own edit mode
	}
}

// CancelFieldEdit cancels editing and restores the original value
func (f *ApplicationForm) CancelFieldEdit() {
	if f == nil {
		return
	}

	ft := f.GetFieldType()
	switch ft {
	case AppFieldName:
		f.NameInput.SetValue(f.OriginalValue)
	case AppFieldDescription:
		f.DescriptionInput.SetValue(f.OriginalValue)
	case AppFieldPackages:
		// List fields don't use text input restoration
	case AppFieldWhen:
		// When field has its own cancel handling
	}

	f.EditingField = false
	f.Err = ""
	f.UpdateFocus()
}

// BuildApplication validates and returns the application data from the form.
// Returns the name, description, when, and package spec, or an error if validation fails.
func (f *ApplicationForm) BuildApplication() (name, description, when string, pkg *config.EntryPackage, err error) {
	if f == nil {
		return "", "", "", nil, errors.New("no form data")
	}

	name = strings.TrimSpace(f.NameInput.Value())
	description = strings.TrimSpace(f.DescriptionInput.Value())

	// Validation
	if name == "" {
		return "", "", "", nil, errors.New("name is required")
	}

	// Build when expression and package
	when = strings.TrimSpace(f.WhenInput.Value())
	pkg = BuildPackageSpec(f.PackageManagers)

	// Merge git package data
	pkg = MergeGitPackage(
		pkg,
		f.HasGitPackage,
		f.GitURLInput,
		f.GitBranchInput,
		f.GitLinuxInput,
		f.GitWindowsInput,
		f.GitSudo,
	)

	// Validate git package if present
	if f.HasGitPackage {
		gitURL := strings.TrimSpace(f.GitURLInput.Value())
		if gitURL == "" {
			return "", "", "", nil, errors.New("git package URL is required")
		}
		gitLinux := strings.TrimSpace(f.GitLinuxInput.Value())
		gitWindows := strings.TrimSpace(f.GitWindowsInput.Value())
		if gitLinux == "" && gitWindows == "" {
			return "", "", "", nil, errors.New("git package requires at least one target (Linux or Windows)")
		}
	}

	// Merge installer package data
	pkg = MergeInstallerPackage(
		pkg,
		f.HasInstallerPackage,
		f.InstallerLinuxInput,
		f.InstallerWindowsInput,
		f.InstallerBinaryInput,
	)

	// Validate installer package if present
	if f.HasInstallerPackage {
		installerLinux := strings.TrimSpace(f.InstallerLinuxInput.Value())
		installerWindows := strings.TrimSpace(f.InstallerWindowsInput.Value())
		if installerLinux == "" && installerWindows == "" {
			return "", "", "", nil, errors.New("installer package requires at least one command (Linux or Windows)")
		}
	}

	// Merge deps into package managers
	if pkg != nil && len(f.PackageDeps) > 0 {
		for manager, deps := range f.PackageDeps {
			if mv, ok := pkg.Managers[manager]; ok {
				mv.Deps = deps
				pkg.Managers[manager] = mv
			} else if len(deps) > 0 {
				// Deps-only entry (no main package name)
				pkg.Managers[manager] = config.ManagerValue{Deps: deps}
			}
		}
	} else if pkg == nil && len(f.PackageDeps) > 0 {
		pkg = &config.EntryPackage{
			Managers: make(map[string]config.ManagerValue),
		}
		for manager, deps := range f.PackageDeps {
			if len(deps) > 0 {
				pkg.Managers[manager] = config.ManagerValue{Deps: deps}
			}
		}
	}

	return name, description, when, pkg, nil
}

// NewApplicationForm creates a new ApplicationForm for testing purposes
func NewApplicationForm(app config.Application, isEdit bool) *ApplicationForm {
	nameInput := NewFormInput(tuishared.PlaceholderNeovim, tuishared.CharLimitName, tuishared.InputWidthNarrow)
	nameInput.SetValue(app.Name)

	descriptionInput := NewFormInput("e.g., Neovim text editor", tuishared.CharLimitDesc, tuishared.InputWidthNarrow)
	descriptionInput.SetValue(app.Description)

	whenInput := NewFormInput(tuishared.PlaceholderWhen, tuishared.CharLimitWhen, tuishared.InputWidthWide)
	whenInput.SetValue(app.When)

	editAppIdx := -1
	if isEdit {
		editAppIdx = 0
	}

	gitURLInput, gitBranchInput, gitLinuxInput, gitWindowsInput := NewGitTextInputs()
	installerLinuxInput, installerWindowsInput, installerBinaryInput := NewInstallerTextInputs()

	// Load package managers (only string-based managers, skip git and installer)
	packageManagers := make(map[string]string)
	if app.Package != nil && len(app.Package.Managers) > 0 {
		for k, v := range app.Package.Managers {
			if k == tuishared.TypeGit || k == tuishared.TypeInstaller {
				continue
			}
			if !v.IsGit() && !v.IsInstaller() {
				packageManagers[k] = v.PackageName
			}
		}
	}

	// Load git package if present
	hasGitPackage := false
	gitSudo := false

	if app.Package != nil {
		if gitVal, ok := app.Package.Managers[tuishared.TypeGit]; ok && gitVal.IsGit() {
			hasGitPackage = true
			gitURLInput.SetValue(gitVal.Git.URL)
			gitBranchInput.SetValue(gitVal.Git.Branch)
			gitSudo = gitVal.Git.Sudo

			if target, ok := gitVal.Git.Targets[tuishared.OSLinux]; ok {
				gitLinuxInput.SetValue(target)
			}
			if target, ok := gitVal.Git.Targets[tuishared.OSWindows]; ok {
				gitWindowsInput.SetValue(target)
			}
		}
	}

	// Load installer package if present
	hasInstallerPackage := false

	if app.Package != nil {
		if installerVal, ok := app.Package.Managers[tuishared.TypeInstaller]; ok && installerVal.IsInstaller() {
			hasInstallerPackage = true
			if cmd, ok := installerVal.Installer.Command[tuishared.OSLinux]; ok {
				installerLinuxInput.SetValue(cmd)
			}
			if cmd, ok := installerVal.Installer.Command[tuishared.OSWindows]; ok {
				installerWindowsInput.SetValue(cmd)
			}
			installerBinaryInput.SetValue(installerVal.Installer.Binary)
		}
	}

	// Load package deps
	packageDeps := make(map[string][]string)
	if app.Package != nil && len(app.Package.Managers) > 0 {
		for k, v := range app.Package.Managers {
			if k == tuishared.TypeGit || k == tuishared.TypeInstaller {
				continue
			}
			if len(v.Deps) > 0 {
				packageDeps[k] = append([]string{}, v.Deps...)
			}
		}
	}

	depInput := NewFormInput(tuishared.PlaceholderDep, tuishared.CharLimitDep, tuishared.InputWidthNarrow)

	return &ApplicationForm{
		NameInput:             nameInput,
		DescriptionInput:      descriptionInput,
		WhenInput:             whenInput,
		PackageManagers:       packageManagers,
		EditAppIdx:            editAppIdx,
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
}
