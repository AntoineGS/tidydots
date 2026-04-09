package forms

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/tui/tuishared"
)

// DisplayPackageManagers is platform.KnownPackageManagers excluding "git"
// (git is handled as a special case, not shown in the package manager form)
var DisplayPackageManagers = func() []string {
	var managers []string
	for _, m := range platform.KnownPackageManagers {
		if m != "git" {
			managers = append(managers, m)
		}
	}
	return managers
}()

// NewFormInput creates a textinput.Model with standard configuration.
func NewFormInput(placeholder string, charLimit int, width int) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.CharLimit = charLimit
	ti.SetWidth(width)
	return ti
}

// NewGitTextInputs creates the four git text inputs with standard placeholders and char limits
func NewGitTextInputs() (gitURLInput, gitBranchInput, gitLinuxInput, gitWindowsInput textinput.Model) {
	gitURLInput = NewFormInput(tuishared.PlaceholderGitURL, tuishared.CharLimitPath, tuishared.InputWidthNarrow)
	gitBranchInput = NewFormInput(tuishared.PlaceholderGitBranch, tuishared.CharLimitBranch, tuishared.InputWidthNarrow)
	gitLinuxInput = NewFormInput(tuishared.PlaceholderGitLinux, tuishared.CharLimitPath, tuishared.InputWidthNarrow)
	gitWindowsInput = NewFormInput(tuishared.PlaceholderGitWindows, tuishared.CharLimitPath, tuishared.InputWidthNarrow)
	return gitURLInput, gitBranchInput, gitLinuxInput, gitWindowsInput
}

// NewInstallerTextInputs creates the three installer text inputs with standard placeholders and char limits
func NewInstallerTextInputs() (installerLinuxInput, installerWindowsInput, installerBinaryInput textinput.Model) {
	installerLinuxInput = NewFormInput(tuishared.PlaceholderInstallerLinux, tuishared.CharLimitURL, tuishared.InputWidthNarrow)
	installerWindowsInput = NewFormInput(tuishared.PlaceholderInstallerWindows, tuishared.CharLimitURL, tuishared.InputWidthNarrow)
	installerBinaryInput = NewFormInput(tuishared.PlaceholderInstallerBinary, tuishared.CharLimitBinary, tuishared.InputWidthNarrow)
	return installerLinuxInput, installerWindowsInput, installerBinaryInput
}

// RenderPackagesSection renders the packages list with editing state
// focused indicates if the packages section is currently focused
// packageManagers is the map of manager -> package name
// packagesCursor is the current cursor position within the list
// editingPackage indicates if currently editing a package name
// packageNameInput is the text input for editing package name
func RenderPackagesSection(
	focused bool,
	packageManagers map[string]string,
	packagesCursor int,
	editingPackage bool,
	packageNameInput textinput.Model,
	packageDeps map[string][]string,
) string {
	var b strings.Builder

	for i, manager := range DisplayPackageManagers {
		prefix := tuishared.IndentSpaces
		pkgName := packageManagers[manager]

		depsCount := 0
		if packageDeps != nil {
			depsCount = len(packageDeps[manager])
		}
		depsIndicator := ""
		if depsCount > 0 {
			depsIndicator = fmt.Sprintf(" (%d deps)", depsCount)
		}

		// Show input if editing this manager's package
		switch {
		case editingPackage && packagesCursor == i:
			fmt.Fprintf(&b, "%s%-8s %s\n", prefix, manager+":", packageNameInput.View())
		case focused && packagesCursor == i:
			// Focused on this manager
			if pkgName != "" {
				fmt.Fprintf(&b, "%s%s\n", prefix, tuishared.SelectedMenuItemStyle.Render(fmt.Sprintf("%-8s %s%s", manager+":", pkgName, depsIndicator)))
			} else {
				fmt.Fprintf(&b, "%s%s\n", prefix, tuishared.SelectedMenuItemStyle.Render(fmt.Sprintf("%-8s (not set)%s", manager+":", depsIndicator)))
			}
		default:
			// Not focused
			if pkgName != "" {
				fmt.Fprintf(&b, "%s%-8s %s%s\n", prefix, manager+":", pkgName, depsIndicator)
			} else {
				fmt.Fprintf(&b, "%s%-8s %s%s\n", prefix, manager+":", tuishared.MutedTextStyle.Render("(not set)"), depsIndicator)
			}
		}
	}

	return b.String()
}

// RenderDepsSection renders the deps editing view for a specific package manager
func RenderDepsSection(
	manager string,
	deps []string,
	cursor int,
	editingItem bool,
	depInput textinput.Model,
) string {
	var b strings.Builder
	prefix := tuishared.IndentSpaces

	fmt.Fprintf(&b, "%s%s\n", prefix, tuishared.HelpKeyStyle.Render(fmt.Sprintf("Dependencies for %s:", manager)))

	for i, dep := range deps {
		if editingItem && cursor == i {
			fmt.Fprintf(&b, "%s  %s\n", prefix, depInput.View())
		} else if cursor == i {
			fmt.Fprintf(&b, "%s  %s\n", prefix, tuishared.SelectedMenuItemStyle.Render(dep))
		} else {
			fmt.Fprintf(&b, "%s  %s\n", prefix, dep)
		}
	}

	// Add button
	addText := "[+ Add dependency]"
	if editingItem && cursor == len(deps) {
		fmt.Fprintf(&b, "%s  %s\n", prefix, depInput.View())
	} else if cursor == len(deps) {
		fmt.Fprintf(&b, "%s  %s\n", prefix, tuishared.SelectedMenuItemStyle.Render(addText))
	} else {
		fmt.Fprintf(&b, "%s  %s\n", prefix, tuishared.MutedTextStyle.Render(addText))
	}

	return b.String()
}

// RenderGitPackageSection renders the git package section within Packages
// focused indicates if the packages section is currently focused
// onGitItem indicates if the packagesCursor is on the git item (last position)
func RenderGitPackageSection(
	focused bool,
	onGitItem bool,
	hasGitPackage bool,
	gitFieldCursor int,
	editingGitField bool,
	gitURLInput textinput.Model,
	gitBranchInput textinput.Model,
	gitLinuxInput textinput.Model,
	gitWindowsInput textinput.Model,
	gitSudo bool,
) string {
	var b strings.Builder
	prefix := tuishared.IndentSpaces

	if !hasGitPackage {
		// Collapsed: show add button
		addText := "[+ Add Git Package]"
		if focused && onGitItem {
			fmt.Fprintf(&b, "%s%s\n", prefix, tuishared.SelectedMenuItemStyle.Render(addText))
		} else {
			fmt.Fprintf(&b, "%s%s\n", prefix, tuishared.MutedTextStyle.Render(addText))
		}
		return b.String()
	}

	// Expanded: show git label and sub-fields
	gitLabel := "git:"
	if focused && onGitItem && gitFieldCursor == -1 {
		fmt.Fprintf(&b, "%s%s\n", prefix, tuishared.SelectedMenuItemStyle.Render(gitLabel))
	} else {
		fmt.Fprintf(&b, "%s%s\n", prefix, gitLabel)
	}

	// Sub-fields with deeper indent
	onSubFields := focused && onGitItem && gitFieldCursor >= 0

	b.WriteString(RenderGitField("URL:     ", gitURLInput, onSubFields, gitFieldCursor == tuishared.GitFieldURL, editingGitField && gitFieldCursor == tuishared.GitFieldURL))
	b.WriteString(RenderGitField("Branch:  ", gitBranchInput, onSubFields, gitFieldCursor == tuishared.GitFieldBranch, editingGitField && gitFieldCursor == tuishared.GitFieldBranch))
	b.WriteString(RenderGitField("Linux:   ", gitLinuxInput, onSubFields, gitFieldCursor == tuishared.GitFieldLinux, editingGitField && gitFieldCursor == tuishared.GitFieldLinux))
	b.WriteString(RenderGitField("Windows: ", gitWindowsInput, onSubFields, gitFieldCursor == tuishared.GitFieldWindows, editingGitField && gitFieldCursor == tuishared.GitFieldWindows))

	// Sudo toggle
	subPrefix := tuishared.IndentSpaces + "  "
	sudoText := tuishared.CheckboxUnchecked + " No"
	if gitSudo {
		sudoText = tuishared.CheckboxChecked + " Yes"
	}
	if onSubFields && gitFieldCursor == tuishared.GitFieldSudo {
		fmt.Fprintf(&b, "%sSudo:    %s\n", subPrefix, tuishared.SelectedMenuItemStyle.Render(sudoText))
	} else {
		fmt.Fprintf(&b, "%sSudo:    %s\n", subPrefix, sudoText)
	}

	return b.String()
}

// RenderGitField renders a single git text field with appropriate styling
func RenderGitField(label string, input textinput.Model, onSubFields, isCurrent, isEditing bool) string {
	prefix := tuishared.IndentSpaces + "  "

	if isEditing {
		return fmt.Sprintf("%s%s%s\n", prefix, label, input.View())
	}

	value := input.Value()
	if value == "" {
		value = tuishared.MutedTextStyle.Render("(not set)")
	}

	if onSubFields && isCurrent {
		return fmt.Sprintf("%s%s\n", prefix, tuishared.SelectedMenuItemStyle.Render(fmt.Sprintf("%s%s", label, value)))
	}

	return fmt.Sprintf("%s%s%s\n", prefix, label, value)
}

// RenderInstallerPackageSection renders the installer package section within Packages
// focused indicates if the packages section is currently focused
// onInstallerItem indicates if the packagesCursor is on the installer item
func RenderInstallerPackageSection(
	focused bool,
	onInstallerItem bool,
	hasInstallerPackage bool,
	installerFieldCursor int,
	editingInstallerField bool,
	installerLinuxInput textinput.Model,
	installerWindowsInput textinput.Model,
	installerBinaryInput textinput.Model,
) string {
	var b strings.Builder
	prefix := tuishared.IndentSpaces

	if !hasInstallerPackage {
		// Collapsed: show add button
		addText := "[+ Add Installer Package]"
		if focused && onInstallerItem {
			fmt.Fprintf(&b, "%s%s\n", prefix, tuishared.SelectedMenuItemStyle.Render(addText))
		} else {
			fmt.Fprintf(&b, "%s%s\n", prefix, tuishared.MutedTextStyle.Render(addText))
		}
		return b.String()
	}

	// Expanded: show installer label and sub-fields
	installerLabel := "installer:"
	if focused && onInstallerItem && installerFieldCursor == -1 {
		fmt.Fprintf(&b, "%s%s\n", prefix, tuishared.SelectedMenuItemStyle.Render(installerLabel))
	} else {
		fmt.Fprintf(&b, "%s%s\n", prefix, installerLabel)
	}

	// Sub-fields with deeper indent
	onSubFields := focused && onInstallerItem && installerFieldCursor >= 0

	b.WriteString(RenderGitField("Linux:   ", installerLinuxInput, onSubFields, installerFieldCursor == tuishared.InstallerFieldLinux, editingInstallerField && installerFieldCursor == tuishared.InstallerFieldLinux))
	b.WriteString(RenderGitField("Windows: ", installerWindowsInput, onSubFields, installerFieldCursor == tuishared.InstallerFieldWindows, editingInstallerField && installerFieldCursor == tuishared.InstallerFieldWindows))
	b.WriteString(RenderGitField("Binary:  ", installerBinaryInput, onSubFields, installerFieldCursor == tuishared.InstallerFieldBinary, editingInstallerField && installerFieldCursor == tuishared.InstallerFieldBinary))

	return b.String()
}

// RenderWhenField renders the when expression text field
func RenderWhenField(
	focused bool,
	editing bool,
	whenInput textinput.Model,
) string {
	prefix := tuishared.IndentSpaces

	if editing {
		return fmt.Sprintf("%s%s\n", prefix, whenInput.View())
	}

	value := whenInput.Value()
	if value == "" {
		if focused {
			return fmt.Sprintf("%s%s\n", prefix, tuishared.SelectedMenuItemStyle.Render("(no condition)"))
		}
		return fmt.Sprintf("%s%s\n", prefix, tuishared.MutedTextStyle.Render("(no condition)"))
	}

	if focused {
		return fmt.Sprintf("%s%s\n", prefix, tuishared.SelectedMenuItemStyle.Render(value))
	}

	return fmt.Sprintf("%s%s\n", prefix, value)
}

// BuildPackageSpec creates a config.EntryPackage from a managers map
func BuildPackageSpec(managers map[string]string) *config.EntryPackage {
	if len(managers) == 0 {
		return nil
	}

	managersTyped := make(map[string]config.ManagerValue, len(managers))
	for k, v := range managers {
		managersTyped[k] = config.ManagerValue{PackageName: v}
	}

	return &config.EntryPackage{
		Managers: managersTyped,
	}
}

// MergeGitPackage merges git package data into an existing EntryPackage
func MergeGitPackage(
	pkg *config.EntryPackage,
	hasGit bool,
	urlInput textinput.Model,
	branchInput textinput.Model,
	linuxInput textinput.Model,
	windowsInput textinput.Model,
	sudo bool,
) *config.EntryPackage {
	if !hasGit {
		return pkg
	}

	url := strings.TrimSpace(urlInput.Value())
	if url == "" {
		return pkg
	}

	if pkg == nil {
		pkg = &config.EntryPackage{
			Managers: make(map[string]config.ManagerValue),
		}
	}

	gitPkg := &config.GitPackage{
		URL:     url,
		Branch:  strings.TrimSpace(branchInput.Value()),
		Targets: make(map[string]string),
		Sudo:    sudo,
	}

	if linux := strings.TrimSpace(linuxInput.Value()); linux != "" {
		gitPkg.Targets[tuishared.OSLinux] = linux
	}

	if windows := strings.TrimSpace(windowsInput.Value()); windows != "" {
		gitPkg.Targets[tuishared.OSWindows] = windows
	}

	pkg.Managers[tuishared.TypeGit] = config.ManagerValue{Git: gitPkg}

	return pkg
}

// MergeInstallerPackage merges installer package data into an existing EntryPackage
func MergeInstallerPackage(
	pkg *config.EntryPackage,
	hasInstaller bool,
	linuxInput textinput.Model,
	windowsInput textinput.Model,
	binaryInput textinput.Model,
) *config.EntryPackage {
	if !hasInstaller {
		return pkg
	}

	linux := strings.TrimSpace(linuxInput.Value())
	windows := strings.TrimSpace(windowsInput.Value())

	if linux == "" && windows == "" {
		return pkg
	}

	if pkg == nil {
		pkg = &config.EntryPackage{
			Managers: make(map[string]config.ManagerValue),
		}
	}

	installerPkg := &config.InstallerPackage{
		Command: make(map[string]string),
		Binary:  strings.TrimSpace(binaryInput.Value()),
	}

	if linux != "" {
		installerPkg.Command[tuishared.OSLinux] = linux
	}

	if windows != "" {
		installerPkg.Command[tuishared.OSWindows] = windows
	}

	pkg.Managers[tuishared.TypeInstaller] = config.ManagerValue{Installer: installerPkg}

	return pkg
}

// BuildTargetsFromInputs creates Targets map from Linux and Windows text inputs
func BuildTargetsFromInputs(linuxInput, windowsInput textinput.Model) map[string]string {
	targets := make(map[string]string)
	if linux := strings.TrimSpace(linuxInput.Value()); linux != "" {
		targets["linux"] = linux
	}

	if windows := strings.TrimSpace(windowsInput.Value()); windows != "" {
		targets["windows"] = windows
	}

	return targets
}
