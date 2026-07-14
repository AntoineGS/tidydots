package tui

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// twoAppModel returns a model with two applications populated directly, the
// way the original selection tests did, so toggles can be exercised without
// touching the filesystem.
func twoAppModel() Model {
	cfg := &config.Config{
		Applications: []config.Application{
			{
				Name: "app1",
				Entries: []config.SubEntry{
					{Name: "config1", Backup: "./config1", Targets: map[string]string{"linux": "~/.config1"}},
					{Name: "config2", Backup: "./config2", Targets: map[string]string{"linux": "~/.config2"}},
				},
			},
			{
				Name: "app2",
				Entries: []config.SubEntry{
					{Name: "config3", Backup: "./config3", Targets: map[string]string{"linux": "~/.config3"}},
				},
			},
		},
	}
	plat := &platform.Platform{
		OS:      "linux",
		EnvVars: map[string]string{"HOME": "/home/test"},
	}

	m := NewModel(cfg, plat, false)
	m.Applications = []ApplicationItem{
		{
			Application: cfg.Applications[0],
			SubItems: []SubEntryItem{
				{SubEntry: cfg.Applications[0].Entries[0]},
				{SubEntry: cfg.Applications[0].Entries[1]},
			},
		},
		{
			Application: cfg.Applications[1],
			SubItems: []SubEntryItem{
				{SubEntry: cfg.Applications[1].Entries[0]},
			},
		},
	}

	return m
}

func TestNewModel_InitializesSelectionState(t *testing.T) {
	cfg := &config.Config{Applications: []config.Application{}}
	plat := &platform.Platform{OS: "linux", EnvVars: map[string]string{"HOME": "/home/test"}}

	m := NewModel(cfg, plat, false)

	if m.selectedApps == nil {
		t.Error("selectedApps map should be initialized")
	}
	if m.selectedSubEntries == nil {
		t.Error("selectedSubEntries map should be initialized")
	}
	if m.multiSelectActive {
		t.Error("multiSelectActive should be false initially")
	}
}

func TestToggleAppSelection(t *testing.T) {
	m := twoAppModel()

	// Toggle app1 on: the app and all its sub-entries select by name.
	m.toggleAppSelection(0)

	if !m.selectedApps["app1"] {
		t.Error("app1 should be selected")
	}
	if !m.selectedSubEntries[subEntryKey{app: "app1", sub: "config1"}] {
		t.Error("app1/config1 should be selected")
	}
	if !m.selectedSubEntries[subEntryKey{app: "app1", sub: "config2"}] {
		t.Error("app1/config2 should be selected")
	}
	if !m.multiSelectActive {
		t.Error("multiSelectActive should be true after selection")
	}

	m.toggleAppSelection(1)

	if !m.selectedApps["app2"] {
		t.Error("app2 should be selected")
	}

	// Toggle app1 off: its keys are removed, app2 keeps the banner active.
	m.toggleAppSelection(0)

	if m.selectedApps["app1"] {
		t.Error("app1 should be deselected")
	}
	if m.selectedSubEntries[subEntryKey{app: "app1", sub: "config1"}] {
		t.Error("app1/config1 should be deselected")
	}
	if !m.multiSelectActive {
		t.Error("multiSelectActive should still be true (app2 is selected)")
	}

	m.toggleAppSelection(1)

	if m.multiSelectActive {
		t.Error("multiSelectActive should be false after all deselections")
	}
}

func TestToggleSubEntrySelection(t *testing.T) {
	m := twoAppModel()

	m.toggleSubEntrySelection(0, 0)

	if !m.selectedSubEntries[subEntryKey{app: "app1", sub: "config1"}] {
		t.Error("app1/config1 should be selected")
	}
	if !m.multiSelectActive {
		t.Error("multiSelectActive should be true after selection")
	}

	m.toggleSubEntrySelection(0, 1)

	if !m.selectedSubEntries[subEntryKey{app: "app1", sub: "config2"}] {
		t.Error("app1/config2 should be selected")
	}

	m.toggleSubEntrySelection(0, 0)

	if m.selectedSubEntries[subEntryKey{app: "app1", sub: "config1"}] {
		t.Error("app1/config1 should be deselected")
	}
	if !m.multiSelectActive {
		t.Error("multiSelectActive should still be true (config2 is selected)")
	}

	m.toggleSubEntrySelection(0, 1)

	if m.multiSelectActive {
		t.Error("multiSelectActive should be false after all deselections")
	}
}

func TestClearSelections(t *testing.T) {
	m := twoAppModel()
	m.toggleAppSelection(0)
	m.toggleSubEntrySelection(1, 0)

	m.clearSelections()

	if len(m.selectedApps) != 0 {
		t.Error("selectedApps should be empty after clearing")
	}
	if len(m.selectedSubEntries) != 0 {
		t.Error("selectedSubEntries should be empty after clearing")
	}
	if m.multiSelectActive {
		t.Error("multiSelectActive should be false after clearing")
	}
}

func TestIsAppSelected(t *testing.T) {
	m := twoAppModel()

	if m.isAppSelected("app1") {
		t.Error("app1 should not be selected initially")
	}

	m.toggleAppSelection(0)

	if !m.isAppSelected("app1") {
		t.Error("app1 should be selected after toggleAppSelection")
	}

	m.toggleAppSelection(0)

	if m.isAppSelected("app1") {
		t.Error("app1 should be deselected after second toggleAppSelection")
	}
}

func TestIsSubEntrySelected(t *testing.T) {
	m := twoAppModel()

	if m.isSubEntrySelected("app1", "config1") {
		t.Error("app1/config1 should not be selected initially")
	}

	m.toggleSubEntrySelection(0, 0)

	if !m.isSubEntrySelected("app1", "config1") {
		t.Error("app1/config1 should be selected after toggle")
	}

	m.toggleSubEntrySelection(0, 0)

	if m.isSubEntrySelected("app1", "config1") {
		t.Error("app1/config1 should be deselected after second toggle")
	}

	// Implicit selection through the parent app.
	m.toggleAppSelection(0)

	if !m.isSubEntrySelected("app1", "config1") {
		t.Error("app1/config1 should be implicitly selected when app1 is selected")
	}
	if !m.isSubEntrySelected("app1", "config2") {
		t.Error("app1/config2 should be implicitly selected when app1 is selected")
	}
	if m.isSubEntrySelected("app2", "config3") {
		t.Error("app2/config3 must not inherit app1's selection")
	}
}

func TestGetSelectionCounts(t *testing.T) {
	m := twoAppModel()

	// One whole app: 1 app, 0 independent sub-entries.
	m.toggleAppSelection(0)

	appCount, subCount := m.getSelectionCounts()
	if appCount != 1 || subCount != 0 {
		t.Errorf("counts = (%d, %d), want (1, 0)", appCount, subCount)
	}

	// Plus one sub-entry whose parent is NOT selected: it counts.
	m.toggleSubEntrySelection(1, 0)

	appCount, subCount = m.getSelectionCounts()
	if appCount != 1 || subCount != 1 {
		t.Errorf("counts = (%d, %d), want (1, 1)", appCount, subCount)
	}
}

func TestCountHiddenSelections(t *testing.T) {
	m := Model{
		Applications: []ApplicationItem{
			{
				Application: config.Application{Name: "app1"},
				IsFiltered:  false,
				SubItems:    []SubEntryItem{{SubEntry: config.SubEntry{Name: "sub1"}}},
			},
			{
				Application: config.Application{Name: "app2"},
				IsFiltered:  true,
				SubItems:    []SubEntryItem{{SubEntry: config.SubEntry{Name: "sub2"}}},
			},
			{
				Application: config.Application{Name: "app3"},
				IsFiltered:  true,
				SubItems:    []SubEntryItem{{SubEntry: config.SubEntry{Name: "sub3"}}},
			},
		},
		selectedApps:       make(map[string]bool),
		selectedSubEntries: make(map[subEntryKey]bool),
	}

	t.Run("counts selected filtered apps", func(t *testing.T) {
		m.selectedApps["app2"] = true
		m.selectedApps["app3"] = true

		if count := m.countHiddenSelections(); count != 2 {
			t.Errorf("expected 2 hidden selections, got %d", count)
		}
	})

	t.Run("counts selected sub-entries under filtered apps", func(t *testing.T) {
		m.selectedApps = make(map[string]bool)
		m.selectedSubEntries[subEntryKey{app: "app2", sub: "sub2"}] = true

		if count := m.countHiddenSelections(); count != 1 {
			t.Errorf("expected 1 hidden selection, got %d", count)
		}
	})

	t.Run("ignores selections under visible apps", func(t *testing.T) {
		m.selectedApps = map[string]bool{"app1": true}
		m.selectedSubEntries = make(map[subEntryKey]bool)

		if count := m.countHiddenSelections(); count != 0 {
			t.Errorf("expected 0 hidden selections, got %d", count)
		}
	})
}

func TestClearHiddenSelections(t *testing.T) {
	m := Model{
		Applications: []ApplicationItem{
			{
				Application: config.Application{Name: "app1"},
				IsFiltered:  false,
				SubItems:    []SubEntryItem{{SubEntry: config.SubEntry{Name: "sub1"}}},
			},
			{
				Application: config.Application{Name: "app2"},
				IsFiltered:  true,
				SubItems:    []SubEntryItem{{SubEntry: config.SubEntry{Name: "sub2"}}},
			},
		},
		selectedApps: map[string]bool{"app1": true, "app2": true},
		selectedSubEntries: map[subEntryKey]bool{
			{app: "app1", sub: "sub1"}: true,
			{app: "app2", sub: "sub2"}: true,
		},
		multiSelectActive: true,
	}

	m.clearHiddenSelections()

	if !m.selectedApps["app1"] {
		t.Error("app1 (visible) should remain selected")
	}
	if m.selectedApps["app2"] {
		t.Error("app2 (filtered) should be deselected")
	}
	if !m.selectedSubEntries[subEntryKey{app: "app1", sub: "sub1"}] {
		t.Error("sub-entry under visible app should remain selected")
	}
	if m.selectedSubEntries[subEntryKey{app: "app2", sub: "sub2"}] {
		t.Error("sub-entry under filtered app should be deselected")
	}
	if !m.multiSelectActive {
		t.Error("multiSelectActive should remain true (visible app still selected)")
	}
}
