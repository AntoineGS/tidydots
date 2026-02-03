package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTextField_NewTextField(t *testing.T) {
	tf := NewTextField("Name", "Enter name", "test value")

	if tf.Label != "Name" {
		t.Errorf("Label = %s, want Name", tf.Label)
	}
	if tf.Placeholder != "Enter name" {
		t.Errorf("Placeholder = %s, want Enter name", tf.Placeholder)
	}
	if tf.Value() != "test value" {
		t.Errorf("Value() = %s, want test value", tf.Value())
	}
	if tf.focused {
		t.Error("New field should not be focused")
	}
	if tf.editing {
		t.Error("New field should not be editing")
	}
}

func TestTextField_FocusBlur(t *testing.T) {
	tf := NewTextField("Name", "Enter name", "")

	// Test Focus
	tf.Focus()
	if !tf.IsFocused() {
		t.Error("IsFocused() = false after Focus(), want true")
	}
	if tf.IsEditing() {
		t.Error("IsEditing() = true after Focus(), want false")
	}

	// Test Blur
	tf.Blur()
	if tf.IsFocused() {
		t.Error("IsFocused() = true after Blur(), want false")
	}
	if tf.IsEditing() {
		t.Error("IsEditing() = true after Blur(), want false")
	}
}

func TestTextField_EditMode(t *testing.T) {
	tf := NewTextField("Name", "Enter name", "initial")

	// Enter edit mode
	tf.Focus()
	tf.EnterEditMode()

	if !tf.IsEditing() {
		t.Error("IsEditing() = false after EnterEditMode(), want true")
	}

	// Exit edit mode
	tf.ExitEditMode()
	if tf.IsEditing() {
		t.Error("IsEditing() = true after ExitEditMode(), want false")
	}
}

func TestTextField_ValueManipulation(t *testing.T) {
	tf := NewTextField("Name", "Enter name", "initial")

	// Test initial value
	if tf.Value() != "initial" {
		t.Errorf("Value() = %s, want initial", tf.Value())
	}

	// Test SetValue
	tf.SetValue("updated")
	if tf.Value() != "updated" {
		t.Errorf("Value() = %s after SetValue, want updated", tf.Value())
	}
}

func TestTextField_Update(t *testing.T) {
	tf := NewTextField("Name", "Enter name", "")

	// Update should do nothing when not editing
	cmd := tf.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if cmd != nil {
		t.Error("Update should return nil when not editing")
	}

	// Enter edit mode
	tf.EnterEditMode()

	// Update should work when editing
	cmd = tf.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	// We don't check the cmd here as it depends on textinput implementation
	_ = cmd
}

func TestTextField_TwoPhaseEditing(t *testing.T) {
	tf := NewTextField("Name", "Enter name", "test")

	// Phase 1: Navigation mode (focused but not editing)
	tf.Focus()
	if !tf.IsFocused() {
		t.Error("Should be focused in navigation mode")
	}
	if tf.IsEditing() {
		t.Error("Should not be editing in navigation mode")
	}

	// Phase 2: Edit mode
	tf.EnterEditMode()
	if !tf.IsFocused() {
		t.Error("Should still be focused in edit mode")
	}
	if !tf.IsEditing() {
		t.Error("Should be editing after EnterEditMode")
	}

	// Exit edit mode returns to navigation mode
	tf.ExitEditMode()
	if !tf.IsFocused() {
		t.Error("Should still be focused after ExitEditMode")
	}
	if tf.IsEditing() {
		t.Error("Should not be editing after ExitEditMode")
	}

	// Blur exits both modes
	tf.Blur()
	if tf.IsFocused() {
		t.Error("Should not be focused after Blur")
	}
	if tf.IsEditing() {
		t.Error("Should not be editing after Blur")
	}
}
