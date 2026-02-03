package components

import (
	tea "github.com/charmbracelet/bubbletea"
)

// ListField manages a list of items with add/edit/delete functionality
type ListField struct {
	Label       string
	Items       []string
	cursor      int
	focused     bool
	editingIdx  int
	editingText TextField
}

// NewListField creates a new ListField
func NewListField(label string, items []string) ListField {
	return ListField{
		Label:      label,
		Items:      items,
		cursor:     0,
		focused:    false,
		editingIdx: -1,
	}
}

// Focus sets the list as focused
func (l *ListField) Focus() {
	l.focused = true
}

// Blur removes focus
func (l *ListField) Blur() {
	l.focused = false
	l.editingIdx = -1
}

// IsFocused returns whether focused
func (l *ListField) IsFocused() bool {
	return l.focused
}

// IsEditing returns whether editing an item
func (l *ListField) IsEditing() bool {
	return l.editingIdx >= 0
}

// GetCursor returns the current cursor position
func (l *ListField) GetCursor() int {
	return l.cursor
}

// SetCursor sets the cursor position
func (l *ListField) SetCursor(pos int) {
	l.cursor = pos
}

// CursorUp moves cursor up, returns true if moved, false if at boundary
func (l *ListField) CursorUp() bool {
	if l.cursor > 0 {
		l.cursor--
		return true
	}
	return false
}

// CursorDown moves cursor down, returns true if moved, false if at boundary
func (l *ListField) CursorDown() bool {
	maxIdx := len(l.Items) // Include "Add" button
	if l.cursor < maxIdx {
		l.cursor++
		return true
	}
	return false
}

// IsAtTop returns true if cursor is at the top
func (l *ListField) IsAtTop() bool {
	return l.cursor == 0
}

// IsAtBottom returns true if cursor is at or past the bottom (on "Add" button)
func (l *ListField) IsAtBottom() bool {
	return l.cursor >= len(l.Items)
}

// EnterEditMode starts editing current item or adds new item if on "Add" button
func (l *ListField) EnterEditMode() {
	if l.cursor < len(l.Items) {
		// Edit existing item
		l.editingIdx = l.cursor
		l.editingText = NewTextField("", "Enter value", l.Items[l.cursor])
		l.editingText.EnterEditMode()
	} else {
		// Adding new item
		l.editingIdx = len(l.Items)
		l.Items = append(l.Items, "")
		l.editingText = NewTextField("", "Enter value", "")
		l.editingText.EnterEditMode()
	}
}

// ExitEditMode saves the edit
func (l *ListField) ExitEditMode() {
	if l.editingIdx >= 0 && l.editingIdx < len(l.Items) {
		value := l.editingText.Value()
		if value == "" && l.editingIdx == len(l.Items)-1 {
			// Remove empty item that was just added
			l.Items = l.Items[:len(l.Items)-1]
		} else {
			l.Items[l.editingIdx] = value
		}
	}
	l.editingIdx = -1
}

// CancelEdit cancels the edit without saving
func (l *ListField) CancelEdit() {
	if l.editingIdx >= 0 {
		// If we were adding a new item, remove it
		if l.editingIdx == len(l.Items)-1 && l.Items[l.editingIdx] == "" {
			l.Items = l.Items[:len(l.Items)-1]
		}
	}
	l.editingIdx = -1
}

// DeleteCurrent removes the current item
func (l *ListField) DeleteCurrent() {
	if l.cursor < len(l.Items) {
		l.Items = append(l.Items[:l.cursor], l.Items[l.cursor+1:]...)
		if l.cursor >= len(l.Items) && l.cursor > 0 {
			l.cursor--
		}
	}
}

// Update handles messages
func (l *ListField) Update(msg tea.Msg) tea.Cmd {
	if l.IsEditing() {
		return l.editingText.Update(msg)
	}
	return nil
}

// GetEditingText returns the TextField being used for editing (for rendering)
func (l *ListField) GetEditingText() *TextField {
	if l.IsEditing() {
		return &l.editingText
	}
	return nil
}

// GetEditingIndex returns the index being edited, or -1 if not editing
func (l *ListField) GetEditingIndex() int {
	return l.editingIdx
}
