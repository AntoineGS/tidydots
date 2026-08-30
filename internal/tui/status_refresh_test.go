package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/cmdexec"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/AntoineGS/tidydots/internal/platform"
)

func TestRefreshAllStatesDispatchesEveryCheckAndPreservesUIState(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0})

	configEntry := config.SubEntry{
		Name:    "config",
		Backup:  "./config",
		Targets: map[string]string{"linux": t.TempDir()},
	}
	setupEntry := setupSubEntry()
	packageConfig := &config.EntryPackage{Managers: map[string]config.ManagerValue{
		"git": {Git: &config.GitPackage{
			URL:     "https://example.com/tool.git",
			Targets: map[string]string{"linux": t.TempDir()},
		}},
	}}
	cfg := &config.Config{Version: 3, BackupRoot: "/repo", Applications: []config.Application{
		{Name: "a-visible", Package: packageConfig, Entries: []config.SubEntry{configEntry}},
		{Name: "z-hidden", When: `{{ eq .OS "windows" }}`, Entries: []config.SubEntry{setupEntry}},
	}}
	plat := &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}
	m := NewModel(cfg, plat, false)
	m.Manager = manager.New(cfg, plat).WithRunner(stub)
	m.pendingStateChecks = 0
	for i := range m.Applications {
		m.Applications[i].PkgMethod = TypeGit
		installed := true
		m.Applications[i].PkgInstalled = &installed
		for j := range m.Applications[i].SubItems {
			m.Applications[i].SubItems[j].State = StateLinked
		}
	}

	m.tableCursor = 1
	m.scrollOffset = 1
	m.sortColumn = SortColumnStatus
	m.sortAscending = false
	m.searchText = "a-visible"
	m.filterEnabled = false
	m.actionFilterEnabled = true
	m.Applications[0].Expanded = true
	m.selectedApps["a-visible"] = true
	m.selectedSubEntries[subEntryKey{app: "a-visible", sub: "config"}] = true
	m.multiSelectActive = true
	m.rebuildTable()

	wantCursor := m.tableCursor
	wantScroll := m.scrollOffset
	wantSort := m.sortColumn
	wantAscending := m.sortAscending
	wantSearch := m.searchText
	wantFilter := m.filterEnabled
	wantActionFilter := m.actionFilterEnabled
	wantExpanded := m.Applications[0].Expanded
	wantApps := mapsClone(m.selectedApps)
	wantSubEntries := mapsClone(m.selectedSubEntries)
	wantMultiSelect := m.multiSelectActive

	cmd := m.refreshAllStates()

	if got := m.pendingStateChecks; got != 3 {
		t.Fatalf("pendingStateChecks = %d, want 3", got)
	}
	if m.Applications[0].PkgInstalled != nil {
		t.Fatal("package status was not invalidated")
	}
	for _, app := range m.Applications {
		for _, sub := range app.SubItems {
			if sub.State != StateLoading {
				t.Fatalf("sub-entry %q state = %v, want loading", sub.SubEntry.Name, sub.State)
			}
		}
	}
	if len(collectMsgs(cmd)) != 3 {
		t.Fatal("refresh did not dispatch all hidden package and sub-entry checks")
	}

	if m.tableCursor != wantCursor || m.scrollOffset != wantScroll || m.sortColumn != wantSort ||
		m.sortAscending != wantAscending || m.searchText != wantSearch || m.filterEnabled != wantFilter ||
		m.actionFilterEnabled != wantActionFilter || m.Applications[0].Expanded != wantExpanded ||
		!reflect.DeepEqual(m.selectedApps, wantApps) || !reflect.DeepEqual(m.selectedSubEntries, wantSubEntries) ||
		m.multiSelectActive != wantMultiSelect {
		t.Fatal("refresh did not preserve navigation, filter, expansion, and selection state")
	}
}

func TestUpdateResultsCtrlRRefreshesStatuses(t *testing.T) {
	pkg := &config.EntryPackage{Managers: map[string]config.ManagerValue{
		"git": {Git: &config.GitPackage{URL: "https://example.com/tool.git", Targets: map[string]string{"linux": t.TempDir()}}},
	}}
	m := newStatusRefreshModel(t, pkg)
	installed := true
	m.Applications[0].PkgInstalled = &installed

	next, cmd := m.updateResults(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	refreshed := next.(Model)
	if cmd == nil {
		t.Fatal("ctrl+r returned a nil refresh command")
	}
	if refreshed.Applications[0].PkgInstalled != nil {
		t.Fatal("ctrl+r did not invalidate package status")
	}
}

func TestUpdateResultsCtrlRIgnoredWhenBusy(t *testing.T) {
	cases := []struct {
		name string
		busy func(*Model)
	}{
		{name: "pending checks", busy: func(m *Model) { m.pendingStateChecks = 1 }},
		{name: "processing", busy: func(m *Model) { m.processing = true }},
		{name: "searching", busy: func(m *Model) { m.searching = true }},
		{name: "showing results", busy: func(m *Model) { m.showingResults = true }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkg := &config.EntryPackage{Managers: map[string]config.ManagerValue{
				"git": {Git: &config.GitPackage{URL: "https://example.com/tool.git", Targets: map[string]string{"linux": t.TempDir()}}},
			}}
			m := newStatusRefreshModel(t, pkg)
			installed := true
			m.Applications[0].PkgInstalled = &installed
			tc.busy(&m)

			next, cmd := m.updateResults(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
			got := next.(Model)
			if cmd != nil {
				t.Fatal("busy ctrl+r returned a command")
			}
			if got.Applications[0].PkgInstalled == nil {
				t.Fatal("busy ctrl+r changed package status")
			}
		})
	}
}

func TestRefreshHelp(t *testing.T) {
	m := Model{width: 80, Operation: OpList, Screen: ScreenResults}
	if help := m.renderHelpForCurrentState(); !strings.Contains(help, "ctrl+r") || !strings.Contains(help, "refresh") {
		t.Fatalf("help = %q, want refresh binding", help)
	}

	m.multiSelectActive = true
	if help := m.renderHelpForCurrentState(); !strings.Contains(help, "ctrl+r") || !strings.Contains(help, "refresh") {
		t.Fatalf("multi-select help = %q, want refresh binding", help)
	}
}

func mapsClone[K comparable, V any](in map[K]V) map[K]V {
	out := make(map[K]V, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func TestPackageInstallCompletionRechecksDetectedState(t *testing.T) {
	target := t.TempDir()
	pkg := &config.EntryPackage{Managers: map[string]config.ManagerValue{
		"git": {Git: &config.GitPackage{
			URL:     "https://example.com/tool.git",
			Targets: map[string]string{"linux": target},
		}},
	}}
	m := newStatusRefreshModel(t, pkg)
	installed := true
	m.Applications[0].PkgInstalled = &installed
	m.pendingPackages = []PackageItem{{Name: "tool", Package: pkg, Method: TypeGit}}

	next, cmd := m.Update(PackageInstallMsg{
		Package: m.pendingPackages[0], Success: true, Message: "Installed via git",
	})
	refreshing := next.(Model)
	if refreshing.Applications[0].PkgInstalled != nil {
		t.Fatal("package status was not invalidated before re-detection")
	}
	if refreshing.pendingStateChecks != 1 || cmd == nil {
		t.Fatalf("pending checks = %d, cmd nil = %v; want 1, false", refreshing.pendingStateChecks, cmd == nil)
	}

	msgs := collectMsgs(cmd)
	checked, _ := refreshing.Update(msgs[0])
	final := checked.(Model)
	if final.Applications[0].PkgInstalled == nil || *final.Applications[0].PkgInstalled {
		t.Fatal("installer success overrode the detector's not-installed result")
	}
}

func TestBatchPackageInstallCompletionRechecksEveryPendingPackage(t *testing.T) {
	firstTarget := t.TempDir()
	secondTarget := t.TempDir()
	firstPackage := &config.EntryPackage{Managers: map[string]config.ManagerValue{
		"git": {Git: &config.GitPackage{
			URL:     "https://example.com/first.git",
			Targets: map[string]string{"linux": firstTarget},
		}},
	}}
	secondPackage := &config.EntryPackage{Managers: map[string]config.ManagerValue{
		"git": {Git: &config.GitPackage{
			URL:     "https://example.com/second.git",
			Targets: map[string]string{"linux": secondTarget},
		}},
	}}
	m := NewModel(&config.Config{Version: 3, Applications: []config.Application{
		{Name: "first", Package: firstPackage},
		{Name: "second", Package: secondPackage},
	}}, &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}, true)
	m.pendingStateChecks = 0
	for i := range m.Applications {
		m.Applications[i].PkgMethod = TypeGit
		installed := true
		m.Applications[i].PkgInstalled = &installed
	}
	m.pendingPackages = []PackageItem{
		{Name: "first", Package: firstPackage, Method: TypeGit},
		{Name: "second", Package: secondPackage, Method: TypeGit},
	}

	next, firstCmd := m.Update(PackageInstallMsg{
		Package: m.pendingPackages[0], Success: true, Message: "Installed via git",
	})
	firstRefreshing := next.(Model)
	if firstCmd == nil || firstRefreshing.pendingStateChecks != 0 {
		t.Fatalf("first completion started refresh: cmd nil = %v, pending checks = %d; want false, 0", firstCmd == nil, firstRefreshing.pendingStateChecks)
	}

	next, finalRefreshCmd := firstRefreshing.Update(PackageInstallMsg{
		Package: firstRefreshing.pendingPackages[1], Success: true, Message: "Installed via git",
	})
	refreshing := next.(Model)
	if refreshing.pendingStateChecks != 2 || finalRefreshCmd == nil {
		t.Fatalf("pending checks = %d, cmd nil = %v; want 2, false", refreshing.pendingStateChecks, finalRefreshCmd == nil)
	}
	msgs := collectMsgs(finalRefreshCmd)
	if len(msgs) != 2 {
		t.Fatalf("refresh messages = %d, want 2", len(msgs))
	}
	for i := range refreshing.Applications {
		if refreshing.Applications[i].PkgInstalled != nil {
			t.Fatalf("application %d retained stale package status", i)
		}
	}
}

func newStatusRefreshModel(t *testing.T, pkg *config.EntryPackage) Model {
	t.Helper()
	m := NewModel(&config.Config{Version: 3, Applications: []config.Application{{
		Name: "tool", Package: pkg,
	}}}, &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}, false)
	m.pendingStateChecks = 0
	m.Applications[0].PkgMethod = TypeGit
	return m
}
