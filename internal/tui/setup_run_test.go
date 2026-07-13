package tui

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// linuxPlatform is the platform used across the setup-entry TUI tests.
func linuxPlatform() *platform.Platform {
	return &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}
}

// configSubEntry returns a config sub-entry (has a Backup and a target).
func configSubEntry() config.SubEntry {
	return config.SubEntry{
		Name:    "config-file",
		Backup:  "vicinae",
		Targets: map[string]string{"linux": "~/.config/vicinae"},
	}
}

// windowsOnlySetupSubEntry returns a setup entry that does not apply to Linux.
func windowsOnlySetupSubEntry() config.SubEntry {
	return config.SubEntry{
		Name:  "windows-only",
		Check: map[string]string{"windows": "check"},
		Run:   map[string]string{"windows": "run"},
	}
}

// setupOnlyConfig builds a config whose single application holds the given entries.
func setupOnlyConfig(entries ...config.SubEntry) *config.Config {
	return &config.Config{
		Version:    3,
		BackupRoot: "/repo",
		Applications: []config.Application{
			{Name: "vicinae", Entries: entries},
		},
	}
}

// TestInitApplicationItems_IncludesSetupEntries is the regression guard for the
// bug that made the whole setup feature invisible in the TUI: setup entries
// declare no targets (they are actions, not file deployments), and the list
// builder skipped every sub-entry whose target was empty. The rows the TUI is
// supposed to flag as "Needs setup" never existed.
func TestInitApplicationItems_IncludesSetupEntries(t *testing.T) {
	cfg := setupOnlyConfig(configSubEntry(), setupSubEntry())

	m := NewModel(cfg, linuxPlatform(), false)

	if len(m.Applications) != 1 {
		t.Fatalf("Applications = %d, want 1", len(m.Applications))
	}

	subs := m.Applications[0].SubItems
	if len(subs) != 2 {
		t.Fatalf("SubItems = %d, want 2 (the config entry and the setup entry)", len(subs))
	}

	if !subs[1].SubEntry.IsSetup() {
		t.Fatalf("second sub-item is %q, want the setup entry", subs[1].SubEntry.Name)
	}

	if subs[1].Target != "" {
		t.Errorf("setup entry Target = %q, want \"\" (a setup entry deploys no files)", subs[1].Target)
	}

	if subs[1].State != StateLoading {
		t.Errorf("setup entry initial State = %v, want StateLoading (resolved asynchronously by its check)", subs[1].State)
	}
}

// TestInitApplicationItems_SetupOnlyApp_IsNotDropped covers an application that
// holds nothing but setup entries and has no package: it must still be listed.
func TestInitApplicationItems_SetupOnlyApp_IsNotDropped(t *testing.T) {
	m := NewModel(setupOnlyConfig(setupSubEntry()), linuxPlatform(), false)

	if len(m.Applications) != 1 {
		t.Fatalf("Applications = %d, want 1 (an app with only setup entries is still an app)", len(m.Applications))
	}

	if len(m.Applications[0].SubItems) != 1 {
		t.Fatalf("SubItems = %d, want 1", len(m.Applications[0].SubItems))
	}
}

// TestInitApplicationItems_SkipsSetupEntryForOtherOS mirrors runSetupEntry,
// which skips an entry with no run command for the current OS. Listing such an
// entry would report it as "Set up" (its check is absent, so IsSetupApplied
// reports nothing outstanding), which is a claim about a machine it never
// applied to.
func TestInitApplicationItems_SkipsSetupEntryForOtherOS(t *testing.T) {
	m := NewModel(setupOnlyConfig(configSubEntry(), windowsOnlySetupSubEntry()), linuxPlatform(), false)

	if len(m.Applications) != 1 {
		t.Fatalf("Applications = %d, want 1", len(m.Applications))
	}

	subs := m.Applications[0].SubItems
	if len(subs) != 1 {
		t.Fatalf("SubItems = %d, want 1 (the Windows-only setup entry must not be listed on Linux)", len(subs))
	}

	if subs[0].SubEntry.Name != "config-file" {
		t.Errorf("listed sub-entry = %q, want the config entry", subs[0].SubEntry.Name)
	}
}

// TestCountInitialStateChecks_CountsSetupEntries keeps the pre-computed
// pendingStateChecks in step with what Init() actually dispatches: the counter
// mirrors checkSubEntryStatesCmd, which now sees setup entries too. A drift
// here leaves the table rebuilding early or never.
func TestCountInitialStateChecks_CountsSetupEntries(t *testing.T) {
	m := NewModel(setupOnlyConfig(configSubEntry(), setupSubEntry()), linuxPlatform(), false)

	_, dispatched := m.checkSubEntryStatesCmd()
	if got := m.countInitialStateChecks(); got != dispatched {
		t.Errorf("countInitialStateChecks() = %d, but checkSubEntryStatesCmd dispatches %d", got, dispatched)
	}

	if dispatched != 2 {
		t.Errorf("checkSubEntryStatesCmd dispatched %d check(s), want 2 (config entry + setup entry)", dispatched)
	}
}

// TestGetTypeInfo_SetupEntry_ReportsSetup covers the info column: a setup entry
// has no Files and no Backup, so the file-count logic described it as "0 files".
func TestGetTypeInfo_SetupEntry_ReportsSetup(t *testing.T) {
	got := getTypeInfo(SubEntryItem{AppName: "vicinae", SubEntry: setupSubEntry()})
	if got != TypeSetup {
		t.Errorf("getTypeInfo(setup entry) = %q, want %q", got, TypeSetup)
	}
}
