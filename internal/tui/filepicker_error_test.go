package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// TestInitFilePickerEmptyTarget tests that empty target path falls back to home directory
func TestInitFilePickerEmptyTarget(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	// Create a model with empty target
	m := &Model{
		Platform: &platform.Platform{OS: OSLinux},
		subEntryForm: &SubEntryForm{
			LinuxTargetInput:   textinput.New(),
			WindowsTargetInput: textinput.New(),
		},
	}

	// Empty linux target
	m.subEntryForm.LinuxTargetInput.SetValue("")

	// Initialize file picker
	err = m.initFilePicker()
	if err != nil {
		t.Fatalf("initFilePicker() failed: %v", err)
	}

	// Verify picker starts at home directory
	if m.subEntryForm.FilePicker.CurrentDirectory != home {
		t.Errorf("initFilePicker() with empty target = %v, want %v", m.subEntryForm.FilePicker.CurrentDirectory, home)
	}
}

// TestInitFilePickerNonExistentTarget tests that non-existent target falls back to parent
func TestInitFilePickerNonExistentTarget(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	existingDir := filepath.Join(tmpDir, "existing")
	err := os.Mkdir(existingDir, 0o750)
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Non-existent nested path
	nonExistentPath := filepath.Join(existingDir, "does-not-exist", "deeply", "nested")

	// Create a model with non-existent target
	m := &Model{
		Platform: &platform.Platform{OS: OSLinux},
		subEntryForm: &SubEntryForm{
			LinuxTargetInput:   textinput.New(),
			WindowsTargetInput: textinput.New(),
		},
	}

	m.subEntryForm.LinuxTargetInput.SetValue(nonExistentPath)

	// Initialize file picker
	err = m.initFilePicker()
	if err != nil {
		t.Fatalf("initFilePicker() failed: %v", err)
	}

	// Verify picker falls back to existing parent
	if m.subEntryForm.FilePicker.CurrentDirectory != existingDir {
		t.Errorf("initFilePicker() with non-existent target = %v, want %v", m.subEntryForm.FilePicker.CurrentDirectory, existingDir)
	}
}

// TestUpdateSubEntryFilePickerOutsideTarget tests that selecting files outside target shows error
func TestUpdateSubEntryFilePickerOutsideTarget(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	err := os.Mkdir(targetDir, 0o750)
	if err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

	outsideDir := filepath.Join(tmpDir, "outside")
	err = os.Mkdir(outsideDir, 0o750)
	if err != nil {
		t.Fatalf("Failed to create outside directory: %v", err)
	}

	outsideFile := filepath.Join(outsideDir, "file.txt")
	err = os.WriteFile(outsideFile, []byte("test"), 0o600)
	if err != nil {
		t.Fatalf("Failed to create outside file: %v", err)
	}

	// Create a model with target and selected file outside target
	m := &Model{
		Platform: &platform.Platform{OS: OSLinux},
		subEntryForm: &SubEntryForm{
			LinuxTargetInput:   textinput.New(),
			WindowsTargetInput: textinput.New(),
			SelectedFiles:      make(map[string]bool),
			Files:              []string{},
		},
	}

	m.subEntryForm.LinuxTargetInput.SetValue(targetDir)
	m.subEntryForm.SelectedFiles[outsideFile] = true
	m.subEntryForm.AddFileMode = ModePicker

	// Simulate enter key to confirm selection
	msg := mockKeyMsg("enter")
	updatedModel, _ := m.updateSubEntryFilePicker(msg)
	updatedM := updatedModel.(Model)

	// Verify that no files were added (since they're all outside target)
	if len(updatedM.subEntryForm.Files) > 0 {
		t.Errorf("updateSubEntryFilePicker() added files outside target: %v", updatedM.subEntryForm.Files)
	}

	// Verify mode was reset
	if updatedM.subEntryForm.AddFileMode != ModeNone {
		t.Errorf("updateSubEntryFilePicker() mode = %v, want ModeNone", updatedM.subEntryForm.AddFileMode)
	}
}

// TestUpdateSubEntryFilePickerInsideTarget tests that selecting files inside target succeeds
func TestUpdateSubEntryFilePickerInsideTarget(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	err := os.Mkdir(targetDir, 0o750)
	if err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

	insideFile := filepath.Join(targetDir, "file.txt")
	err = os.WriteFile(insideFile, []byte("test"), 0o600)
	if err != nil {
		t.Fatalf("Failed to create inside file: %v", err)
	}

	// Create a model with target and selected file inside target
	m := &Model{
		Platform: &platform.Platform{OS: OSLinux},
		subEntryForm: &SubEntryForm{
			LinuxTargetInput:   textinput.New(),
			WindowsTargetInput: textinput.New(),
			SelectedFiles:      make(map[string]bool),
			Files:              []string{},
		},
	}

	m.subEntryForm.LinuxTargetInput.SetValue(targetDir)
	m.subEntryForm.SelectedFiles[insideFile] = true
	m.subEntryForm.AddFileMode = ModePicker

	// Simulate enter key to confirm selection
	msg := mockKeyMsg("enter")
	updatedModel, _ := m.updateSubEntryFilePicker(msg)
	updatedM := updatedModel.(Model)

	// Verify that file was added with relative path
	if len(updatedM.subEntryForm.Files) != 1 {
		t.Fatalf("updateSubEntryFilePicker() added %d files, want 1", len(updatedM.subEntryForm.Files))
	}

	if updatedM.subEntryForm.Files[0] != "file.txt" {
		t.Errorf("updateSubEntryFilePicker() files[0] = %v, want file.txt", updatedM.subEntryForm.Files[0])
	}

	// Verify mode was reset
	if updatedM.subEntryForm.AddFileMode != ModeNone {
		t.Errorf("updateSubEntryFilePicker() mode = %v, want ModeNone", updatedM.subEntryForm.AddFileMode)
	}

	// Verify selections were cleared
	if len(updatedM.subEntryForm.SelectedFiles) != 0 {
		t.Errorf("updateSubEntryFilePicker() selectedFiles not cleared: %v", updatedM.subEntryForm.SelectedFiles)
	}
}

// TestUpdateSubEntryFilePickerMixedSelection tests mixed inside/outside target files
func TestUpdateSubEntryFilePickerMixedSelection(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	err := os.Mkdir(targetDir, 0o750)
	if err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

	outsideDir := filepath.Join(tmpDir, "outside")
	err = os.Mkdir(outsideDir, 0o750)
	if err != nil {
		t.Fatalf("Failed to create outside directory: %v", err)
	}

	insideFile1 := filepath.Join(targetDir, "inside1.txt")
	err = os.WriteFile(insideFile1, []byte("test"), 0o600)
	if err != nil {
		t.Fatalf("Failed to create inside file 1: %v", err)
	}

	insideFile2 := filepath.Join(targetDir, "inside2.txt")
	err = os.WriteFile(insideFile2, []byte("test"), 0o600)
	if err != nil {
		t.Fatalf("Failed to create inside file 2: %v", err)
	}

	outsideFile := filepath.Join(outsideDir, "outside.txt")
	err = os.WriteFile(outsideFile, []byte("test"), 0o600)
	if err != nil {
		t.Fatalf("Failed to create outside file: %v", err)
	}

	// Create a model with target and mixed selections
	m := &Model{
		Platform: &platform.Platform{OS: OSLinux},
		subEntryForm: &SubEntryForm{
			LinuxTargetInput:   textinput.New(),
			WindowsTargetInput: textinput.New(),
			SelectedFiles:      make(map[string]bool),
			Files:              []string{},
		},
	}

	m.subEntryForm.LinuxTargetInput.SetValue(targetDir)
	m.subEntryForm.SelectedFiles[insideFile1] = true
	m.subEntryForm.SelectedFiles[outsideFile] = true
	m.subEntryForm.SelectedFiles[insideFile2] = true
	m.subEntryForm.AddFileMode = ModePicker

	// Simulate enter key to confirm selection
	msg := mockKeyMsg("enter")
	updatedModel, _ := m.updateSubEntryFilePicker(msg)
	updatedM := updatedModel.(Model)

	// Verify that only inside files were added
	if len(updatedM.subEntryForm.Files) != 2 {
		t.Fatalf("updateSubEntryFilePicker() added %d files, want 2", len(updatedM.subEntryForm.Files))
	}

	// Check that files contain inside1 and inside2 (order doesn't matter due to map iteration)
	fileMap := make(map[string]bool)
	for _, f := range updatedM.subEntryForm.Files {
		fileMap[f] = true
	}

	if !fileMap["inside1.txt"] || !fileMap["inside2.txt"] {
		t.Errorf("updateSubEntryFilePicker() files = %v, want inside1.txt and inside2.txt", updatedM.subEntryForm.Files)
	}

	// Verify outside file was not added
	if fileMap["outside.txt"] {
		t.Errorf("updateSubEntryFilePicker() added outside.txt which should have been rejected")
	}
}

// TestInitFilePickerErrorHandling tests error cases in initFilePicker
func TestInitFilePickerErrorHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setupModel  func() *Model
		wantErr     bool
		errContains string
	}{
		{
			name: "nil subEntryForm",
			setupModel: func() *Model {
				return &Model{
					Platform:     &platform.Platform{OS: OSLinux},
					subEntryForm: nil,
				}
			},
			wantErr:     true,
			errContains: "subEntryForm is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := tt.setupModel()
			err := m.initFilePicker()

			if (err != nil) != tt.wantErr {
				t.Errorf("initFilePicker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("initFilePicker() error = %v, want error containing %v", err, tt.errContains)
			}
		})
	}
}

// TestFilePickerSuccessMessage tests that success messages are set after adding files
func TestFilePickerSuccessMessage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "target")
	err := os.Mkdir(targetDir, 0o750)
	if err != nil {
		t.Fatalf("Failed to create target directory: %v", err)
	}

	file1 := filepath.Join(targetDir, "file1.txt")
	err = os.WriteFile(file1, []byte("test"), 0o600)
	if err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}

	file2 := filepath.Join(targetDir, "file2.txt")
	err = os.WriteFile(file2, []byte("test"), 0o600)
	if err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}

	tests := []struct {
		name          string
		selectedFiles map[string]bool
		wantCount     int
		wantMessage   string
	}{
		{
			name: "single file added",
			selectedFiles: map[string]bool{
				file1: true,
			},
			wantCount:   1,
			wantMessage: "Added 1 file(s)",
		},
		{
			name: "multiple files added",
			selectedFiles: map[string]bool{
				file1: true,
				file2: true,
			},
			wantCount:   2,
			wantMessage: "Added 2 file(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := &Model{
				Platform: &platform.Platform{OS: OSLinux},
				subEntryForm: &SubEntryForm{
					LinuxTargetInput:   textinput.New(),
					WindowsTargetInput: textinput.New(),
					SelectedFiles:      tt.selectedFiles,
					Files:              []string{},
					AddFileMode:        ModePicker,
				},
			}

			m.subEntryForm.LinuxTargetInput.SetValue(targetDir)

			// Simulate enter key to confirm selection
			msg := mockKeyMsg("enter")
			updatedModel, _ := m.updateSubEntryFilePicker(msg)
			updatedM := updatedModel.(Model)

			// Verify files were added
			if len(updatedM.subEntryForm.Files) != tt.wantCount {
				t.Errorf("Added %d files, want %d", len(updatedM.subEntryForm.Files), tt.wantCount)
			}

			// Verify success message is set
			if updatedM.subEntryForm.SuccessMessage != tt.wantMessage {
				t.Errorf("Success message = %v, want %v", updatedM.subEntryForm.SuccessMessage, tt.wantMessage)
			}

			// Verify no error was set
			if updatedM.subEntryForm.Err != "" {
				t.Errorf("Unexpected error: %v", updatedM.subEntryForm.Err)
			}
		})
	}
}

// TestErrorClearedOnNextAction tests that errors are cleared on navigation
func TestErrorClearedOnNextAction(t *testing.T) {
	t.Parallel()

	m := &Model{
		Config:   &config.Config{},
		Platform: &platform.Platform{OS: OSLinux},
		subEntryForm: &SubEntryForm{
			LinuxTargetInput:   textinput.New(),
			WindowsTargetInput: textinput.New(),
			BackupInput:        textinput.New(),
			NameInput:          textinput.New(),
			FocusIndex:         0,
			Err:                "Previous error",
		},
	}

	// Simulate navigation (down arrow)
	msg := mockKeyMsg("down")
	updatedModel, _ := m.updateSubEntryForm(msg)
	updatedM := updatedModel.(Model)

	// Verify error was cleared
	if updatedM.subEntryForm.Err != "" {
		t.Errorf("Error was not cleared on navigation: %v", updatedM.subEntryForm.Err)
	}
}

// TestErrorClearedOnTyping tests that errors are cleared when typing
func TestErrorClearedOnTyping(t *testing.T) {
	t.Parallel()

	m := &Model{
		Platform: &platform.Platform{OS: OSLinux},
		subEntryForm: &SubEntryForm{
			NameInput:    textinput.New(),
			FocusIndex:   0,
			EditingField: true,
			Err:          "Previous error",
		},
	}

	// Enter name field edit mode
	m.subEntryForm.NameInput.Focus()

	// Simulate typing
	msg := mockKeyMsg("a")
	updatedModel, _ := m.updateSubEntryFieldInput(msg)
	updatedM := updatedModel.(Model)

	// Verify error was cleared
	if updatedM.subEntryForm.Err != "" {
		t.Errorf("Error was not cleared on typing: %v", updatedM.subEntryForm.Err)
	}
}

// mockKeyMsg creates a tea.KeyPressMsg for testing
func mockKeyMsg(key string) tea.KeyPressMsg {
	// Map common keys to their KeyType
	switch key {
	case KeyEnter:
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case KeyDown:
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case KeyEsc:
		return tea.KeyPressMsg{Code: tea.KeyEsc}
	default:
		// For single character keys, use KeyRunes
		return tea.KeyPressMsg{Code: rune(key[0]), Text: key}
	}
}
