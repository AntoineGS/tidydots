package forms

import (
	"errors"
	"maps"
	"strings"

	"charm.land/bubbles/v2/filepicker"
	"charm.land/bubbles/v2/textinput"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/tui/tuishared"
)

// SubEntryFieldType represents the type of field in the SubEntryForm
type SubEntryFieldType int

// SubEntryForm field type constants.
const (
	SubFieldName SubEntryFieldType = iota
	SubFieldLinux
	SubFieldWindows
	SubFieldBackup   // Config-specific
	SubFieldIsFolder // Config-specific toggle
	SubFieldFiles    // Config-specific list
	SubFieldIsSudo   // Sudo toggle
)

// AddFileMode represents the current mode for adding files to the files list
type AddFileMode int

const (
	// ModeNone indicates no file adding operation is active
	ModeNone AddFileMode = iota
	// ModeChoosing indicates user is choosing between browse/type options
	ModeChoosing
	// ModePicker indicates file picker is active for browsing
	ModePicker
	// ModeTextInput indicates manual text input mode is active
	ModeTextInput
)

// SubEntryForm holds state for editing SubEntry data
type SubEntryForm struct {
	Err            string
	SuccessMessage string
	OriginalValue  string
	Suggestions    []string
	Files          []string
	SelectedFiles  map[string]bool
	// Check and Run belong to setup entries, which this form does not edit (they
	// are edited in tidydots.yaml). They are carried through anyway so that a
	// form built from an entry cannot silently delete them on the way back out.
	Check              map[string]string
	Run                map[string]string
	NameInput          textinput.Model
	LinuxTargetInput   textinput.Model
	WindowsTargetInput textinput.Model
	BackupInput        textinput.Model
	NewFileInput       textinput.Model
	FilePicker         filepicker.Model
	EditingFileIndex   int
	TargetAppIdx       int
	EditSubIdx         int
	EditAppIdx         int
	FocusIndex         int
	FilesCursor        int
	SuggestionCursor   int
	ModeMenuCursor     int
	AddFileMode        AddFileMode
	IsFolder           bool
	ShowSuggestions    bool
	EditingField       bool
	AddingFile         bool
	EditingFile        bool
	IsSudo             bool
}

// GetFieldType returns the field type at the current focus index
func (f *SubEntryForm) GetFieldType() SubEntryFieldType {
	if f == nil {
		return SubFieldName
	}

	idx := f.FocusIndex

	// Common fields: name (0), linux (1), windows (2)
	switch idx {
	case 0:
		return SubFieldName
	case 1:
		return SubFieldLinux
	case 2:
		return SubFieldWindows
	}

	// Config-specific fields start at index 3
	if f.IsFolder {
		// Folder mode: backup (3), isFolder (4), isSudo (5)
		switch idx {
		case 3:
			return SubFieldBackup
		case 4:
			return SubFieldIsFolder
		case 5:
			return SubFieldIsSudo
		}
	} else {
		// Files mode: backup (3), isFolder (4), files (5), isSudo (6)
		switch idx {
		case 3:
			return SubFieldBackup
		case 4:
			return SubFieldIsFolder
		case 5:
			return SubFieldFiles
		case 6:
			return SubFieldIsSudo
		}
	}

	// Fallback to name field if index is out of range
	return SubFieldName
}

// MaxIndex returns the maximum focus index based on state
func (f *SubEntryForm) MaxIndex() int {
	if f == nil {
		return 0
	}

	// Common fields: name, linux, windows = 3 fields (0-2)
	// Config-specific fields start at 3
	if f.IsFolder {
		// Config folder: backup, isFolder, isSudo = 3 fields (3-5)
		return 5
	}

	// Config files: backup, isFolder, files, isSudo = 4 fields (3-6)
	return 6
}

// IsTextInputField returns true if the current field is a text input
func (f *SubEntryForm) IsTextInputField() bool {
	if f == nil {
		return false
	}

	ft := f.GetFieldType()
	switch ft {
	case SubFieldName, SubFieldLinux, SubFieldWindows, SubFieldBackup:
		return true
	case SubFieldIsFolder, SubFieldFiles, SubFieldIsSudo:
		// These fields don't have suggestions
	}

	return false
}

// IsToggleField returns true if the current field is a toggle
func (f *SubEntryForm) IsToggleField() bool {
	if f == nil {
		return false
	}

	ft := f.GetFieldType()

	return ft == SubFieldIsFolder || ft == SubFieldIsSudo
}

// UpdateFocus updates which input field is focused
func (f *SubEntryForm) UpdateFocus() {
	if f == nil {
		return
	}

	f.NameInput.Blur()
	f.LinuxTargetInput.Blur()
	f.WindowsTargetInput.Blur()
	f.BackupInput.Blur()
	f.NewFileInput.Blur()

	ft := f.GetFieldType()
	switch ft {
	case SubFieldName:
		f.NameInput.Focus()
	case SubFieldLinux:
		f.LinuxTargetInput.Focus()
	case SubFieldWindows:
		f.WindowsTargetInput.Focus()
	case SubFieldBackup:
		f.BackupInput.Focus()
	case SubFieldIsFolder, SubFieldFiles, SubFieldIsSudo:
		// Boolean and list fields don't use text input focus
	}
}

// EnterFieldEditMode enters edit mode for the current text field
func (f *SubEntryForm) EnterFieldEditMode() {
	if f == nil {
		return
	}

	f.EditingField = true
	ft := f.GetFieldType()

	switch ft {
	case SubFieldName:
		f.OriginalValue = f.NameInput.Value()
		f.NameInput.Focus()
		f.NameInput.SetCursor(len(f.NameInput.Value()))
	case SubFieldLinux:
		f.OriginalValue = f.LinuxTargetInput.Value()
		f.LinuxTargetInput.Focus()
		f.LinuxTargetInput.SetCursor(len(f.LinuxTargetInput.Value()))
	case SubFieldWindows:
		f.OriginalValue = f.WindowsTargetInput.Value()
		f.WindowsTargetInput.Focus()
		f.WindowsTargetInput.SetCursor(len(f.WindowsTargetInput.Value()))
	case SubFieldBackup:
		f.OriginalValue = f.BackupInput.Value()
		f.BackupInput.Focus()
		f.BackupInput.SetCursor(len(f.BackupInput.Value()))
	case SubFieldIsFolder, SubFieldFiles, SubFieldIsSudo:
		// Boolean and list fields don't use text input editing
	}
}

// CancelFieldEdit cancels editing and restores the original value
func (f *SubEntryForm) CancelFieldEdit() {
	if f == nil {
		return
	}

	ft := f.GetFieldType()
	switch ft {
	case SubFieldName:
		f.NameInput.SetValue(f.OriginalValue)
	case SubFieldLinux:
		f.LinuxTargetInput.SetValue(f.OriginalValue)
	case SubFieldWindows:
		f.WindowsTargetInput.SetValue(f.OriginalValue)
	case SubFieldBackup:
		f.BackupInput.SetValue(f.OriginalValue)
	case SubFieldIsFolder, SubFieldFiles, SubFieldIsSudo:
		// Boolean and list fields don't use text input restoration
	}

	f.EditingField = false
	f.ShowSuggestions = false
	f.Err = ""
	f.UpdateFocus()
}

// Validate checks if the SubEntryForm has valid data
func (f *SubEntryForm) Validate() error {
	if strings.TrimSpace(f.NameInput.Value()) == "" {
		return errors.New("entry name is required")
	}

	if strings.TrimSpace(f.BackupInput.Value()) == "" {
		return errors.New("backup path is required")
	}

	// Check if at least one target is specified
	hasTarget := strings.TrimSpace(f.LinuxTargetInput.Value()) != "" ||
		strings.TrimSpace(f.WindowsTargetInput.Value()) != ""

	if !hasTarget {
		return errors.New("at least one target is required")
	}

	return nil
}

// BuildSubEntry validates and returns the SubEntry from the form, or an error.
func (f *SubEntryForm) BuildSubEntry() (config.SubEntry, error) {
	if f == nil {
		return config.SubEntry{}, errors.New("no form data")
	}

	name := strings.TrimSpace(f.NameInput.Value())
	targets := BuildTargetsFromInputs(f.LinuxTargetInput, f.WindowsTargetInput)

	// Validation
	if name == "" {
		return config.SubEntry{}, errors.New("name is required")
	}

	if len(targets) == 0 {
		return config.SubEntry{}, errors.New("at least one target is required")
	}

	backup := strings.TrimSpace(f.BackupInput.Value())
	if backup == "" {
		return config.SubEntry{}, errors.New("backup path is required")
	}

	// Build SubEntry from form. Check and Run have no fields in this form, so they
	// are written back exactly as they came in: whatever the form does not carry
	// through is deleted from the config file when the caller saves.
	subEntry := config.SubEntry{
		Name:    name,
		Targets: targets,
		Sudo:    f.IsSudo,
		Backup:  backup,
		Check:   maps.Clone(f.Check),
		Run:     maps.Clone(f.Run),
	}

	// Add files if in files mode
	if !f.IsFolder {
		if len(f.Files) == 0 {
			return config.SubEntry{}, errors.New("at least one file is required when using Files mode")
		}
		subEntry.Files = make([]string, len(f.Files))
		copy(subEntry.Files, f.Files)
	}

	return subEntry, nil
}

// NewSubEntryForm creates a new SubEntryForm for testing purposes
func NewSubEntryForm(entry config.SubEntry) *SubEntryForm {
	nameInput := NewFormInput("e.g., nvim-config", tuishared.CharLimitName, tuishared.InputWidthNarrow)
	nameInput.SetValue(entry.Name)

	linuxTargetInput := NewFormInput("e.g., ~/.config/nvim", tuishared.CharLimitPath, tuishared.InputWidthNarrow)
	if target, ok := entry.Targets["linux"]; ok {
		linuxTargetInput.SetValue(target)
	}

	windowsTargetInput := NewFormInput("e.g., ~/AppData/Local/nvim", tuishared.CharLimitPath, tuishared.InputWidthNarrow)
	if target, ok := entry.Targets["windows"]; ok {
		windowsTargetInput.SetValue(target)
	}

	backupInput := NewFormInput("e.g., ./nvim", tuishared.CharLimitPath, tuishared.InputWidthNarrow)
	backupInput.SetValue(entry.Backup)

	return &SubEntryForm{
		NameInput:          nameInput,
		LinuxTargetInput:   linuxTargetInput,
		WindowsTargetInput: windowsTargetInput,
		BackupInput:        backupInput,
		IsSudo:             entry.Sudo,
		IsFolder:           entry.IsFolder(),
		Files:              entry.Files,
		Check:              maps.Clone(entry.Check),
		Run:                maps.Clone(entry.Run),
	}
}
