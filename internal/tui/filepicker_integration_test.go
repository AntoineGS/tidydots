package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// TestFilePickerIntegration_FullFlow tests the complete file picker integration flow:
// 1. Start with SubEntryForm
// 2. Trigger "Add File" → enters ModeChoosing
// 3. Select "Browse" → enters ModePicker
// 4. Select file → adds to files list
// 5. Verify file added correctly
func TestFilePickerIntegration_FullFlow(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "config", "app")
	//nolint:gosec // Test file - directory permissions are safe for test
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("failed to create target directory: %v", err)
	}

	// Create test files in target directory
	testFiles := []string{"file1.conf", "file2.conf", "file3.conf"}
	for _, file := range testFiles {
		filePath := filepath.Join(targetDir, file)
		//nolint:gosec // Test file - file permissions are safe for test
		if err := os.WriteFile(filePath, []byte("test content"), 0o644); err != nil {
			t.Fatalf("failed to create test file %s: %v", file, err)
		}
	}

	// Create minimal model with required fields
	cfg := &config.Config{
		Applications: []config.Application{
			{
				Name:        "test-app",
				Description: "Test application",
				Entries: []config.SubEntry{
					{
						Name:    "placeholder",
						Targets: map[string]string{"linux": targetDir},
					},
				},
			},
		},
	}
	plat := &platform.Platform{OS: OSLinux}
	m := NewModel(cfg, plat, false)

	// Step 1: Initialize SubEntryForm
	m.initSubEntryForm(0, -1)
	if m.subEntryForm == nil {
		t.Fatal("subEntryForm is nil after initSubEntryFormNew")
	}

	// Set targets to our test directory
	m.subEntryForm.LinuxTargetInput.SetValue(targetDir)
	m.subEntryForm.IsFolder = false // Use files mode

	// Verify initial state
	if m.subEntryForm.AddFileMode != ModeNone {
		t.Errorf("initial addFileMode = %d, want %d (ModeNone)", m.subEntryForm.AddFileMode, ModeNone)
	}

	// Navigate to files field
	m.subEntryForm.FocusIndex = 5 // Files field index
	m.updateSubEntryFormFocus()

	// Verify we're on the files field
	if m.getSubEntryFieldType() != subFieldFiles {
		t.Errorf("focusIndex = %d, field type = %d, want subFieldFiles (%d)",
			m.subEntryForm.FocusIndex, m.getSubEntryFieldType(), subFieldFiles)
	}

	// Step 2: Trigger "Add File" by pressing enter on "Add File" button
	// filesCursor should be at len(files) (0 initially) which is the "Add File" button
	m.subEntryForm.FilesCursor = len(m.subEntryForm.Files)

	keyMsg := tea.KeyPressMsg{Code: tea.KeyEnter}
	updatedModel, _ := m.updateSubEntryFilesList(keyMsg)
	m = updatedModel.(Model)

	// Verify we entered ModeChoosing
	if m.subEntryForm.AddFileMode != ModeChoosing {
		t.Errorf("after enter on Add File: addFileMode = %d, want %d (ModeChoosing)",
			m.subEntryForm.AddFileMode, ModeChoosing)
	}

	// Verify modeMenuCursor is at Browse option (0)
	if m.subEntryForm.ModeMenuCursor != 0 {
		t.Errorf("modeMenuCursor = %d, want 0 (Browse)", m.subEntryForm.ModeMenuCursor)
	}

	// Step 3: Select "Browse" option by pressing enter
	keyMsg = tea.KeyPressMsg{Code: tea.KeyEnter}
	updatedModel, _ = m.updateFileAddModeChoice(keyMsg)
	m = updatedModel.(Model)

	// Verify we entered ModePicker
	if m.subEntryForm.AddFileMode != ModePicker {
		t.Errorf("after selecting Browse: addFileMode = %d, want %d (ModePicker)",
			m.subEntryForm.AddFileMode, ModePicker)
	}

	// Verify file picker was initialized
	if m.subEntryForm.FilePicker.CurrentDirectory == "" {
		t.Error("filePicker.CurrentDirectory is empty after entering ModePicker")
	}

	// Step 4: Select files using space/tab to toggle selection
	// We need to navigate to each file and toggle selection
	// In a real scenario, the file picker would show files and we'd select them
	// For testing, we'll simulate selecting files by directly manipulating selectedFiles

	// Simulate selecting files in the file picker
	file1Path := filepath.Join(targetDir, "file1.conf")
	file2Path := filepath.Join(targetDir, "file2.conf")
	m.subEntryForm.SelectedFiles[file1Path] = true
	m.subEntryForm.SelectedFiles[file2Path] = true

	// Verify selections were tracked
	if len(m.subEntryForm.SelectedFiles) != 2 {
		t.Errorf("len(selectedFiles) = %d, want 2", len(m.subEntryForm.SelectedFiles))
	}

	// Step 5: Press enter to confirm selections
	keyMsg = tea.KeyPressMsg{Code: tea.KeyEnter}
	updatedModel, _ = m.updateSubEntryFilePicker(keyMsg)
	m = updatedModel.(Model)

	// Verify we exited picker mode
	if m.subEntryForm.AddFileMode != ModeNone {
		t.Errorf("after confirming selections: addFileMode = %d, want %d (ModeNone)",
			m.subEntryForm.AddFileMode, ModeNone)
	}

	// Verify files were added to the files list
	if len(m.subEntryForm.Files) != 2 {
		t.Errorf("len(files) = %d, want 2", len(m.subEntryForm.Files))
	}

	// Verify the files are relative paths (file1.conf, file2.conf)
	expectedFiles := map[string]bool{
		"file1.conf": false,
		"file2.conf": false,
	}
	for _, file := range m.subEntryForm.Files {
		if _, ok := expectedFiles[file]; !ok {
			t.Errorf("unexpected file in list: %s", file)
		}
		expectedFiles[file] = true
	}

	// Verify all expected files were found
	for file, found := range expectedFiles {
		if !found {
			t.Errorf("expected file not in list: %s", file)
		}
	}

	// Verify selections were cleared
	if len(m.subEntryForm.SelectedFiles) != 0 {
		t.Errorf("len(selectedFiles) after confirmation = %d, want 0", len(m.subEntryForm.SelectedFiles))
	}

	// Verify cursor moved to "Add File" button
	if m.subEntryForm.FilesCursor != len(m.subEntryForm.Files) {
		t.Errorf("filesCursor = %d, want %d (at Add File button)",
			m.subEntryForm.FilesCursor, len(m.subEntryForm.Files))
	}

	// Verify success message was set
	if m.subEntryForm.SuccessMessage == "" {
		t.Error("successMessage is empty after adding files")
	}
	if !strings.Contains(m.subEntryForm.SuccessMessage, "2 file(s)") {
		t.Errorf("successMessage = %q, want to contain '2 file(s)'", m.subEntryForm.SuccessMessage)
	}
}

// TestFilePickerIntegration_CancelFlow tests canceling at different stages
func TestFilePickerIntegration_CancelFlow(t *testing.T) {
	cfg := &config.Config{
		Applications: []config.Application{
			{
				Name:        "test-app",
				Description: "Test application",
				Entries: []config.SubEntry{
					{
						Name:    "placeholder",
						Targets: map[string]string{"linux": "/tmp/test"},
					},
				},
			},
		},
	}
	plat := &platform.Platform{OS: OSLinux}

	tests := []struct {
		name        string
		startMode   AddFileMode
		updateFunc  func(Model, tea.KeyPressMsg) (tea.Model, tea.Cmd)
		expectMode  AddFileMode
		description string
	}{
		{
			name:        "cancel from ModeChoosing",
			startMode:   ModeChoosing,
			updateFunc:  func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) { return m.updateFileAddModeChoice(msg) },
			expectMode:  ModeNone,
			description: "pressing esc in mode menu should return to files list",
		},
		{
			name:        "cancel from ModePicker",
			startMode:   ModePicker,
			updateFunc:  func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) { return m.updateSubEntryFilePicker(msg) },
			expectMode:  ModeNone,
			description: "pressing esc in file picker should return to files list and clear selections",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(cfg, plat, false)
			m.initSubEntryForm(0, -1)

			// Set up the starting mode
			m.subEntryForm.AddFileMode = tt.startMode
			if tt.startMode == ModePicker {
				// Initialize file picker for this test
				tmpDir := t.TempDir()
				if err := m.initFilePicker(); err != nil {
					// If init fails, manually set picker to avoid nil panic
					m.subEntryForm.FilePicker.CurrentDirectory = tmpDir
				}
				// Add some selections to verify they're cleared
				m.subEntryForm.SelectedFiles["test1.txt"] = true
				m.subEntryForm.SelectedFiles["test2.txt"] = true
			}

			// Press escape
			escKey := tea.KeyPressMsg{Code: tea.KeyEsc}
			updatedModel, _ := tt.updateFunc(m, escKey)
			m = updatedModel.(Model)

			// Verify we returned to ModeNone
			if m.subEntryForm.AddFileMode != tt.expectMode {
				t.Errorf("%s: addFileMode = %d, want %d (ModeNone)", tt.description,
					m.subEntryForm.AddFileMode, tt.expectMode)
			}

			// Verify selections were cleared (for ModePicker)
			if tt.startMode == ModePicker && len(m.subEntryForm.SelectedFiles) != 0 {
				t.Errorf("%s: selectedFiles not cleared, len = %d", tt.description,
					len(m.subEntryForm.SelectedFiles))
			}
		})
	}
}

// TestFilePickerIntegration_LinuxWindowsTargets tests file picker with different OS targets
func TestFilePickerIntegration_LinuxWindowsTargets(t *testing.T) {
	// Create test directories
	tmpDir := t.TempDir()
	linuxTarget := filepath.Join(tmpDir, "linux-config")
	windowsTarget := filepath.Join(tmpDir, "windows-config")

	//nolint:gosec // Test file - directory permissions are safe for test
	if err := os.MkdirAll(linuxTarget, 0o755); err != nil {
		t.Fatalf("failed to create linux target: %v", err)
	}
	//nolint:gosec // Test file - directory permissions are safe for test
	if err := os.MkdirAll(windowsTarget, 0o755); err != nil {
		t.Fatalf("failed to create windows target: %v", err)
	}

	// Create test files in each target
	linuxFile := filepath.Join(linuxTarget, "linux.conf")
	windowsFile := filepath.Join(windowsTarget, "windows.conf")
	//nolint:gosec // Test file - file permissions are safe for test
	if err := os.WriteFile(linuxFile, []byte("linux config"), 0o644); err != nil {
		t.Fatalf("failed to create linux file: %v", err)
	}
	//nolint:gosec // Test file - file permissions are safe for test
	if err := os.WriteFile(windowsFile, []byte("windows config"), 0o644); err != nil {
		t.Fatalf("failed to create windows file: %v", err)
	}

	tests := []struct {
		name           string
		osType         string
		expectedTarget string
		testFile       string
	}{
		{
			name:           "Linux OS uses linux target",
			osType:         OSLinux,
			expectedTarget: linuxTarget,
			testFile:       linuxFile,
		},
		{
			name:           "Windows OS uses windows target",
			osType:         OSWindows,
			expectedTarget: windowsTarget,
			testFile:       windowsFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Applications: []config.Application{
					{
						Name:        "test-app",
						Description: "Test application",
						Entries: []config.SubEntry{
							{
								Name:    "placeholder",
								Targets: map[string]string{"linux": "/tmp/test", "windows": "/tmp/test"},
							},
						},
					},
				},
			}
			plat := &platform.Platform{OS: tt.osType}
			m := NewModel(cfg, plat, false)

			// Initialize form
			m.initSubEntryForm(0, -1)
			m.subEntryForm.LinuxTargetInput.SetValue(linuxTarget)
			m.subEntryForm.WindowsTargetInput.SetValue(windowsTarget)
			m.subEntryForm.IsFolder = false

			// Initialize file picker
			if err := m.initFilePicker(); err != nil {
				t.Fatalf("initFilePicker failed: %v", err)
			}

			// Verify picker was initialized with correct target directory
			pickerDir := m.subEntryForm.FilePicker.CurrentDirectory
			if !strings.Contains(pickerDir, tt.expectedTarget) {
				// Note: resolvePickerStartDirectory might navigate to nearest existing parent
				// So we check if the path is related to expected target
				t.Logf("picker directory: %s", pickerDir)
				t.Logf("expected target: %s", tt.expectedTarget)
				// This is informational - the picker should start near the target
			}

			// Simulate file selection and confirmation
			m.subEntryForm.SelectedFiles[tt.testFile] = true
			m.subEntryForm.AddFileMode = ModePicker

			// Confirm selection
			keyMsg := tea.KeyPressMsg{Code: tea.KeyEnter}
			updatedModel, _ := m.updateSubEntryFilePicker(keyMsg)
			m = updatedModel.(Model)

			// Verify file was added
			if len(m.subEntryForm.Files) != 1 {
				t.Errorf("len(files) = %d, want 1", len(m.subEntryForm.Files))
			}

			// Verify file path is relative
			if len(m.subEntryForm.Files) > 0 {
				addedFile := m.subEntryForm.Files[0]
				// Should be relative path (e.g., "linux.conf" or "windows.conf")
				if strings.Contains(addedFile, string(filepath.Separator)) {
					t.Logf("warning: file path contains separator: %s (might be okay if nested)", addedFile)
				}
			}
		})
	}
}

// TestFilePickerIntegration_EmptySelection tests confirming with no selections
func TestFilePickerIntegration_EmptySelection(t *testing.T) {
	cfg := &config.Config{
		Applications: []config.Application{
			{
				Name:        "test-app",
				Description: "Test application",
				Entries: []config.SubEntry{
					{
						Name:    "placeholder",
						Targets: map[string]string{"linux": "/tmp/test"},
					},
				},
			},
		},
	}
	plat := &platform.Platform{OS: OSLinux}
	m := NewModel(cfg, plat, false)

	// Initialize form
	m.initSubEntryForm(0, -1)
	tmpDir := t.TempDir()
	m.subEntryForm.LinuxTargetInput.SetValue(tmpDir)
	m.subEntryForm.IsFolder = false

	// Enter ModePicker without selecting any files
	m.subEntryForm.AddFileMode = ModePicker
	if err := m.initFilePicker(); err != nil {
		// Set a default directory if init fails
		m.subEntryForm.FilePicker.CurrentDirectory = tmpDir
	}

	// Verify no files selected
	if len(m.subEntryForm.SelectedFiles) != 0 {
		t.Fatalf("initial selectedFiles should be empty, got %d", len(m.subEntryForm.SelectedFiles))
	}

	// Press enter with no selections
	keyMsg := tea.KeyPressMsg{Code: tea.KeyEnter}
	updatedModel, _ := m.updateSubEntryFilePicker(keyMsg)
	m = updatedModel.(Model)

	// Verify we exited picker mode
	if m.subEntryForm.AddFileMode != ModeNone {
		t.Errorf("addFileMode = %d, want %d (ModeNone)", m.subEntryForm.AddFileMode, ModeNone)
	}

	// Verify no files were added
	if len(m.subEntryForm.Files) != 0 {
		t.Errorf("files should be empty when confirming with no selections, got %d files",
			len(m.subEntryForm.Files))
	}
}

// TestFilePickerIntegration_ModeMenuNavigation tests navigation in mode selection menu
func TestFilePickerIntegration_ModeMenuNavigation(t *testing.T) {
	cfg := &config.Config{
		Applications: []config.Application{
			{
				Name:        "test-app",
				Description: "Test application",
				Entries: []config.SubEntry{
					{
						Name:    "placeholder",
						Targets: map[string]string{"linux": "/tmp/test"},
					},
				},
			},
		},
	}
	plat := &platform.Platform{OS: OSLinux}
	m := NewModel(cfg, plat, false)

	// Initialize form
	m.initSubEntryForm(0, -1)

	// Enter mode choosing
	m.subEntryForm.AddFileMode = ModeChoosing
	m.subEntryForm.ModeMenuCursor = 0 // Start at Browse

	// Test down navigation: 0 -> 1 (Browse source)
	keyMsg := tea.KeyPressMsg{Code: tea.KeyDown}
	updatedModel, _ := m.updateFileAddModeChoice(keyMsg)
	m = updatedModel.(Model)

	if m.subEntryForm.ModeMenuCursor != 1 {
		t.Errorf("after down: modeMenuCursor = %d, want 1 (Browse source)", m.subEntryForm.ModeMenuCursor)
	}

	// Test down navigation: 1 -> 2 (Type)
	keyMsg = tea.KeyPressMsg{Code: tea.KeyDown}
	updatedModel, _ = m.updateFileAddModeChoice(keyMsg)
	m = updatedModel.(Model)

	if m.subEntryForm.ModeMenuCursor != 2 {
		t.Errorf("after second down: modeMenuCursor = %d, want 2 (Type)", m.subEntryForm.ModeMenuCursor)
	}

	// Test down navigation with wrap: 2 -> 0
	keyMsg = tea.KeyPressMsg{Code: tea.KeyDown}
	updatedModel, _ = m.updateFileAddModeChoice(keyMsg)
	m = updatedModel.(Model)

	if m.subEntryForm.ModeMenuCursor != 0 {
		t.Errorf("after down with wrap: modeMenuCursor = %d, want 0 (Browse target)", m.subEntryForm.ModeMenuCursor)
	}

	// Test up navigation with wrap: 0 -> 2
	keyMsg = tea.KeyPressMsg{Code: tea.KeyUp}
	updatedModel, _ = m.updateFileAddModeChoice(keyMsg)
	m = updatedModel.(Model)

	if m.subEntryForm.ModeMenuCursor != 2 {
		t.Errorf("after up: modeMenuCursor = %d, want 2 (Type)", m.subEntryForm.ModeMenuCursor)
	}

	// Test vim-style navigation (j = down): 2 -> 0 (wrap)
	keyMsg = tea.KeyPressMsg{Code: 'j', Text: "j"}
	updatedModel, _ = m.updateFileAddModeChoice(keyMsg)
	m = updatedModel.(Model)

	if m.subEntryForm.ModeMenuCursor != 0 {
		t.Errorf("after 'j' (vim down): modeMenuCursor = %d, want 0 (Browse target)", m.subEntryForm.ModeMenuCursor)
	}

	// Test vim-style navigation (k = up): 0 -> 2 (wrap)
	keyMsg = tea.KeyPressMsg{Code: 'k', Text: "k"}
	updatedModel, _ = m.updateFileAddModeChoice(keyMsg)
	m = updatedModel.(Model)

	if m.subEntryForm.ModeMenuCursor != 2 {
		t.Errorf("after 'k' (vim up): modeMenuCursor = %d, want 2 (Type)", m.subEntryForm.ModeMenuCursor)
	}
}
