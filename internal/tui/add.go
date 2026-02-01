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

	targetInput := textinput.New()
	targetInput.Placeholder = "e.g., ~/.config/nvim"
	targetInput.CharLimit = 256
	targetInput.Width = 40

	backupInput := textinput.New()
	backupInput.Placeholder = "e.g., ./nvim"
	backupInput.CharLimit = 256
	backupInput.Width = 40

	m.addForm = AddForm{
		nameInput:   nameInput,
		targetInput: targetInput,
		backupInput: backupInput,
		isFolder:    true,
		focusIndex:  0,
		err:         "",
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
		if m.addForm.focusIndex > 3 {
			m.addForm.focusIndex = 0
		}
		m.updateAddFormFocus()
		return m, nil

	case "shift+tab", "up":
		// Move to previous field
		m.addForm.focusIndex--
		if m.addForm.focusIndex < 0 {
			m.addForm.focusIndex = 3
		}
		m.updateAddFormFocus()
		return m, nil

	case " ":
		// Toggle isFolder if on the toggle field
		if m.addForm.focusIndex == 3 {
			m.addForm.isFolder = !m.addForm.isFolder
			return m, nil
		}

	case "enter":
		// If on toggle field, toggle it
		if m.addForm.focusIndex == 3 {
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
		m.addForm.targetInput, cmd = m.addForm.targetInput.Update(msg)
	case 2:
		m.addForm.backupInput, cmd = m.addForm.backupInput.Update(msg)
	}

	// Clear error when typing
	m.addForm.err = ""

	return m, cmd
}

// updateAddFormFocus updates which input field is focused
func (m *Model) updateAddFormFocus() {
	m.addForm.nameInput.Blur()
	m.addForm.targetInput.Blur()
	m.addForm.backupInput.Blur()

	switch m.addForm.focusIndex {
	case 0:
		m.addForm.nameInput.Focus()
	case 1:
		m.addForm.targetInput.Focus()
	case 2:
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

	// Target field
	targetLabel := fmt.Sprintf("Target (%s):", m.Platform.OS)
	if m.addForm.focusIndex == 1 {
		targetLabel = HelpKeyStyle.Render(targetLabel)
	}
	b.WriteString(fmt.Sprintf("  %s\n", targetLabel))
	b.WriteString(fmt.Sprintf("  %s\n\n", m.addForm.targetInput.View()))

	// Backup field
	backupLabel := "Backup path:"
	if m.addForm.focusIndex == 2 {
		backupLabel = HelpKeyStyle.Render("Backup path:")
	}
	b.WriteString(fmt.Sprintf("  %s\n", backupLabel))
	b.WriteString(fmt.Sprintf("  %s\n\n", m.addForm.backupInput.View()))

	// Is folder toggle
	toggleLabel := "Type:"
	if m.addForm.focusIndex == 3 {
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
	target := strings.TrimSpace(m.addForm.targetInput.Value())
	backup := strings.TrimSpace(m.addForm.backupInput.Value())

	// Validate required fields
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if target == "" {
		return fmt.Errorf("target path is required")
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

	// Create new PathSpec
	newPath := config.PathSpec{
		Name:   name,
		Backup: backup,
		Targets: map[string]string{
			m.Platform.OS: target,
		},
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

	// Update the Paths slice in the model
	m.Paths = append(m.Paths, PathItem{
		Spec:     newPath,
		Target:   target,
		Selected: true,
	})

	// Refresh path states
	m.refreshPathStates()

	return nil
}
