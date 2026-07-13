package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/cmdexec"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// collectMsgs runs a tea.Cmd and flattens any tea.BatchMsg it produces into
// the individual messages returned by each dispatched sub-command. A nil cmd
// (e.g. zero sub-entries dispatched) yields no messages.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}

	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, sub := range batch {
			out = append(out, collectMsgs(sub)...)
		}
		return out
	}

	if msg == nil {
		return nil
	}

	return []tea.Msg{msg}
}

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

// TestDetectSubEntryState_SetupEntry_NeverShellsOut is the regression guard
// for the Task 5 bug: detectSubEntryState is a Model method that runs
// synchronously on the bubbletea UI goroutine (called from
// refreshApplicationStates and reinitPreservingState). Resolving a setup
// entry's state requires running its check command as a real subprocess
// (detectSetupPathState -> manager.IsSetupApplied -> runner.RunIn), and doing
// that here would visibly stall the UI. The sync path must therefore defer to
// StateLoading without touching the runner at all — the strongest assertion
// of that is that the stub recorded zero calls.
func TestDetectSubEntryState_SetupEntry_NeverShellsOut(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	// Queue a result so that if the sync path incorrectly ran the check, the
	// state returned would differ from StateLoading too — belt and braces on
	// top of the zero-Calls assertion below.
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0})

	m := &Model{
		Config:   &config.Config{Version: 3, BackupRoot: "/repo"},
		Platform: &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}},
		Manager:  newStubManager(stub),
	}
	item := &SubEntryItem{AppName: "vicinae", SubEntry: setupSubEntry()}

	got := m.detectSubEntryState(item)

	if got != StateLoading {
		t.Errorf("detectSubEntryState for a setup entry = %v, want StateLoading (must defer to async resolution, not resolve inline)", got)
	}
	if len(stub.Calls) != 0 {
		t.Errorf("detectSubEntryState executed %d command(s) on the UI goroutine, want 0: %+v", len(stub.Calls), stub.Calls)
	}
}

// TestDetectSubEntryState_SetupEntry_NilManager_ReturnsStateLoading confirms
// the sync path defers to StateLoading for setup entries even when there is
// no Manager at all (e.g. immediately after adding a brand new app), rather
// than reporting a falsely-resolved state.
func TestDetectSubEntryState_SetupEntry_NilManager_ReturnsStateLoading(t *testing.T) {
	m := &Model{
		Config:   &config.Config{Version: 3, BackupRoot: "/repo"},
		Platform: &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}},
	}
	item := &SubEntryItem{AppName: "vicinae", SubEntry: setupSubEntry()}

	got := m.detectSubEntryState(item)
	if got != StateLoading {
		t.Errorf("detectSubEntryState for a setup entry with nil Manager = %v, want StateLoading", got)
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

// otherSetupSubEntry returns a second, distinct setup sub-entry so tests can
// tell two dispatched checks apart by their resolved state.
func otherSetupSubEntry() config.SubEntry {
	return config.SubEntry{
		Name:  "enable-other",
		Check: map[string]string{"linux": "true"},
		Run:   map[string]string{"linux": "true"},
	}
}

// TestCheckLoadingSubEntryStatesCmd_ResolvesOnlyLoadingEntries is the
// regression guard for the async resolver: it must dispatch exactly one
// check per sub-entry still at StateLoading — including in filtered
// (hidden) apps, since refreshApplicationStates and reinitPreservingState
// touch every application regardless of filter state — and must leave
// already-resolved sub-entries untouched.
func TestCheckLoadingSubEntryStatesCmd_ResolvesOnlyLoadingEntries(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0}) // vicinae/enable-service check passes
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1}) // other/enable-other check fails

	m := Model{
		Config:   &config.Config{Version: 3, BackupRoot: "/repo"},
		Platform: &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}},
		Manager:  newStubManager(stub),
		Applications: []ApplicationItem{
			{
				Application: config.Application{Name: "vicinae"},
				IsFiltered:  true, // deliberately hidden: must still be resolved
				SubItems: []SubEntryItem{
					{AppName: "vicinae", SubEntry: setupSubEntry(), State: StateLoading},
					{AppName: "vicinae", SubEntry: config.SubEntry{Name: "config-file", Backup: "cfg"}, State: StateLinked},
				},
			},
			{
				Application: config.Application{Name: "other"},
				SubItems: []SubEntryItem{
					{AppName: "other", SubEntry: otherSetupSubEntry(), State: StateLoading},
				},
			},
		},
	}

	cmd, count := m.checkLoadingSubEntryStatesCmd()
	if count != 2 {
		t.Fatalf("checkLoadingSubEntryStatesCmd() count = %d, want 2 (one per StateLoading sub-entry, filtered or not)", count)
	}

	msgs := collectMsgs(cmd)
	if len(msgs) != 2 {
		t.Fatalf("dispatched %d message(s), want 2", len(msgs))
	}

	got := map[[2]int]PathState{}
	for _, msg := range msgs {
		res, ok := msg.(stateCheckResultMsg)
		if !ok {
			t.Fatalf("unexpected message type %T", msg)
		}
		got[[2]int{res.appIndex, res.subIndex}] = res.state
	}

	if state, resolved := got[[2]int{0, 0}]; !resolved || state != StateSetupOk {
		t.Errorf("vicinae/enable-service resolved to (%v, resolved=%v), want (StateSetupOk, true)", state, resolved)
	}
	if state, resolved := got[[2]int{1, 0}]; !resolved || state != StateSetupNeeded {
		t.Errorf("other/enable-other resolved to (%v, resolved=%v), want (StateSetupNeeded, true)", state, resolved)
	}
	if _, resolved := got[[2]int{0, 1}]; resolved {
		t.Errorf("config-file sub-entry (not StateLoading) was unexpectedly resolved by the loading-only resolver")
	}
}

// TestCheckLoadingSubEntryStatesCmd_NoLoadingEntries_DispatchesNothing
// confirms the resolver is a no-op (and does not touch the runner) once every
// sub-entry has already been resolved.
func TestCheckLoadingSubEntryStatesCmd_NoLoadingEntries_DispatchesNothing(t *testing.T) {
	stub := cmdexec.NewStubRunner()

	m := Model{
		Config:   &config.Config{Version: 3, BackupRoot: "/repo"},
		Platform: &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}},
		Manager:  newStubManager(stub),
		Applications: []ApplicationItem{
			{
				Application: config.Application{Name: "vicinae"},
				SubItems: []SubEntryItem{
					{AppName: "vicinae", SubEntry: setupSubEntry(), State: StateSetupOk},
				},
			},
		},
	}

	cmd, count := m.checkLoadingSubEntryStatesCmd()
	if count != 0 {
		t.Errorf("checkLoadingSubEntryStatesCmd() count = %d, want 0", count)
	}
	if cmd != nil {
		t.Errorf("checkLoadingSubEntryStatesCmd() cmd = %v, want nil", cmd)
	}
	if len(stub.Calls) != 0 {
		t.Errorf("checkLoadingSubEntryStatesCmd executed %d command(s), want 0: %+v", len(stub.Calls), stub.Calls)
	}
}

// TestDispatchLoadingSubEntryStates_TracksPendingCount proves
// dispatchLoadingSubEntryStates dispatches the async resolver AND adds its
// count to pendingStateChecks (rather than overwriting it), matching the
// bookkeeping convention of dispatchFilteredStates / dispatchUncheckedPackageStates.
// A drift here would leave the TUI's spinner/rebuild logic waiting forever
// (or rebuilding early).
func TestDispatchLoadingSubEntryStates_TracksPendingCount(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0})

	m := &Model{
		Config:   &config.Config{Version: 3, BackupRoot: "/repo"},
		Platform: &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}},
		Manager:  newStubManager(stub),
		Applications: []ApplicationItem{
			{
				Application: config.Application{Name: "vicinae"},
				SubItems: []SubEntryItem{
					{AppName: "vicinae", SubEntry: setupSubEntry(), State: StateLoading},
				},
			},
		},
		pendingStateChecks: 3, // simulate other in-flight checks already tracked
	}

	cmd := m.dispatchLoadingSubEntryStates()
	if cmd == nil {
		t.Fatal("dispatchLoadingSubEntryStates() returned a nil cmd despite a StateLoading sub-entry")
	}
	if m.pendingStateChecks != 4 {
		t.Errorf("pendingStateChecks = %d, want 4 (3 pre-existing + 1 dispatched)", m.pendingStateChecks)
	}

	msgs := collectMsgs(cmd)
	if len(msgs) != 1 {
		t.Fatalf("dispatched %d message(s), want 1", len(msgs))
	}
	res, ok := msgs[0].(stateCheckResultMsg)
	if !ok {
		t.Fatalf("unexpected message type %T", msgs[0])
	}
	if res.state != StateSetupOk {
		t.Errorf("resolved state = %v, want StateSetupOk", res.state)
	}
}
