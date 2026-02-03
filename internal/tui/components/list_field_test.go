package components

import (
	"testing"
)

func TestListField_NewListField(t *testing.T) {
	items := []string{"item1", "item2", "item3"}
	lf := NewListField("Files", items)

	if lf.Label != "Files" {
		t.Errorf("Label = %s, want Files", lf.Label)
	}
	if len(lf.Items) != 3 {
		t.Errorf("len(Items) = %d, want 3", len(lf.Items))
	}
	if lf.GetCursor() != 0 {
		t.Errorf("GetCursor() = %d, want 0", lf.GetCursor())
	}
	if lf.focused {
		t.Error("New list should not be focused")
	}
	if lf.IsEditing() {
		t.Error("New list should not be editing")
	}
}

func TestListField_FocusBlur(t *testing.T) {
	lf := NewListField("Files", []string{"item1"})

	// Test Focus
	lf.Focus()
	if !lf.IsFocused() {
		t.Error("IsFocused() = false after Focus(), want true")
	}

	// Test Blur
	lf.Blur()
	if lf.IsFocused() {
		t.Error("IsFocused() = true after Blur(), want false")
	}
	if lf.IsEditing() {
		t.Error("IsEditing() = true after Blur(), want false")
	}
}

func TestListField_CursorMovement(t *testing.T) {
	items := []string{"item1", "item2", "item3"}
	lf := NewListField("Files", items)

	// Test CursorDown
	if !lf.CursorDown() {
		t.Error("CursorDown should return true when not at bottom")
	}
	if lf.GetCursor() != 1 {
		t.Errorf("GetCursor() = %d after CursorDown, want 1", lf.GetCursor())
	}

	// Test CursorUp
	if !lf.CursorUp() {
		t.Error("CursorUp should return true when not at top")
	}
	if lf.GetCursor() != 0 {
		t.Errorf("GetCursor() = %d after CursorUp, want 0", lf.GetCursor())
	}

	// Test CursorUp at boundary
	if lf.CursorUp() {
		t.Error("CursorUp should return false at top boundary")
	}

	// Move to bottom (including Add button)
	lf.SetCursor(3) // On "Add" button
	if !lf.IsAtBottom() {
		t.Error("IsAtBottom() should be true when cursor is at Add button")
	}
	if lf.CursorDown() {
		t.Error("CursorDown should return false at bottom boundary")
	}
}

func TestListField_EditExistingItem(t *testing.T) {
	items := []string{"item1", "item2", "item3"}
	lf := NewListField("Files", items)

	// Edit first item
	lf.SetCursor(0)
	lf.EnterEditMode()

	if !lf.IsEditing() {
		t.Error("IsEditing() should be true after EnterEditMode")
	}
	if lf.GetEditingIndex() != 0 {
		t.Errorf("GetEditingIndex() = %d, want 0", lf.GetEditingIndex())
	}

	// Modify value
	editField := lf.GetEditingText()
	if editField == nil {
		t.Fatal("GetEditingText() should not be nil when editing")
	}
	editField.SetValue("modified")

	// Save edit
	lf.ExitEditMode()
	if lf.IsEditing() {
		t.Error("IsEditing() should be false after ExitEditMode")
	}
	if lf.Items[0] != "modified" {
		t.Errorf("Items[0] = %s after edit, want modified", lf.Items[0])
	}
}

func TestListField_AddNewItem(t *testing.T) {
	items := []string{"item1", "item2"}
	lf := NewListField("Files", items)

	// Move to "Add" button
	lf.SetCursor(len(items))

	// Enter edit mode to add new item
	lf.EnterEditMode()

	if !lf.IsEditing() {
		t.Error("IsEditing() should be true after EnterEditMode on Add button")
	}
	if len(lf.Items) != 3 {
		t.Errorf("len(Items) = %d after EnterEditMode, want 3", len(lf.Items))
	}

	// Set value for new item
	editField := lf.GetEditingText()
	if editField == nil {
		t.Fatal("GetEditingText() should not be nil when adding")
	}
	editField.SetValue("item3")

	// Save new item
	lf.ExitEditMode()
	if len(lf.Items) != 3 {
		t.Errorf("len(Items) = %d after ExitEditMode, want 3", len(lf.Items))
	}
	if lf.Items[2] != "item3" {
		t.Errorf("Items[2] = %s, want item3", lf.Items[2])
	}
}

func TestListField_CancelAddNewItem(t *testing.T) {
	items := []string{"item1", "item2"}
	lf := NewListField("Files", items)

	// Move to "Add" button and start adding
	lf.SetCursor(len(items))
	lf.EnterEditMode()

	if len(lf.Items) != 3 {
		t.Errorf("len(Items) = %d after EnterEditMode, want 3", len(lf.Items))
	}

	// Cancel without setting value
	lf.CancelEdit()

	if lf.IsEditing() {
		t.Error("IsEditing() should be false after CancelEdit")
	}
	if len(lf.Items) != 2 {
		t.Errorf("len(Items) = %d after CancelEdit, want 2 (empty item removed)", len(lf.Items))
	}
}

func TestListField_DeleteItem(t *testing.T) {
	items := []string{"item1", "item2", "item3"}
	lf := NewListField("Files", items)

	// Delete second item
	lf.SetCursor(1)
	lf.DeleteCurrent()

	if len(lf.Items) != 2 {
		t.Errorf("len(Items) = %d after DeleteCurrent, want 2", len(lf.Items))
	}
	if lf.Items[0] != "item1" || lf.Items[1] != "item3" {
		t.Errorf("Items = %v after delete, want [item1, item3]", lf.Items)
	}
}

func TestListField_DeleteLastItem(t *testing.T) {
	items := []string{"item1", "item2", "item3"}
	lf := NewListField("Files", items)

	// Delete last item
	lf.SetCursor(2)
	lf.DeleteCurrent()

	if len(lf.Items) != 2 {
		t.Errorf("len(Items) = %d after DeleteCurrent, want 2", len(lf.Items))
	}
	// Cursor should move up when deleting last item
	if lf.GetCursor() != 1 {
		t.Errorf("GetCursor() = %d after deleting last item, want 1", lf.GetCursor())
	}
}

func TestListField_BoundaryChecks(t *testing.T) {
	items := []string{"item1", "item2"}
	lf := NewListField("Files", items)

	// Test IsAtTop
	if !lf.IsAtTop() {
		t.Error("IsAtTop() should be true at cursor 0")
	}

	lf.SetCursor(1)
	if lf.IsAtTop() {
		t.Error("IsAtTop() should be false when not at top")
	}

	// Test IsAtBottom (Add button is at index 2)
	lf.SetCursor(2)
	if !lf.IsAtBottom() {
		t.Error("IsAtBottom() should be true at Add button")
	}

	lf.SetCursor(1)
	if lf.IsAtBottom() {
		t.Error("IsAtBottom() should be false when not at bottom")
	}
}

func TestListField_EmptyList(t *testing.T) {
	lf := NewListField("Files", []string{})

	if !lf.IsAtTop() {
		t.Error("Empty list should be at top")
	}
	if !lf.IsAtBottom() {
		t.Error("Empty list should be at bottom (on Add button)")
	}

	// Should be able to add to empty list
	lf.EnterEditMode()
	if !lf.IsEditing() {
		t.Error("Should be able to enter edit mode on empty list")
	}

	editField := lf.GetEditingText()
	editField.SetValue("first item")
	lf.ExitEditMode()

	if len(lf.Items) != 1 {
		t.Errorf("len(Items) = %d after adding to empty list, want 1", len(lf.Items))
	}
	if lf.Items[0] != "first item" {
		t.Errorf("Items[0] = %s, want 'first item'", lf.Items[0])
	}
}
