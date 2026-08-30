package tui

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

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
