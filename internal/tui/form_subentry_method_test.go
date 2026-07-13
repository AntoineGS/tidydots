package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
)

// copySubEntry returns a config sub-entry deployed with method: copy. Copy mode
// is files-only, so a valid copy entry always declares Files.
func copySubEntry() config.SubEntry {
	return config.SubEntry{
		Name:    "modprobe",
		Backup:  "modprobe.d",
		Targets: map[string]string{"linux": "/etc/modprobe.d"},
		Method:  config.MethodCopy,
		Files:   []string{"nvidia.conf"},
		Sudo:    true,
	}
}

// modelOnDisk writes cfg to a temp tidydots.yaml and returns a Model bound to it,
// so that saving the form exercises the real config.Save write path.
func modelOnDisk(t *testing.T, cfg *config.Config) (*Model, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "tidydots.yaml")
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("seeding config: %v", err)
	}

	m := NewModel(cfg, linuxPlatform(), false)
	m.ConfigPath = path

	return &m, path
}

// subItemIndexByName returns the index of the named sub-entry row.
func subItemIndexByName(t *testing.T, m *Model, appIdx int, name string) int {
	t.Helper()

	for i := range m.Applications[appIdx].SubItems {
		if m.Applications[appIdx].SubItems[i].SubEntry.Name == name {
			return i
		}
	}

	t.Fatalf("no sub-entry named %q", name)

	return -1
}

// TestSaveSubEntryForm_PreservesCopyMethod is the regression guard for a silent
// data-loss bug. method: copy exists for targets read before /home is mounted
// (/etc/modprobe.d, udev rules), where a symlink into the repo cannot resolve.
// The sub-entry form had no Method field, so opening a copy entry with `e` and
// saving rewrote tidydots.yaml *without* `method` — the entry validated clean,
// deployed as a symlink, and the breakage only surfaced at the next boot.
func TestSaveSubEntryForm_PreservesCopyMethod(t *testing.T) {
	cfg := setupOnlyConfig(copySubEntry())
	m, path := modelOnDisk(t, cfg)

	subIdx := subItemIndexByName(t, m, 0, "modprobe")

	m.initSubEntryForm(0, subIdx)
	if m.subEntryForm == nil {
		t.Fatal("the form did not open on a config entry")
	}

	// Save with no edits: a no-op round-trip must not change the entry.
	if err := m.saveSubEntryForm(); err != nil {
		t.Fatalf("saveSubEntryForm() error = %v", err)
	}

	saved, err := config.Load(path)
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}

	entry := saved.Applications[0].Entries[0]
	if entry.Method != config.MethodCopy {
		t.Errorf("method = %q, want %q: copy mode was dropped when the form saved", entry.Method, config.MethodCopy)
	}

	if !entry.IsCopy() {
		t.Error("the entry deploys as a symlink after a no-op edit; it must still be a copy")
	}

	// Guard the file text too: the bug was an omitted key, not a wrong value.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}

	if !strings.Contains(string(raw), "method: copy") {
		t.Errorf("tidydots.yaml has no `method: copy` after the save:\n%s", raw)
	}
}

// TestSaveSubEntryForm_ToggleCopyOnWritesMethod proves the toggle is wired to the
// saved file, not just to the form struct.
func TestSaveSubEntryForm_ToggleCopyOnWritesMethod(t *testing.T) {
	entry := copySubEntry()
	entry.Method = "" // a plain symlink entry

	cfg := setupOnlyConfig(entry)
	m, path := modelOnDisk(t, cfg)

	subIdx := subItemIndexByName(t, m, 0, "modprobe")

	m.initSubEntryForm(0, subIdx)
	if m.subEntryForm == nil {
		t.Fatal("the form did not open on a config entry")
	}

	if m.subEntryForm.IsCopy {
		t.Fatal("IsCopy = true for an entry with no method, want false")
	}

	m.subEntryForm.IsCopy = true

	if err := m.saveSubEntryForm(); err != nil {
		t.Fatalf("saveSubEntryForm() error = %v", err)
	}

	saved, err := config.Load(path)
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}

	if got := saved.Applications[0].Entries[0].Method; got != config.MethodCopy {
		t.Errorf("method = %q, want %q: the toggle did not reach the config file", got, config.MethodCopy)
	}
}
