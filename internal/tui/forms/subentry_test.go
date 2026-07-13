package forms_test

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/tui/forms"
)

func TestNewSubEntryForm_Empty(t *testing.T) {
	form := forms.NewSubEntryForm(config.SubEntry{})

	if form == nil {
		t.Fatal("NewSubEntryForm returned nil")
	}

	t.Run("inputs_are_empty", func(t *testing.T) {
		if form.NameInput.Value() != "" {
			t.Errorf("NameInput = %q, want empty", form.NameInput.Value())
		}
		if form.BackupInput.Value() != "" {
			t.Errorf("BackupInput = %q, want empty", form.BackupInput.Value())
		}
		if form.LinuxTargetInput.Value() != "" {
			t.Errorf("LinuxTargetInput = %q, want empty", form.LinuxTargetInput.Value())
		}
		if form.WindowsTargetInput.Value() != "" {
			t.Errorf("WindowsTargetInput = %q, want empty", form.WindowsTargetInput.Value())
		}
	})

	t.Run("sudo_is_false_by_default", func(t *testing.T) {
		if form.IsSudo {
			t.Error("IsSudo = true, want false")
		}
	})

	t.Run("is_folder_false_when_no_backup", func(t *testing.T) {
		// IsFolder requires both a backup path and no files;
		// without a backup, IsConfig() is false, so IsFolder() is false.
		if form.IsFolder {
			t.Error("IsFolder = true, want false for entry with no backup")
		}
	})
}

func TestNewSubEntryForm_PopulatedEntry(t *testing.T) {
	entry := config.SubEntry{
		Name:   "nvim-config",
		Backup: "./nvim",
		Sudo:   true,
		Files:  []string{"init.lua", "lua/config.lua"},
		Targets: map[string]string{
			"linux":   "~/.config/nvim",
			"windows": "~/AppData/Local/nvim",
		},
	}
	form := forms.NewSubEntryForm(entry)

	if form == nil {
		t.Fatal("NewSubEntryForm returned nil")
	}

	t.Run("name_loaded", func(t *testing.T) {
		if form.NameInput.Value() != "nvim-config" {
			t.Errorf("NameInput = %q, want %q", form.NameInput.Value(), "nvim-config")
		}
	})

	t.Run("backup_loaded", func(t *testing.T) {
		if form.BackupInput.Value() != "./nvim" {
			t.Errorf("BackupInput = %q, want %q", form.BackupInput.Value(), "./nvim")
		}
	})

	t.Run("targets_loaded", func(t *testing.T) {
		if form.LinuxTargetInput.Value() != "~/.config/nvim" {
			t.Errorf("LinuxTargetInput = %q, want %q", form.LinuxTargetInput.Value(), "~/.config/nvim")
		}
		if form.WindowsTargetInput.Value() != "~/AppData/Local/nvim" {
			t.Errorf("WindowsTargetInput = %q, want %q", form.WindowsTargetInput.Value(), "~/AppData/Local/nvim")
		}
	})

	t.Run("sudo_loaded", func(t *testing.T) {
		if !form.IsSudo {
			t.Error("IsSudo = false, want true")
		}
	})

	t.Run("files_loaded", func(t *testing.T) {
		if len(form.Files) != 2 {
			t.Errorf("len(Files) = %d, want 2", len(form.Files))
		}
		if form.Files[0] != "init.lua" {
			t.Errorf("Files[0] = %q, want %q", form.Files[0], "init.lua")
		}
		if form.Files[1] != "lua/config.lua" {
			t.Errorf("Files[1] = %q, want %q", form.Files[1], "lua/config.lua")
		}
	})

	t.Run("is_folder_false_when_files_present", func(t *testing.T) {
		if form.IsFolder {
			t.Error("IsFolder = true, want false when files are specified")
		}
	})
}

func TestNewSubEntryForm_FolderEntry(t *testing.T) {
	entry := config.SubEntry{
		Name:   "nvim-config",
		Backup: "./nvim",
		Targets: map[string]string{
			"linux": "~/.config/nvim",
		},
		// No Files → IsFolder
	}
	form := forms.NewSubEntryForm(entry)

	if !form.IsFolder {
		t.Error("IsFolder = false, want true when no files specified")
	}
}

func TestNewSubEntryForm_PartialTargets(t *testing.T) {
	t.Run("linux_only", func(t *testing.T) {
		entry := config.SubEntry{
			Name:    "test",
			Backup:  "./test",
			Targets: map[string]string{"linux": "~/.config/test"},
		}
		form := forms.NewSubEntryForm(entry)

		if form.LinuxTargetInput.Value() != "~/.config/test" {
			t.Errorf("LinuxTargetInput = %q, want %q", form.LinuxTargetInput.Value(), "~/.config/test")
		}
		if form.WindowsTargetInput.Value() != "" {
			t.Errorf("WindowsTargetInput = %q, want empty", form.WindowsTargetInput.Value())
		}
	})

	t.Run("windows_only", func(t *testing.T) {
		entry := config.SubEntry{
			Name:    "test",
			Backup:  "./test",
			Targets: map[string]string{"windows": "~/AppData/Local/test"},
		}
		form := forms.NewSubEntryForm(entry)

		if form.LinuxTargetInput.Value() != "" {
			t.Errorf("LinuxTargetInput = %q, want empty", form.LinuxTargetInput.Value())
		}
		if form.WindowsTargetInput.Value() != "~/AppData/Local/test" {
			t.Errorf("WindowsTargetInput = %q, want %q", form.WindowsTargetInput.Value(), "~/AppData/Local/test")
		}
	})
}

func TestSubEntryForm_Validate(t *testing.T) {
	tests := []struct {
		name    string
		entry   config.SubEntry
		wantErr bool
	}{
		{
			name: "valid_config_entry",
			entry: config.SubEntry{
				Name:    "nvim-config",
				Backup:  "./nvim",
				Targets: map[string]string{"linux": "~/.config/nvim"},
			},
			wantErr: false,
		},
		{
			name: "empty_name_returns_error",
			entry: config.SubEntry{
				Name:    "",
				Backup:  "./nvim",
				Targets: map[string]string{"linux": "~/.config/nvim"},
			},
			wantErr: true,
		},
		{
			name: "whitespace_name_returns_error",
			entry: config.SubEntry{
				Name:    "   ",
				Backup:  "./nvim",
				Targets: map[string]string{"linux": "~/.config/nvim"},
			},
			wantErr: true,
		},
		{
			name: "empty_backup_returns_error",
			entry: config.SubEntry{
				Name:    "nvim-config",
				Backup:  "",
				Targets: map[string]string{"linux": "~/.config/nvim"},
			},
			wantErr: true,
		},
		{
			name: "whitespace_backup_returns_error",
			entry: config.SubEntry{
				Name:    "nvim-config",
				Backup:  "   ",
				Targets: map[string]string{"linux": "~/.config/nvim"},
			},
			wantErr: true,
		},
		{
			name: "no_targets_returns_error",
			entry: config.SubEntry{
				Name:    "nvim-config",
				Backup:  "./nvim",
				Targets: map[string]string{},
			},
			wantErr: true,
		},
		{
			name: "valid_with_both_targets",
			entry: config.SubEntry{
				Name:   "nvim-config",
				Backup: "./nvim",
				Targets: map[string]string{
					"linux":   "~/.config/nvim",
					"windows": "~/AppData/Local/nvim",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := forms.NewSubEntryForm(tt.entry)
			err := form.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSubEntryForm_BuildSubEntry(t *testing.T) {
	t.Run("nil_form_returns_error", func(t *testing.T) {
		var f *forms.SubEntryForm
		_, err := f.BuildSubEntry()
		if err == nil {
			t.Error("BuildSubEntry() on nil form should return error")
		}
	})

	t.Run("empty_name_returns_error", func(t *testing.T) {
		entry := config.SubEntry{
			Name:    "",
			Backup:  "./nvim",
			Targets: map[string]string{"linux": "~/.config/nvim"},
		}
		form := forms.NewSubEntryForm(entry)
		_, err := form.BuildSubEntry()
		if err == nil {
			t.Error("BuildSubEntry() with empty name should return error")
		}
	})

	t.Run("no_targets_returns_error", func(t *testing.T) {
		form := forms.NewSubEntryForm(config.SubEntry{
			Name:   "test",
			Backup: "./test",
		})
		_, err := form.BuildSubEntry()
		if err == nil {
			t.Error("BuildSubEntry() with no targets should return error")
		}
	})

	t.Run("no_backup_returns_error", func(t *testing.T) {
		form := forms.NewSubEntryForm(config.SubEntry{
			Name:    "test",
			Targets: map[string]string{"linux": "~/.config/test"},
		})
		_, err := form.BuildSubEntry()
		if err == nil {
			t.Error("BuildSubEntry() with no backup should return error")
		}
	})

	t.Run("builds_folder_entry", func(t *testing.T) {
		entry := config.SubEntry{
			Name:   "nvim-config",
			Backup: "./nvim",
			Targets: map[string]string{
				"linux": "~/.config/nvim",
			},
		}
		form := forms.NewSubEntryForm(entry)
		// IsFolder=true because no Files

		result, err := form.BuildSubEntry()
		if err != nil {
			t.Fatalf("BuildSubEntry() unexpected error: %v", err)
		}
		if result.Name != "nvim-config" {
			t.Errorf("Name = %q, want %q", result.Name, "nvim-config")
		}
		if result.Backup != "./nvim" {
			t.Errorf("Backup = %q, want %q", result.Backup, "./nvim")
		}
		if result.Targets["linux"] != "~/.config/nvim" {
			t.Errorf("Targets[linux] = %q, want %q", result.Targets["linux"], "~/.config/nvim")
		}
		if len(result.Files) != 0 {
			t.Errorf("len(Files) = %d, want 0 (folder mode)", len(result.Files))
		}
	})

	t.Run("builds_files_entry", func(t *testing.T) {
		entry := config.SubEntry{
			Name:   "nvim-config",
			Backup: "./nvim",
			Files:  []string{"init.lua", "lua/config.lua"},
			Targets: map[string]string{
				"linux": "~/.config/nvim",
			},
		}
		form := forms.NewSubEntryForm(entry)

		result, err := form.BuildSubEntry()
		if err != nil {
			t.Fatalf("BuildSubEntry() unexpected error: %v", err)
		}
		if len(result.Files) != 2 {
			t.Errorf("len(Files) = %d, want 2", len(result.Files))
		}
		if result.Files[0] != "init.lua" {
			t.Errorf("Files[0] = %q, want %q", result.Files[0], "init.lua")
		}
	})

	t.Run("files_mode_with_no_files_returns_error", func(t *testing.T) {
		form := forms.NewSubEntryForm(config.SubEntry{
			Name:    "test",
			Backup:  "./test",
			Targets: map[string]string{"linux": "~/.config/test"},
		})
		// IsFolder=true by default for empty Files; force files mode
		form.IsFolder = false
		form.Files = nil

		_, err := form.BuildSubEntry()
		if err == nil {
			t.Error("BuildSubEntry() in files mode with no files should return error")
		}
	})

	t.Run("sudo_preserved", func(t *testing.T) {
		entry := config.SubEntry{
			Name:    "hosts",
			Backup:  "./system/hosts",
			Sudo:    true,
			Targets: map[string]string{"linux": "/etc/hosts"},
		}
		form := forms.NewSubEntryForm(entry)

		result, err := form.BuildSubEntry()
		if err != nil {
			t.Fatalf("BuildSubEntry() unexpected error: %v", err)
		}
		if !result.Sudo {
			t.Error("Sudo = false, want true")
		}
	})
}

func TestSubEntryForm_GetFieldType(t *testing.T) {
	tests := []struct {
		name       string
		focusIndex int
		isFolder   bool
		wantType   forms.SubEntryFieldType
	}{
		{
			name:       "index_0_is_name",
			focusIndex: 0,
			wantType:   forms.SubFieldName,
		},
		{
			name:       "index_1_is_linux",
			focusIndex: 1,
			wantType:   forms.SubFieldLinux,
		},
		{
			name:       "index_2_is_windows",
			focusIndex: 2,
			wantType:   forms.SubFieldWindows,
		},
		{
			name:       "index_3_is_backup",
			focusIndex: 3,
			wantType:   forms.SubFieldBackup,
		},
		{
			name:       "index_4_is_isFolder",
			focusIndex: 4,
			wantType:   forms.SubFieldIsFolder,
		},
		{
			name:       "index_5_in_folder_mode_is_sudo",
			focusIndex: 5,
			isFolder:   true,
			wantType:   forms.SubFieldIsSudo,
		},
		{
			name:       "index_5_in_files_mode_is_files",
			focusIndex: 5,
			isFolder:   false,
			wantType:   forms.SubFieldFiles,
		},
		{
			name:       "index_6_in_files_mode_is_sudo",
			focusIndex: 6,
			isFolder:   false,
			wantType:   forms.SubFieldIsSudo,
		},
		{
			name:       "out_of_range_defaults_to_name",
			focusIndex: 99,
			wantType:   forms.SubFieldName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := forms.NewSubEntryForm(config.SubEntry{})
			form.FocusIndex = tt.focusIndex
			form.IsFolder = tt.isFolder

			got := form.GetFieldType()
			if got != tt.wantType {
				t.Errorf("GetFieldType() = %v, want %v", got, tt.wantType)
			}
		})
	}

	t.Run("nil_form_returns_name", func(t *testing.T) {
		var f *forms.SubEntryForm
		got := f.GetFieldType()
		if got != forms.SubFieldName {
			t.Errorf("GetFieldType() on nil = %v, want SubFieldName", got)
		}
	})
}

func TestSubEntryForm_MaxIndex(t *testing.T) {
	tests := []struct {
		name      string
		isFolder  bool
		wantIndex int
	}{
		{
			name:      "folder_mode_max_is_5",
			isFolder:  true,
			wantIndex: 5,
		},
		{
			name:      "files_mode_max_is_6",
			isFolder:  false,
			wantIndex: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := forms.NewSubEntryForm(config.SubEntry{})
			form.IsFolder = tt.isFolder

			got := form.MaxIndex()
			if got != tt.wantIndex {
				t.Errorf("MaxIndex() = %d, want %d", got, tt.wantIndex)
			}
		})
	}

	t.Run("nil_form_returns_0", func(t *testing.T) {
		var f *forms.SubEntryForm
		got := f.MaxIndex()
		if got != 0 {
			t.Errorf("MaxIndex() on nil = %d, want 0", got)
		}
	})
}

func TestSubEntryForm_IsTextInputField(t *testing.T) {
	tests := []struct {
		name       string
		focusIndex int
		isFolder   bool
		want       bool
	}{
		{name: "name_is_text_input", focusIndex: 0, want: true},
		{name: "linux_is_text_input", focusIndex: 1, want: true},
		{name: "windows_is_text_input", focusIndex: 2, want: true},
		{name: "backup_is_text_input", focusIndex: 3, want: true},
		{name: "isFolder_is_not_text_input", focusIndex: 4, want: false},
		{name: "files_is_not_text_input", focusIndex: 5, isFolder: false, want: false},
		{name: "sudo_in_folder_mode_is_not_text_input", focusIndex: 5, isFolder: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := forms.NewSubEntryForm(config.SubEntry{})
			form.FocusIndex = tt.focusIndex
			form.IsFolder = tt.isFolder

			got := form.IsTextInputField()
			if got != tt.want {
				t.Errorf("IsTextInputField() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubEntryForm_IsToggleField(t *testing.T) {
	tests := []struct {
		name       string
		focusIndex int
		isFolder   bool
		want       bool
	}{
		{name: "name_is_not_toggle", focusIndex: 0, want: false},
		{name: "linux_is_not_toggle", focusIndex: 1, want: false},
		{name: "isFolder_is_toggle", focusIndex: 4, want: true},
		{name: "sudo_in_folder_mode_is_toggle", focusIndex: 5, isFolder: true, want: true},
		{name: "files_is_not_toggle", focusIndex: 5, isFolder: false, want: false},
		{name: "sudo_in_files_mode_is_toggle", focusIndex: 6, isFolder: false, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := forms.NewSubEntryForm(config.SubEntry{})
			form.FocusIndex = tt.focusIndex
			form.IsFolder = tt.isFolder

			got := form.IsToggleField()
			if got != tt.want {
				t.Errorf("IsToggleField() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSubEntryForm_RoundTripsCheckAndRun is belt-and-braces against a data-loss
// bug: the form has no UI for check/run (setup entries are edited in
// tidydots.yaml), but anything the form does not carry through NewSubEntryForm →
// BuildSubEntry is silently dropped when the caller saves the entry back to the
// config file. The form must preserve the fields it does not display.
//
// The entry below is deliberately hybrid (targets + backup + check + run):
// config validation rejects that shape, and the form is the last place that
// should be quietly "fixing" it by deleting half of it.
func TestSubEntryForm_RoundTripsCheckAndRun(t *testing.T) {
	entry := config.SubEntry{
		Name:    "enable-service",
		Targets: map[string]string{"linux": "~/.config/vicinae"},
		Backup:  "./vicinae",
		Check:   map[string]string{"linux": "systemctl --user is-enabled --quiet vicinae.service"},
		Run:     map[string]string{"linux": "systemctl --user enable --now vicinae.service"},
	}

	form := forms.NewSubEntryForm(entry)

	got, err := form.BuildSubEntry()
	if err != nil {
		t.Fatalf("BuildSubEntry() = %v, want no error", err)
	}

	if got.Check["linux"] != entry.Check["linux"] {
		t.Errorf("check survived as %v, want %v — the form dropped it, which deletes the setup step on save",
			got.Check, entry.Check)
	}

	if got.Run["linux"] != entry.Run["linux"] {
		t.Errorf("run survived as %v, want %v — the form dropped it, which deletes the setup step on save",
			got.Run, entry.Run)
	}
}
