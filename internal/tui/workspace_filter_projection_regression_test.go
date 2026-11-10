package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

func TestWorkspaceMachineFilterProjectsInactiveOwnerRowsAcrossToggleAndRefresh(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	public, private := t.TempDir(), t.TempDir()
	configs := map[string]*config.Config{
		public:  {Version: 3, Applications: []config.Application{{Name: "shell", Entries: []config.SubEntry{{Name: "active", Backup: "./shell", Files: []string{"active"}, Targets: map[string]string{"linux": t.TempDir()}}}}}},
		private: {Version: 3, Applications: []config.Application{{Name: "shell", When: "false", Entries: []config.SubEntry{{Name: "inactive", Backup: "./shell", Files: []string{"inactive"}, Targets: map[string]string{"linux": t.TempDir()}}}}}},
	}
	for root, cfg := range configs {
		if err := config.Save(cfg, root+"/tidydots.yaml"); err != nil {
			t.Fatal(err)
		}
	}
	w, err := workspace.Load(public, private)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.pendingStateChecks = 0
	m.Operation, m.Screen = OpList, ScreenResults
	m.Applications[0].Expanded = true
	m.rebuildTable()
	assertInactiveVisible := func(want bool) {
		t.Helper()
		found := false
		for _, row := range m.tableRows {
			found = found || row.SubName == "inactive"
		}
		if found != want {
			t.Fatalf("inactive row visible = %v, want %v", found, want)
		}
	}
	assertInactiveVisible(false)
	next, _ := m.updateResults(tea.KeyPressMsg{Code: 'f'})
	m = next.(Model)
	assertInactiveVisible(true)
	if !m.Applications[0].SubItems[1].Filtered {
		t.Fatal("inactive entry lost its operational filter marker")
	}
	if err := m.refreshWorkspaceForInspection(nil, ""); err != nil {
		t.Fatal(err)
	}
	assertInactiveVisible(true)
	next, _ = m.updateResults(tea.KeyPressMsg{Code: 'f'})
	m = next.(Model)
	assertInactiveVisible(false)
}
