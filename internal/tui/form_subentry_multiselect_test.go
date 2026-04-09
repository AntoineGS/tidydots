package tui

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

const (
	testNvimDir = "/home/user/.config/nvim" // used as map key in toggle/navigation tests
	testInitLua = "init.lua"
)

// testNvimTmpDir creates a platform-appropriate absolute path for tests that
// call expandTargetPath (which uses filepath.Abs). Returns a path under t.TempDir().
func testNvimTmpDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".config", "nvim")
}

// TestMultiSelect_ToggleSelection tests toggling a file selection with space/tab
func TestMultiSelect_ToggleSelection(t *testing.T) {
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
	plat := &platform.Platform{OS: "linux"}
	m := NewModel(cfg, plat, false)
	m.initSubEntryForm(0, -1)

	// Set up file picker mode with a simulated current directory and path
	m.subEntryForm.AddFileMode = ModePicker
	m.subEntryForm.FilePicker.CurrentDirectory = testNvimDir
	m.subEntryForm.FilePicker.Path = testInitLua

	initialCount := len(m.subEntryForm.SelectedFiles)

	// Simulate space key to toggle selection
	updatedModel, _ := m.updateSubEntryFilePicker(createKeyMsg(" "))
	model := updatedModel.(Model)

	// Verify selection was added
	expectedPath := filepath.Join(testNvimDir, testInitLua)
	if !model.subEntryForm.SelectedFiles[expectedPath] {
		t.Errorf("file not selected: %s", expectedPath)
	}

	if len(model.subEntryForm.SelectedFiles) != initialCount+1 {
		t.Errorf("selectedFiles count = %d, want %d", len(model.subEntryForm.SelectedFiles), initialCount+1)
	}

	// Toggle again to deselect
	updatedModel2, _ := model.updateSubEntryFilePicker(createKeyMsg(" "))
	model2 := updatedModel2.(Model)

	if model2.subEntryForm.SelectedFiles[expectedPath] {
		t.Errorf("file still selected after toggle: %s", expectedPath)
	}

	if len(model2.subEntryForm.SelectedFiles) != initialCount {
		t.Errorf("selectedFiles count = %d, want %d after deselect", len(model2.subEntryForm.SelectedFiles), initialCount)
	}
}

// TestMultiSelect_ToggleWithTab tests toggling selection with tab key
func TestMultiSelect_ToggleWithTab(t *testing.T) {
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
	plat := &platform.Platform{OS: "linux"}
	m := NewModel(cfg, plat, false)
	m.initSubEntryForm(0, -1)

	// Set up file picker mode
	m.subEntryForm.AddFileMode = ModePicker
	m.subEntryForm.FilePicker.CurrentDirectory = testNvimDir
	m.subEntryForm.FilePicker.Path = testInitLua

	// Tab key should also toggle selection
	updatedModel, _ := m.updateSubEntryFilePicker(tea.KeyPressMsg{Code: tea.KeyTab})
	model := updatedModel.(Model)

	expectedPath := filepath.Join(testNvimDir, testInitLua)
	if !model.subEntryForm.SelectedFiles[expectedPath] {
		t.Errorf("file not selected with tab key: %s", expectedPath)
	}
}

// TestMultiSelect_PersistAcrossNavigation tests selections persist when navigating
func TestMultiSelect_PersistAcrossNavigation(t *testing.T) {
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
	plat := &platform.Platform{OS: "linux"}
	m := NewModel(cfg, plat, false)
	m.initSubEntryForm(0, -1)

	// Select first file
	m.subEntryForm.AddFileMode = ModePicker
	m.subEntryForm.FilePicker.CurrentDirectory = testNvimDir
	m.subEntryForm.FilePicker.Path = testInitLua

	updatedModel, _ := m.updateSubEntryFilePicker(createKeyMsg(" "))
	model := updatedModel.(Model)

	file1Path := filepath.Join(testNvimDir, testInitLua)

	// Simulate navigation to another file
	model.subEntryForm.FilePicker.Path = "plugins.lua"

	// Select second file
	updatedModel2, _ := model.updateSubEntryFilePicker(createKeyMsg(" "))
	model2 := updatedModel2.(Model)

	file2Path := filepath.Join(testNvimDir, "plugins.lua")

	// Verify both files are still selected
	if !model2.subEntryForm.SelectedFiles[file1Path] {
		t.Errorf("first file not selected after navigation: %s", file1Path)
	}

	if !model2.subEntryForm.SelectedFiles[file2Path] {
		t.Errorf("second file not selected: %s", file2Path)
	}

	if len(model2.subEntryForm.SelectedFiles) != 2 {
		t.Errorf("selectedFiles count = %d, want 2", len(model2.subEntryForm.SelectedFiles))
	}
}

// TestMultiSelect_ConfirmMultipleFiles tests confirming multiple selections with enter
func TestMultiSelect_ConfirmMultipleFiles(t *testing.T) {
	// Use platform-appropriate absolute paths for tests that call expandTargetPath
	nvimDir := testNvimTmpDir(t)

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
	plat := &platform.Platform{OS: "linux"}
	m := NewModel(cfg, plat, false)
	m.initSubEntryForm(0, -1)

	// Set up target path
	m.subEntryForm.LinuxTargetInput.SetValue(nvimDir)

	// Pre-populate selectedFiles map with multiple selections using filepath.Join
	m.subEntryForm.AddFileMode = ModePicker
	m.subEntryForm.SelectedFiles[filepath.Join(nvimDir, testInitLua)] = true
	m.subEntryForm.SelectedFiles[filepath.Join(nvimDir, "plugins.lua")] = true
	m.subEntryForm.SelectedFiles[filepath.Join(nvimDir, "lua", "config.lua")] = true

	initialFilesCount := len(m.subEntryForm.Files)

	// Simulate enter key to confirm selections
	updatedModel, _ := m.updateSubEntryFilePicker(createKeyMsg(KeyEnter))
	model := updatedModel.(Model)

	// Verify all files were added to the files list
	expectedCount := initialFilesCount + 3
	if len(model.subEntryForm.Files) != expectedCount {
		t.Errorf("files count = %d, want %d", len(model.subEntryForm.Files), expectedCount)
	}

	// Verify selectedFiles map was cleared
	if len(model.subEntryForm.SelectedFiles) != 0 {
		t.Errorf("selectedFiles not cleared: count = %d, want 0", len(model.subEntryForm.SelectedFiles))
	}

	// Verify mode was reset
	if model.subEntryForm.AddFileMode != ModeNone {
		t.Errorf("addFileMode = %d, want %d (ModeNone)", model.subEntryForm.AddFileMode, ModeNone)
	}

	// Verify files contain expected relative paths (using filepath.Join for cross-platform)
	expectedFiles := []string{testInitLua, "plugins.lua", filepath.Join("lua", "config.lua")}
	for _, expected := range expectedFiles {
		found := false
		for _, file := range model.subEntryForm.Files {
			if file == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected file not found in files list: %s", expected)
		}
	}
}

// TestMultiSelect_EmptyConfirm tests confirming with no selections
func TestMultiSelect_EmptyConfirm(t *testing.T) {
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
	plat := &platform.Platform{OS: "linux"}
	m := NewModel(cfg, plat, false)
	m.initSubEntryForm(0, -1)

	// Set up file picker mode with no selections
	m.subEntryForm.AddFileMode = ModePicker
	m.subEntryForm.FilePicker.CurrentDirectory = testNvimDir
	m.subEntryForm.FilePicker.Path = ""

	initialFilesCount := len(m.subEntryForm.Files)

	// Simulate enter key with no selections
	updatedModel, _ := m.updateSubEntryFilePicker(createKeyMsg(KeyEnter))
	model := updatedModel.(Model)

	// Verify no files were added
	if len(model.subEntryForm.Files) != initialFilesCount {
		t.Errorf("files count changed: got %d, want %d", len(model.subEntryForm.Files), initialFilesCount)
	}

	// Verify mode was reset
	if model.subEntryForm.AddFileMode != ModeNone {
		t.Errorf("addFileMode = %d, want %d (ModeNone)", model.subEntryForm.AddFileMode, ModeNone)
	}
}

// TestMultiSelect_CancelPreservesNoFiles tests that canceling doesn't add files
func TestMultiSelect_CancelPreservesNoFiles(t *testing.T) {
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
	plat := &platform.Platform{OS: "linux"}
	m := NewModel(cfg, plat, false)
	m.initSubEntryForm(0, -1)

	// Set up file picker mode with selections
	m.subEntryForm.AddFileMode = ModePicker
	m.subEntryForm.SelectedFiles[testNvimDir+"/"+testInitLua] = true
	m.subEntryForm.SelectedFiles[testNvimDir+"/plugins.lua"] = true

	initialFilesCount := len(m.subEntryForm.Files)

	// Simulate ESC key to cancel
	updatedModel, _ := m.updateSubEntryFilePicker(createKeyMsg(KeyEsc))
	model := updatedModel.(Model)

	// Verify no files were added
	if len(model.subEntryForm.Files) != initialFilesCount {
		t.Errorf("files count changed after cancel: got %d, want %d", len(model.subEntryForm.Files), initialFilesCount)
	}

	// Verify selections were cleared
	if len(model.subEntryForm.SelectedFiles) != 0 {
		t.Errorf("selectedFiles not cleared after cancel: count = %d, want 0", len(model.subEntryForm.SelectedFiles))
	}

	// Verify mode was reset
	if model.subEntryForm.AddFileMode != ModeNone {
		t.Errorf("addFileMode = %d, want %d (ModeNone)", model.subEntryForm.AddFileMode, ModeNone)
	}
}

// TestMultiSelect_SelectionCount tests tracking the number of selections
func TestMultiSelect_SelectionCount(t *testing.T) {
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
	plat := &platform.Platform{OS: "linux"}
	m := NewModel(cfg, plat, false)
	m.initSubEntryForm(0, -1)

	// Start with empty selections
	if len(m.subEntryForm.SelectedFiles) != 0 {
		t.Errorf("initial selectedFiles count = %d, want 0", len(m.subEntryForm.SelectedFiles))
	}

	// Add selections
	m.subEntryForm.SelectedFiles[testNvimDir+"/"+testInitLua] = true
	m.subEntryForm.SelectedFiles[testNvimDir+"/plugins.lua"] = true
	m.subEntryForm.SelectedFiles[testNvimDir+"/lua/config.lua"] = true

	// Verify count
	if len(m.subEntryForm.SelectedFiles) != 3 {
		t.Errorf("selectedFiles count = %d, want 3", len(m.subEntryForm.SelectedFiles))
	}

	// Remove one selection
	delete(m.subEntryForm.SelectedFiles, testNvimDir+"/plugins.lua")

	// Verify count updated
	if len(m.subEntryForm.SelectedFiles) != 2 {
		t.Errorf("selectedFiles count after delete = %d, want 2", len(m.subEntryForm.SelectedFiles))
	}
}

// TestMultiSelect_DuplicateSelectionIgnored tests that selecting same file twice doesn't duplicate
func TestMultiSelect_DuplicateSelectionIgnored(t *testing.T) {
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
	plat := &platform.Platform{OS: "linux"}
	m := NewModel(cfg, plat, false)
	m.initSubEntryForm(0, -1)

	filePath := testNvimDir + "/" + testInitLua

	// Select file twice
	m.subEntryForm.SelectedFiles[filePath] = true
	m.subEntryForm.SelectedFiles[filePath] = true

	// Verify only one selection
	if len(m.subEntryForm.SelectedFiles) != 1 {
		t.Errorf("selectedFiles count = %d, want 1", len(m.subEntryForm.SelectedFiles))
	}

	if !m.subEntryForm.SelectedFiles[filePath] {
		t.Errorf("file not selected: %s", filePath)
	}
}

// TestMultiSelect_MixedSelectionsAndConfirm tests selecting, deselecting, and confirming
func TestMultiSelect_MixedSelectionsAndConfirm(t *testing.T) {
	// Use platform-appropriate absolute paths for tests that call expandTargetPath
	nvimDir := testNvimTmpDir(t)

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
	plat := &platform.Platform{OS: "linux"}
	m := NewModel(cfg, plat, false)
	m.initSubEntryForm(0, -1)

	// Set up target path
	m.subEntryForm.LinuxTargetInput.SetValue(nvimDir)

	// Select multiple files using filepath.Join
	m.subEntryForm.SelectedFiles[filepath.Join(nvimDir, testInitLua)] = true
	m.subEntryForm.SelectedFiles[filepath.Join(nvimDir, "plugins.lua")] = true
	m.subEntryForm.SelectedFiles[filepath.Join(nvimDir, "colors.lua")] = true

	// Deselect one
	delete(m.subEntryForm.SelectedFiles, filepath.Join(nvimDir, "colors.lua"))

	// Verify count before confirm
	if len(m.subEntryForm.SelectedFiles) != 2 {
		t.Errorf("selectedFiles count before confirm = %d, want 2", len(m.subEntryForm.SelectedFiles))
	}

	// Confirm selections
	m.subEntryForm.AddFileMode = ModePicker
	updatedModel, _ := m.updateSubEntryFilePicker(createKeyMsg(KeyEnter))
	model := updatedModel.(Model)

	// Verify only 2 files were added (not 3)
	if len(model.subEntryForm.Files) != 2 {
		t.Errorf("files count = %d, want 2", len(model.subEntryForm.Files))
	}

	// Verify correct files were added
	expectedFiles := []string{testInitLua, "plugins.lua"}
	for _, expected := range expectedFiles {
		found := false
		for _, file := range model.subEntryForm.Files {
			if file == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected file not found in files list: %s", expected)
		}
	}

	// Verify deselected file was not added
	for _, file := range model.subEntryForm.Files {
		if file == "colors.lua" {
			t.Errorf("deselected file was added: colors.lua")
		}
	}
}
