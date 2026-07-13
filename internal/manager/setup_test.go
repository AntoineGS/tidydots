package manager

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/cmdexec"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// setupEntry is the canonical setup sub-entry used across these tests.
func setupEntry() config.SubEntry {
	return config.SubEntry{
		Name:  "enable-service",
		Check: map[string]string{"linux": "systemctl --user is-enabled --quiet vicinae.service"},
		Run:   map[string]string{"linux": "systemctl --user enable --now vicinae.service"},
	}
}

// newSetupManager builds a Manager on Linux with the given stub runner.
func newSetupManager(stub *cmdexec.StubRunner, dryRun bool) *Manager {
	cfg := &config.Config{Version: 3, BackupRoot: "/repo"}
	plat := &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}

	m := New(cfg, plat).WithRunner(stub)
	m.DryRun = dryRun

	return m
}

// shellCalls returns only the calls that executed a shell command.
func shellCalls(stub *cmdexec.StubRunner) []cmdexec.Call {
	var out []cmdexec.Call
	for _, c := range stub.Calls {
		if c.Name == "sh" {
			out = append(out, c)
		}
	}
	return out
}

func TestRunSetupEntry_CheckPasses_DoesNotRun(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0}) // check passes

	m := newSetupManager(stub, false)

	if err := m.runSetupEntry("vicinae", setupEntry()); err != nil {
		t.Fatalf("runSetupEntry returned error: %v", err)
	}

	calls := shellCalls(stub)
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 shell call (the check), got %d: %+v", len(calls), calls)
	}

	if !strings.Contains(calls[0].Args[1], "is-enabled") {
		t.Errorf("the single call was not the check: %q", calls[0].Args[1])
	}
}

func TestRunSetupEntry_CheckFails_RunsThenRechecks(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1}) // check fails
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0}) // run succeeds
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0}) // re-check passes

	m := newSetupManager(stub, false)

	if err := m.runSetupEntry("vicinae", setupEntry()); err != nil {
		t.Fatalf("runSetupEntry returned error: %v", err)
	}

	calls := shellCalls(stub)
	if len(calls) != 3 {
		t.Fatalf("expected 3 shell calls (check, run, re-check), got %d", len(calls))
	}

	if !strings.Contains(calls[1].Args[1], "enable --now") {
		t.Errorf("second call should be the run command, got %q", calls[1].Args[1])
	}

	if !strings.Contains(calls[2].Args[1], "is-enabled") {
		t.Errorf("third call should be the re-check, got %q", calls[2].Args[1])
	}

	if calls[1].Dir != "/repo" {
		t.Errorf("run command Dir = %q, want %q (the configurations repo root)", calls[1].Dir, "/repo")
	}
}

func TestRunSetupEntry_RunFails_ReturnsError(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1})                                      // check fails
	stub.AddResult("sh", cmdexec.Result{ExitCode: 2, Stderr: []byte("permission denied")}) // run fails

	m := newSetupManager(stub, false)

	err := m.runSetupEntry("vicinae", setupEntry())
	if err == nil {
		t.Fatal("expected an error when the run command fails, got nil")
	}

	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error should surface stderr, got: %v", err)
	}
}

func TestRunSetupEntry_RunSucceedsButRecheckFails_ReturnsError(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1}) // check fails
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0}) // run "succeeds"
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1}) // re-check STILL fails

	m := newSetupManager(stub, false)

	err := m.runSetupEntry("vicinae", setupEntry())
	if err == nil {
		t.Fatal("expected an error when the re-check still fails, got nil")
	}

	if !strings.Contains(err.Error(), "check still fails") {
		t.Errorf("error should name the silent-failure case, got: %v", err)
	}
}

func TestRunSetupEntry_DryRun_RunsCheckButNeverRun(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1}) // check fails

	m := newSetupManager(stub, true)

	if err := m.runSetupEntry("vicinae", setupEntry()); err != nil {
		t.Fatalf("dry run returned error: %v", err)
	}

	calls := shellCalls(stub)
	if len(calls) != 1 {
		t.Fatalf("dry run should make exactly 1 shell call (the check), got %d", len(calls))
	}

	if strings.Contains(calls[0].Args[1], "enable --now") {
		t.Error("dry run executed the run command; it must never do that")
	}
}

func TestRunSetupEntry_OSNotInRunMap_SkipsEntirely(t *testing.T) {
	stub := cmdexec.NewStubRunner()

	e := setupEntry()
	e.Check = map[string]string{"windows": "check"}
	e.Run = map[string]string{"windows": "run"}

	m := newSetupManager(stub, false) // Linux

	if err := m.runSetupEntry("vicinae", e); err != nil {
		t.Fatalf("runSetupEntry returned error: %v", err)
	}

	if len(shellCalls(stub)) != 0 {
		t.Errorf("expected no shell calls for an entry that does not apply to this OS, got %d", len(shellCalls(stub)))
	}
}

func TestRunSetupEntry_Sudo_UsesSudo(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1}) // check fails
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0}) // run succeeds
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0}) // re-check passes

	e := setupEntry()
	e.Sudo = true

	m := newSetupManager(stub, false)

	if err := m.runSetupEntry("vicinae", e); err != nil {
		t.Fatalf("runSetupEntry returned error: %v", err)
	}

	calls := shellCalls(stub)
	if len(calls) != 3 {
		t.Fatalf("expected 3 shell calls, got %d", len(calls))
	}

	if !calls[1].Sudo {
		t.Error("run command should have been dispatched with sudo")
	}

	if calls[0].Sudo {
		t.Error("check command must NOT use sudo; only the run command does")
	}
}

func TestRunSetupEntry_MissingCheckForOS_ReturnsError(t *testing.T) {
	stub := cmdexec.NewStubRunner()

	e := setupEntry()
	e.Check = nil // validation normally prevents this; guard against a hand-built config

	m := newSetupManager(stub, false)

	if err := m.runSetupEntry("vicinae", e); err == nil {
		t.Fatal("expected an error when run has no matching check, got nil")
	}

	if len(shellCalls(stub)) != 0 {
		t.Error("must not execute anything when the check is missing")
	}
}

func TestRestore_SetupEntryFailure_DoesNotAbortRestore(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1})                         // app-one check fails
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1, Stderr: []byte("boom")}) // app-one run fails
	stub.AddResult("sh", cmdexec.Result{ExitCode: 0})                         // app-two check passes

	appOne := config.Application{Name: "app-one", Entries: []config.SubEntry{setupEntry()}}
	appTwo := config.Application{Name: "app-two", Entries: []config.SubEntry{setupEntry()}}

	cfg := &config.Config{
		Version:      3,
		BackupRoot:   "/repo",
		Applications: []config.Application{appOne, appTwo},
	}
	plat := &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}

	m := New(cfg, plat).WithRunner(stub)

	err := m.Restore()
	if err == nil {
		t.Fatal("expected Restore to report the failed setup entry")
	}

	// app-two's check must still have run: one entry's failure must not abort the rest.
	if len(shellCalls(stub)) != 3 {
		t.Errorf("expected 3 shell calls (app-one check+run, app-two check), got %d; "+
			"restore aborted early instead of collecting the error",
			len(shellCalls(stub)))
	}
}

// scriptedResponse is a single canned reply from scriptedRunner.
type scriptedResponse struct {
	err error
	res cmdexec.Result
}

// scriptedRunner is a Runner that can return a NON-NIL error, which
// cmdexec.StubRunner deliberately cannot (it signals failure through ExitCode
// alone). OsRunner returns exactly this shape whenever the command never
// launched — shell missing, working directory gone, context already canceled:
// a non-nil error together with ExitCode 0 and no captured output.
type scriptedRunner struct {
	responses []scriptedResponse
	Calls     []cmdexec.Call
}

func (r *scriptedRunner) RunIn(
	_ context.Context, opts cmdexec.RunOptions, name string, args ...string,
) (cmdexec.Result, error) {
	r.Calls = append(r.Calls, cmdexec.Call{Name: name, Args: args, Dir: opts.Dir, Sudo: opts.Sudo})

	if len(r.responses) == 0 {
		return cmdexec.Result{}, nil
	}

	next := r.responses[0]
	r.responses = r.responses[1:]

	return next.res, next.err
}

func (r *scriptedRunner) Run(ctx context.Context, name string, args ...string) (cmdexec.Result, error) {
	return r.RunIn(ctx, cmdexec.RunOptions{}, name, args...)
}

func (r *scriptedRunner) RunWithSudo(ctx context.Context, name string, args ...string) (cmdexec.Result, error) {
	return r.RunIn(ctx, cmdexec.RunOptions{Sudo: true}, name, args...)
}

func (r *scriptedRunner) LookPath(string) (string, error) {
	return "", exec.ErrNotFound
}

// newScriptedManager builds a Linux Manager backed by a scriptedRunner.
func newScriptedManager(responses ...scriptedResponse) (*Manager, *scriptedRunner) {
	runner := &scriptedRunner{responses: responses}

	cfg := &config.Config{Version: 3, BackupRoot: "/repo"}
	plat := &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}

	return New(cfg, plat).WithRunner(runner), runner
}

// TestRunSetupEntry_RunNeverLaunched_SurfacesRealCause is the regression guard
// for the "exit 0" bug: when the run command cannot even start, OsRunner
// returns a non-nil error with ExitCode 0 and empty Stderr. Formatting only
// the exit code and stderr produced "command failed (exit 0): " — which reads
// as success and threw away the only thing that explained the failure.
func TestRunSetupEntry_RunNeverLaunched_SurfacesRealCause(t *testing.T) {
	launchErr := errors.New(`exec: "sh": executable file not found in $PATH`)

	m, runner := newScriptedManager(
		scriptedResponse{res: cmdexec.Result{ExitCode: 1}},      // check fails
		scriptedResponse{res: cmdexec.Result{}, err: launchErr}, // run never launches
	)

	err := m.runSetupEntry("vicinae", setupEntry())
	if err == nil {
		t.Fatal("expected an error when the run command cannot be launched, got nil")
	}

	if !strings.Contains(err.Error(), "executable file not found") {
		t.Errorf("error must surface the real cause, got: %v", err)
	}

	if strings.HasSuffix(err.Error(), ": ") {
		t.Errorf("error must not end in a dangling %q, got: %q", ": ", err.Error())
	}

	if !errors.Is(err, launchErr) {
		t.Errorf("error must wrap the launch error so callers can inspect it, got: %v", err)
	}

	// A run that never launched changed nothing, so there is nothing to
	// re-check: the failure is reported instead.
	if len(runner.Calls) != 2 {
		t.Errorf("executed %d command(s), want 2 (check, then the run that failed to launch); "+
			"the re-check must not run after a failed run: %+v", len(runner.Calls), runner.Calls)
	}
}

// TestRunSetupEntry_RunContextCanceled_WrapsContextCanceled proves a canceled
// run is still detectable by callers via errors.Is.
func TestRunSetupEntry_RunContextCanceled_WrapsContextCanceled(t *testing.T) {
	m, runner := newScriptedManager(
		scriptedResponse{res: cmdexec.Result{ExitCode: 1}},             // check fails
		scriptedResponse{res: cmdexec.Result{}, err: context.Canceled}, // run canceled
	)
	defer func() {
		if len(runner.Calls) != 2 {
			t.Errorf("executed %d command(s), want 2 (check, then the canceled run)", len(runner.Calls))
		}
	}()

	err := m.runSetupEntry("vicinae", setupEntry())
	if err == nil {
		t.Fatal("expected an error when the run command is canceled, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error must wrap context.Canceled, got: %v", err)
	}
}

// TestRunSetupEntry_RunFailsSilently_OmitsDanglingSeparator covers a real
// non-zero exit that printed nothing: there is no reason to report, so the
// message must stop after the exit code rather than trailing a bare ": ".
func TestRunSetupEntry_RunFailsSilently_OmitsDanglingSeparator(t *testing.T) {
	stub := cmdexec.NewStubRunner()
	stub.AddResult("sh", cmdexec.Result{ExitCode: 1}) // check fails
	stub.AddResult("sh", cmdexec.Result{ExitCode: 3}) // run fails, prints nothing

	m := newSetupManager(stub, false)

	err := m.runSetupEntry("vicinae", setupEntry())
	if err == nil {
		t.Fatal("expected an error when the run command exits non-zero, got nil")
	}

	want := "setup vicinae/enable-service: command failed (exit 3)"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}
