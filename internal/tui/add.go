package tui

import (
	"fmt"
	"strings"

	"github.com/AntoineGS/dot-manager/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// initAddForm initializes the add form with empty inputs
func (m *Model) initAddForm() {
	nameInput := textinput.New()
	nameInput.Placeholder = "e.g., neovim"
	nameInput.Focus()
	nameInput.CharLimit = 64
	nameInput.Width = 40

	linuxTargetInput := textinput.New()
	linuxTargetInput.Placeholder = "e.g., ~/.config/nvim"
	linuxTargetInput.CharLimit = 256
	linuxTargetInput.Width = 40

	windowsTargetInput := textinput.New()
	windowsTargetInput.Placeholder = "e.g., ~/AppData/Local/nvim"
	windowsTargetInput.CharLimit = 256
	windowsTargetInput.Width = 40

	backupInput := textinput.New()
	backupInput.Placeholder = "e.g., ./nvim"
	backupInput.CharLimit = 256
	backupInput.Width = 40

	m.addForm = AddForm{
		nameInput:          nameInput,
		linuxTargetInput:   linuxTargetInput,
		windowsTargetInput: windowsTargetInput,
		backupInput:        backupInput,
		isFolder:           true,
		focusIndex:         0,
		err:                "",
	}
}

// updateAddForm handles key events for the add form
func (m Model) updateAddForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.Screen = ScreenMenu
		return m, nil

	case "tab", "down":
		// Move to next field
		m.addForm.focusIndex++
		if m.addForm.focusIndex > 4 {
			m.addForm.focusIndex = 0
		}
		m.updateAddFormFocus()
		return m, nil

	case "shift+tab", "up":
		// Move to previous field
		m.addForm.focusIndex--
		if m.addForm.focusIndex < 0 {
			m.addForm.focusIndex = 4
		}
		m.updateAddFormFocus()
		return m, nil

	case " ":
		// Toggle isFolder if on the toggle field
		if m.addForm.focusIndex == 4 {
			m.addForm.isFolder = !m.addForm.isFolder
			return m, nil
		}

	case "enter":
		// If on toggle field, toggle it
		if m.addForm.focusIndex == 4 {
			m.addForm.isFolder = !m.addForm.isFolder
			return m, nil
		}
		// Otherwise try to save
		if err := m.saveNewPath(); err != nil {
			m.addForm.err = err.Error()
			return m, nil
		}
		// Success - go back to menu
		m.Screen = ScreenMenu
		return m, nil
	}

	// Handle text input for the focused field
	switch m.addForm.focusIndex {
	case 0:
		m.addForm.nameInput, cmd = m.addForm.nameInput.Update(msg)
	case 1:
		m.addForm.linuxTargetInput, cmd = m.addForm.linuxTargetInput.Update(msg)
	case 2:
		m.addForm.windowsTargetInput, cmd = m.addForm.windowsTargetInput.Update(msg)
	case 3:
		m.addForm.backupInput, cmd = m.addForm.backupInput.Update(msg)
	}

	// Clear error when typing
	m.addForm.err = ""

	return m, cmd
}

// updateAddFormFocus updates which input field is focused
func (m *Model) updateAddFormFocus() {
	m.addForm.nameInput.Blur()
	m.addForm.linuxTargetInput.Blur()
	m.addForm.windowsTargetInput.Blur()
	m.addForm.backupInput.Blur()

	switch m.addForm.focusIndex {
	case 0:
		m.addForm.nameInput.Focus()
	case 1:
		m.addForm.linuxTargetInput.Focus()
	case 2:
		m.addForm.windowsTargetInput.Focus()
	case 3:
		m.addForm.backupInput.Focus()
	}
}

// viewAddForm renders the add form
func (m Model) viewAddForm() string {
	var b strings.Builder

	// Title
	title := TitleStyle.Render("󰐕  Add Path Configuration")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Instructions
	b.WriteString(SubtitleStyle.Render("Add a new path to your dotfiles configuration"))
	b.WriteString("\n\n")

	// Name field
	nameLabel := "Name:"
	if m.addForm.focusIndex == 0 {
		nameLabel = HelpKeyStyle.Render("Name:")
	}
	b.WriteString(fmt.Sprintf("  %s\n", nameLabel))
	b.WriteString(fmt.Sprintf("  %s\n\n", m.addForm.nameInput.View()))

	// Linux target field
	linuxTargetLabel := "Target (linux):"
	if m.addForm.focusIndex == 1 {
		linuxTargetLabel = HelpKeyStyle.Render(linuxTargetLabel)
	}
	b.WriteString(fmt.Sprintf("  %s\n", linuxTargetLabel))
	b.WriteString(fmt.Sprintf("  %s\n\n", m.addForm.linuxTargetInput.View()))

	// Windows target field
	windowsTargetLabel := "Target (windows):"
	if m.addForm.focusIndex == 2 {
		windowsTargetLabel = HelpKeyStyle.Render(windowsTargetLabel)
	}
	b.WriteString(fmt.Sprintf("  %s\n", windowsTargetLabel))
	b.WriteString(fmt.Sprintf("  %s\n\n", m.addForm.windowsTargetInput.View()))

	// Backup field
	backupLabel := "Backup path:"
	if m.addForm.focusIndex == 3 {
		backupLabel = HelpKeyStyle.Render("Backup path:")
	}
	b.WriteString(fmt.Sprintf("  %s\n", backupLabel))
	b.WriteString(fmt.Sprintf("  %s\n\n", m.addForm.backupInput.View()))

	// Is folder toggle
	toggleLabel := "Type:"
	if m.addForm.focusIndex == 4 {
		toggleLabel = HelpKeyStyle.Render("Type:")
	}
	folderCheck := "[ ]"
	filesCheck := "[✓]"
	if m.addForm.isFolder {
		folderCheck = "[✓]"
		filesCheck = "[ ]"
	}
	b.WriteString(fmt.Sprintf("  %s  %s Folder  %s Files\n", toggleLabel, folderCheck, filesCheck))
	b.WriteString("\n")

	// Error message
	if m.addForm.err != "" {
		b.WriteString(ErrorStyle.Render("  Error: " + m.addForm.err))
		b.WriteString("\n\n")
	}

	// Help
	b.WriteString(RenderHelp(
		"tab/↓", "next field",
		"shift+tab/↑", "prev field",
		"enter", "save",
		"esc", "cancel",
	))

	return BaseStyle.Render(b.String())
}

// saveNewPath validates the form and saves the new path to the config
func (m *Model) saveNewPath() error {
	name := strings.TrimSpace(m.addForm.nameInput.Value())
	linuxTarget := strings.TrimSpace(m.addForm.linuxTargetInput.Value())
	windowsTarget := strings.TrimSpace(m.addForm.windowsTargetInput.Value())
	backup := strings.TrimSpace(m.addForm.backupInput.Value())

	// Validate required fields
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if linuxTarget == "" && windowsTarget == "" {
		return fmt.Errorf("at least one target path is required")
	}
	if backup == "" {
		return fmt.Errorf("backup path is required")
	}

	// Check for duplicate names
	for _, p := range m.Config.Paths {
		if p.Name == name {
			return fmt.Errorf("a path with name '%s' already exists", name)
		}
	}

	// Create new PathSpec with targets
	targets := make(map[string]string)
	if linuxTarget != "" {
		targets["linux"] = linuxTarget
	}
	if windowsTarget != "" {
		targets["windows"] = windowsTarget
	}

	newPath := config.PathSpec{
		Name:    name,
		Backup:  backup,
		Targets: targets,
	}

	// For files mode, we'd need file names - for now only support folder mode
	if !m.addForm.isFolder {
		return fmt.Errorf("file mode not yet supported in TUI - use folder mode or edit config directly")
	}

	// Add to config
	m.Config.Paths = append(m.Config.Paths, newPath)

	// Save config to file
	if err := config.Save(m.Config, m.ConfigPath); err != nil {
		// Remove the path we just added since save failed
		m.Config.Paths = m.Config.Paths[:len(m.Config.Paths)-1]
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Update the Paths slice in the model (only if current platform has a target)
	currentTarget := newPath.GetTarget(m.Platform.OS)
	if currentTarget != "" {
		m.Paths = append(m.Paths, PathItem{
			Spec:     newPath,
			Target:   currentTarget,
			Selected: true,
		})

		// Refresh path states
		m.refreshPathStates()
	}

	return nil
}
