package tui

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/cmdexec"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// setupSubEntry returns a canonical setup sub-entry (Run set, no Backup) for
// use across these tests. Setup entries never have a Backup; detection must
// not fall through to the config-state logic that expects one.
func setupSubEntry() config.SubEntry {
	return config.SubEntry{
		Name:  "enable-service",
		Check: map[string]string{"linux": "systemctl --user is-enabled --quiet vicinae.service"},
		Run:   map[string]string{"linux": "systemctl --user enable --now vicinae.service"},
	}
}

// newStubManager builds a Manager on Linux backed by the given stub runner.
func newStubManager(stub *cmdexec.StubRunner) *manager.Manager {
	cfg := &config.Config{Version: 3, BackupRoot: "/repo"}
	plat := &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}

	return manager.New(cfg, plat).WithRunner(stub)
}

func TestDetectSetupPathState_NilManager_ReturnsSetupOk(t *testing.T) {
	got := detectSetupPathState(setupSubEntry(), nil)
	if got != StateSetupOk {
		t.Errorf("detectSetupPathState with nil manager = %v, want StateSetupOk (a nil manager cannot run the check, so it must not falsely flag the entry)", got)
	}
}

func TestDetectSetupPathState_CheckPasses_ReturnsSetupOk(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0})

	got := detectSetupPathState(setupSubEntry(), newStubManager(stub))
	if got != StateSetupOk {
		t.Errorf("detectSetupPathState with passing check = %v, want StateSetupOk", got)
	}
}

func TestDetectSetupPathState_CheckFails_ReturnsSetupNeeded(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1})

	got := detectSetupPathState(setupSubEntry(), newStubManager(stub))
	if got != StateSetupNeeded {
		t.Errorf("detectSetupPathState with failing check = %v, want StateSetupNeeded", got)
	}
}

// TestDetectSubEntryState_SetupEntry_RoutesThroughSetupBranch guards against a
// regression where a setup entry falls through to the config-state logic:
// with no Backup, that logic would report StateMissing instead of a setup state.
func TestDetectSubEntryState_SetupEntry_RoutesThroughSetupBranch(t *testing.T) {
	m := &Model{
		Config:   &config.Config{Version: 3, BackupRoot: "/repo"},
		Platform: &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}},
	}
	item := &SubEntryItem{AppName: "vicinae", SubEntry: setupSubEntry()}

	got := m.detectSubEntryState(item)
	if got != StateSetupOk {
		t.Errorf("detectSubEntryState for a setup entry with nil Manager = %v, want StateSetupOk", got)
	}
}

func TestDetectSubEntryState_SetupEntryCheckFails_ReturnsSetupNeeded(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1})

	m := &Model{
		Config:   &config.Config{Version: 3, BackupRoot: "/repo"},
		Platform: &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}},
		Manager:  newStubManager(stub),
	}
	item := &SubEntryItem{AppName: "vicinae", SubEntry: setupSubEntry()}

	got := m.detectSubEntryState(item)
	if got != StateSetupNeeded {
		t.Errorf("detectSubEntryState for a failing setup entry = %v, want StateSetupNeeded", got)
	}
}

// TestDetectSubEntryStateStatic_SetupEntry_RoutesThroughSetupBranch mirrors the
// Model-method test above for the goroutine-safe static variant used by the
// async detection pipeline.
func TestDetectSubEntryStateStatic_SetupEntry_RoutesThroughSetupBranch(t *testing.T) {
	plat := &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}
	cfg := &config.Config{Version: 3, BackupRoot: "/repo"}
	item := SubEntryItem{AppName: "vicinae", SubEntry: setupSubEntry()}

	got := detectSubEntryStateStatic(item, plat, cfg, nil)
	if got != StateSetupOk {
		t.Errorf("detectSubEntryStateStatic for a setup entry with nil manager = %v, want StateSetupOk", got)
	}
}

func TestDetectSubEntryStateStatic_SetupEntryCheckPasses_ReturnsSetupOk(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0})

	plat := &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}
	cfg := &config.Config{Version: 3, BackupRoot: "/repo"}
	item := SubEntryItem{AppName: "vicinae", SubEntry: setupSubEntry()}

	got := detectSubEntryStateStatic(item, plat, cfg, newStubManager(stub))
	if got != StateSetupOk {
		t.Errorf("detectSubEntryStateStatic for a passing setup entry = %v, want StateSetupOk", got)
	}
}
