package components

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// TextField is a reusable text input component with label and two-phase editing
type TextField struct {
	Label       string
	Placeholder string
	input       textinput.Model
	focused     bool
	editing     bool
}

// NewTextField creates a new TextField
func NewTextField(label, placeholder, value string) TextField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.SetValue(value)
	ti.CharLimit = 256
	ti.Width = 40

	return TextField{
		Label:       label,
		Placeholder: placeholder,
		input:       ti,
		focused:     false,
		editing:     false,
	}
}

// NewTextFieldWithLimits creates a new TextField with custom char limit and width
func NewTextFieldWithLimits(label, placeholder, value string, charLimit, width int) TextField {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.SetValue(value)
	ti.CharLimit = charLimit
	ti.Width = width

	return TextField{
		Label:       label,
		Placeholder: placeholder,
		input:       ti,
		focused:     false,
		editing:     false,
	}
}

// Focus sets the field as focused (highlighted but not editing)
func (t *TextField) Focus() {
	t.focused = true
	t.editing = false
}

// Blur removes focus from the field
func (t *TextField) Blur() {
	t.focused = false
	t.editing = false
	t.input.Blur()
}

// IsFocused returns whether the field is focused
func (t *TextField) IsFocused() bool {
	return t.focused
}

// IsEditing returns whether the field is in edit mode
func (t *TextField) IsEditing() bool {
	return t.editing
}

// EnterEditMode starts editing the field
func (t *TextField) EnterEditMode() {
	t.editing = true
	t.input.Focus()
	t.input.SetCursor(len(t.input.Value()))
}

// ExitEditMode stops editing and saves the value
func (t *TextField) ExitEditMode() {
	t.editing = false
	t.input.Blur()
}

// Value returns the current field value
func (t *TextField) Value() string {
	return t.input.Value()
}

// SetValue sets the field value
func (t *TextField) SetValue(value string) {
	t.input.SetValue(value)
}

// Update handles bubble tea messages
func (t *TextField) Update(msg tea.Msg) tea.Cmd {
	if !t.editing {
		return nil
	}

	var cmd tea.Cmd
	t.input, cmd = t.input.Update(msg)
	return cmd
}

// GetInput returns the underlying textinput.Model for direct access
func (t *TextField) GetInput() textinput.Model {
	return t.input
}

// SetCursor sets the cursor position
func (t *TextField) SetCursor(pos int) {
	t.input.SetCursor(pos)
}
