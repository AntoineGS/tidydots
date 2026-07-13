package forms_test

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/tui/forms"
)

// copyEntry is a valid config sub-entry deployed with method: copy. Copy mode is
// files-only — config validation rejects a copy entry with no files list — so a
// valid fixture always carries Files.
func copyEntry() config.SubEntry {
	return config.SubEntry{
		Name:    "modprobe",
		Targets: map[string]string{"linux": "/etc/modprobe.d"},
		Backup:  "./Linux/modprobe.d",
		Method:  config.MethodCopy,
		Files:   []string{"nvidia.conf"},
	}
}

// TestSubEntryForm_CopyMethodSurvivesRoundTrip guards a data-loss path.
// method: copy exists because some targets (/etc/modprobe.d, udev rules) are
// read before /home mounts, so a symlink into the repo cannot resolve. The form
// had no Method field at all, so editing such an entry rewrote it to
// tidydots.yaml without `method`, silently downgrading it to a symlink — the
// entry still validated clean, and the breakage only surfaced at next boot.
func TestSubEntryForm_CopyMethodSurvivesRoundTrip(t *testing.T) {
	form := forms.NewSubEntryForm(copyEntry())

	if !form.IsCopy {
		t.Fatal("IsCopy = false, want true: form did not read method: copy off the entry")
	}

	got, err := form.BuildSubEntry()
	if err != nil {
		t.Fatalf("BuildSubEntry() error = %v", err)
	}

	if got.Method != config.MethodCopy {
		t.Errorf("Method = %q, want %q: copy mode was dropped on save", got.Method, config.MethodCopy)
	}

	if !got.IsCopy() {
		t.Error("IsCopy() = false: entry was silently downgraded to a symlink")
	}
}

// TestSubEntryForm_ToggleCopyOn covers the other half: a symlink entry the user
// switches to copy in the TUI must come back out carrying the method.
func TestSubEntryForm_ToggleCopyOn(t *testing.T) {
	entry := copyEntry()
	entry.Method = ""

	form := forms.NewSubEntryForm(entry)
	if form.IsCopy {
		t.Fatal("IsCopy = true for an entry with no method, want false")
	}

	form.IsCopy = true

	got, err := form.BuildSubEntry()
	if err != nil {
		t.Fatalf("BuildSubEntry() error = %v", err)
	}

	if got.Method != config.MethodCopy {
		t.Errorf("Method = %q, want %q", got.Method, config.MethodCopy)
	}
}

// TestSubEntryForm_ToggleCopyOff verifies turning copy off omits the method
// rather than writing method: symlink, which is the default anyway.
func TestSubEntryForm_ToggleCopyOff(t *testing.T) {
	form := forms.NewSubEntryForm(copyEntry())
	form.IsCopy = false

	got, err := form.BuildSubEntry()
	if err != nil {
		t.Fatalf("BuildSubEntry() error = %v", err)
	}

	if got.Method != "" {
		t.Errorf("Method = %q, want empty (symlink is the default; do not write it out)", got.Method)
	}
}

// TestSubEntryForm_ExplicitSymlinkPreserved verifies an entry that spells out
// method: symlink keeps it. The form must not normalize the user's file.
func TestSubEntryForm_ExplicitSymlinkPreserved(t *testing.T) {
	entry := copyEntry()
	entry.Method = config.MethodSymlink

	form := forms.NewSubEntryForm(entry)
	if form.IsCopy {
		t.Fatal("IsCopy = true for method: symlink, want false")
	}

	got, err := form.BuildSubEntry()
	if err != nil {
		t.Fatalf("BuildSubEntry() error = %v", err)
	}

	if got.Method != config.MethodSymlink {
		t.Errorf("Method = %q, want %q: an explicit method was rewritten", got.Method, config.MethodSymlink)
	}
}

// copyFieldReachable reports whether any focus index maps to the copy toggle.
func copyFieldReachable(form *forms.SubEntryForm) bool {
	for i := 0; i <= form.MaxIndex(); i++ {
		form.FocusIndex = i
		if form.GetFieldType() == forms.SubFieldIsCopy {
			return true
		}
	}

	return false
}

// TestSubEntryForm_CopyToggleReachableInFilesMode verifies the toggle has a focus
// index. Carrying Method through invisibly would fix the data loss but leave copy
// mode unsettable in the TUI, so the field must be navigable.
func TestSubEntryForm_CopyToggleReachableInFilesMode(t *testing.T) {
	form := forms.NewSubEntryForm(copyEntry())
	if form.IsFolder {
		t.Fatal("a copy entry with files is in folder mode; the fixture is wrong")
	}

	if !copyFieldReachable(form) {
		t.Fatalf("SubFieldIsCopy is unreachable: no focus index in 0..%d maps to it", form.MaxIndex())
	}

	if !form.IsToggleField() {
		t.Error("IsToggleField() = false at the copy field, want true")
	}
}

// TestSubEntryForm_CopyToggleAbsentInFolderMode pins copy mode to files mode.
// Config validation rejects a copy entry with no files list, so offering the
// toggle in folder mode would only let the user build an unloadable config.
func TestSubEntryForm_CopyToggleAbsentInFolderMode(t *testing.T) {
	form := forms.NewSubEntryForm(copyEntry())
	form.IsFolder = true

	if copyFieldReachable(form) {
		t.Error("the copy toggle is reachable in folder mode; copy mode is files-only")
	}
}

// TestSubEntryForm_ToggleFolderModeClearsCopy guards the transition. Switching a
// copy entry to folder mode must drop copy with it: config.Save does not
// validate, so building copy-without-files would write a tidydots.yaml that no
// longer loads.
func TestSubEntryForm_ToggleFolderModeClearsCopy(t *testing.T) {
	form := forms.NewSubEntryForm(copyEntry())
	if !form.IsCopy {
		t.Fatal("fixture is not in copy mode")
	}

	form.ToggleFolderMode()

	if !form.IsFolder {
		t.Fatal("ToggleFolderMode() did not enter folder mode")
	}

	if form.IsCopy {
		t.Error("IsCopy survived the switch to folder mode: the form would build an invalid entry")
	}
}

// TestSubEntryForm_BuildRejectsCopyInFolderMode is the last line of defense. The
// UI keeps the toggle out of folder mode; if any other path sets both, the build
// must fail loudly rather than emit a config that cannot be loaded back.
func TestSubEntryForm_BuildRejectsCopyInFolderMode(t *testing.T) {
	form := forms.NewSubEntryForm(copyEntry())
	form.IsFolder = true
	form.IsCopy = true

	_, err := form.BuildSubEntry()
	if err == nil {
		t.Fatal("BuildSubEntry() succeeded with copy + folder mode; it must reject an unloadable entry")
	}
}
