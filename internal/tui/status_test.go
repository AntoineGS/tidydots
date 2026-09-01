package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AntoineGS/tidydots/internal/cmdexec"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
)

func TestComputeStatusResolvesChecksAndUsesActionFilter(t *testing.T) {
	repo := t.TempDir()
	linkedBackup := filepath.Join(repo, "linked")
	readyBackup := filepath.Join(repo, "ready")
	if err := os.MkdirAll(linkedBackup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(readyBackup, 0o755); err != nil {
		t.Fatal(err)
	}

	linkedTarget := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(linkedBackup, linkedTarget); err != nil {
		t.Fatal(err)
	}

	setup := config.SubEntry{
		Name:  "setup",
		Check: map[string]string{"linux": "false"},
		Run:   map[string]string{"linux": "true"},
	}
	cfg := &config.Config{
		Version:    3,
		BackupRoot: repo,
		Applications: []config.Application{{
			Name:        "tool",
			Description: "Tool description",
			Package:     &config.EntryPackage{Custom: map[string]string{"linux": "install tool"}},
			Entries: []config.SubEntry{
				{Name: "linked", Backup: "linked", Targets: map[string]string{"linux": linkedTarget}},
				{Name: "ready", Backup: "ready", Targets: map[string]string{"linux": filepath.Join(t.TempDir(), "ready")}},
				setup,
			},
		}},
	}
	plat := linuxPlatform()

	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1})
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1})
	mgr := manager.New(cfg, plat).WithRunner(stub)

	all, err := ComputeStatus(cfg, plat, mgr, false)
	if err != nil {
		t.Fatalf("ComputeStatus() error = %v", err)
	}
	if !all.Actionable || all.ActionsOnly {
		t.Fatalf("report flags = actionable:%t actions_only:%t, want true:false", all.Actionable, all.ActionsOnly)
	}
	if all.Counts.Applications != 1 || all.Counts.Entries != 3 || all.Counts.Packages != 1 {
		t.Fatalf("total counts = %+v, want 1 application, 3 entries, 1 package", all.Counts)
	}
	if all.Counts.ActionableApplications != 1 || all.Counts.ActionableEntries != 2 || all.Counts.ActionablePackages != 1 {
		t.Fatalf("action counts = %+v, want 1 application, 2 entries, 1 package", all.Counts)
	}
	if len(all.Applications) != 1 || len(all.Applications[0].Entries) != 3 {
		t.Fatalf("all details = %+v, want one application with three entries", all.Applications)
	}
	if got := all.Applications[0].Entries[0].State; got != StateLinked.String() {
		t.Errorf("linked entry state = %q, want %q", got, StateLinked.String())
	}
	if got := all.Applications[0].Entries[1].State; got != StateReady.String() {
		t.Errorf("ready entry state = %q, want %q", got, StateReady.String())
	}
	if got := all.Applications[0].Entries[2].State; got != StateSetupNeeded.String() {
		t.Errorf("setup entry state = %q, want %q", got, StateSetupNeeded.String())
	}

	actions, err := ComputeStatus(cfg, plat, mgr, true)
	if err != nil {
		t.Fatalf("ComputeStatus(actionsOnly) error = %v", err)
	}
	if !actions.ActionsOnly || !actions.Actionable {
		t.Fatalf("action report flags = actionable:%t actions_only:%t, want true:true", actions.Actionable, actions.ActionsOnly)
	}
	if len(actions.Applications) != 1 || len(actions.Applications[0].Entries) != 2 {
		t.Fatalf("action details = %+v, want one application with two entries", actions.Applications)
	}
	if actions.Applications[0].Entries[0].Name != "ready" || actions.Applications[0].Entries[1].Name != "setup" {
		t.Fatalf("action entries = %+v, want ready and setup", actions.Applications[0].Entries)
	}
}

func TestNewModelWithActionFilterEnablesFilterBeforeChecksSettle(t *testing.T) {
	cfg := &config.Config{
		Version: 3,
		Applications: []config.Application{{
			Name:    "tool",
			Entries: []config.SubEntry{{Name: "config", Backup: "config", Targets: map[string]string{"linux": t.TempDir()}}},
		}},
	}

	model := NewModelWithActionFilter(cfg, linuxPlatform(), false, true)
	if !model.actionFilterEnabled {
		t.Fatal("action filter was not enabled at startup")
	}
	if model.pendingStateChecks != 1 {
		t.Fatalf("pending state checks = %d, want 1", model.pendingStateChecks)
	}
	if len(model.tableRows) != 1 || model.tableRows[0].AppName != "tool" {
		t.Fatalf("startup action-filter rows = %+v, want the loading application row", model.tableRows)
	}
}
