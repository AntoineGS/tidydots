package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/tui"
)

func preserveCommandGlobals(t *testing.T) {
	t.Helper()

	originalConfigDir := configDir
	originalOSOverride := osOverride
	originalDryRun := dryRun
	originalVerbose := verbose
	originalActions := actions
	originalStatusActions := statusActions
	originalStatusJSON := statusJSON
	originalInteractiveIsTerminal := interactiveIsTerminal
	originalRunInteractiveTUI := runInteractiveTUI

	configDir = ""
	osOverride = ""
	dryRun = false
	verbose = false
	actions = false
	statusActions = false
	statusJSON = false

	t.Cleanup(func() {
		configDir = originalConfigDir
		osOverride = originalOSOverride
		dryRun = originalDryRun
		verbose = originalVerbose
		actions = originalActions
		statusActions = originalStatusActions
		statusJSON = originalStatusJSON
		interactiveIsTerminal = originalInteractiveIsTerminal
		runInteractiveTUI = originalRunInteractiveTUI
	})
}

func writeStatusConfig(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	configYAML := fmt.Sprintf(`version: 3
applications:
  - name: tool
    description: Tool description
    entries:
      - name: missing
        backup: ./missing
        targets:
          linux: %q
`, target)
	if err := os.WriteFile(filepath.Join(dir, "tidydots.yaml"), []byte(configYAML), 0o600); err != nil {
		t.Fatalf("writing status config: %v", err)
	}

	return dir
}

func executeStatusCommand(t *testing.T, dir string) (string, error) {
	t.Helper()

	root := newRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--dir", dir,
		"--os", "linux",
		"--verbose",
		"status", "--actions", "--json",
	})
	err := root.Execute()

	return output.String(), err
}

func TestStatusJSONIsStableAndActionableDoesNotChangeExitStatus(t *testing.T) {
	preserveCommandGlobals(t)
	dir := writeStatusConfig(t)

	first, err := executeStatusCommand(t, dir)
	if err != nil {
		t.Fatalf("status command error for actionable config = %v", err)
	}
	second, err := executeStatusCommand(t, dir)
	if err != nil {
		t.Fatalf("second status command error = %v", err)
	}
	if first != second {
		t.Fatalf("status JSON changed between identical runs:\nfirst:\n%ssecond:\n%s", first, second)
	}

	var report tui.StatusReport
	if err := json.Unmarshal([]byte(first), &report); err != nil {
		t.Fatalf("status output is not JSON: %v\n%s", err, first)
	}
	if !report.Actionable || !report.ActionsOnly {
		t.Fatalf("report flags = actionable:%t actions_only:%t, want true:true", report.Actionable, report.ActionsOnly)
	}
	if report.Counts.ActionableApplications != 1 || report.Counts.ActionableEntries != 1 {
		t.Fatalf("action counts = %+v, want one application and entry", report.Counts)
	}
	if len(report.Applications) != 1 || len(report.Applications[0].Entries) != 1 {
		t.Fatalf("action details = %+v, want one application and entry", report.Applications)
	}
	if report.Applications[0].Name != "tool" || report.Applications[0].Entries[0].Name != "missing" {
		t.Fatalf("detail identity = %+v, want tool/missing", report.Applications)
	}
	if report.Applications[0].Entries[0].State != tui.StateMissing.String() {
		t.Fatalf("entry state = %q, want %q", report.Applications[0].Entries[0].State, tui.StateMissing.String())
	}
}

func TestStatusFailureReturnsError(t *testing.T) {
	preserveCommandGlobals(t)

	root := newRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--dir", filepath.Join(t.TempDir(), "missing"), "status", "--json"})
	if err := root.Execute(); err == nil {
		t.Fatal("status command succeeded with a missing configuration directory")
	}
}

func TestRootActionsStartsInteractiveTUIWithActionFilter(t *testing.T) {
	preserveCommandGlobals(t)
	dir := writeStatusConfig(t)

	interactiveIsTerminal = func() bool { return true }
	var gotActionFilter bool
	runInteractiveTUI = func(_ *config.Config, _ *platform.Platform, _ bool, _ string, actionFilterEnabled bool) error {
		gotActionFilter = actionFilterEnabled
		return nil
	}

	root := newRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--dir", dir, "--os", "linux", "--actions"})
	if err := root.Execute(); err != nil {
		t.Fatalf("root --actions command error = %v", err)
	}
	if !gotActionFilter {
		t.Fatal("root --actions did not enable the TUI action filter")
	}
}

func TestRootNoArgsPreservesInteractiveStartup(t *testing.T) {
	preserveCommandGlobals(t)
	dir := writeStatusConfig(t)

	interactiveIsTerminal = func() bool { return true }
	var gotActionFilter bool
	runInteractiveTUI = func(_ *config.Config, _ *platform.Platform, _ bool, _ string, actionFilterEnabled bool) error {
		gotActionFilter = actionFilterEnabled
		return nil
	}

	root := newRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--dir", dir, "--os", "linux"})
	if err := root.Execute(); err != nil {
		t.Fatalf("no-argument command error = %v", err)
	}
	if gotActionFilter {
		t.Fatal("no-argument startup unexpectedly enabled the action filter")
	}
}

func TestRootActionsRejectsSubcommandCombination(t *testing.T) {
	preserveCommandGlobals(t)

	root := newRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"--actions", "list"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --actions") {
		t.Fatalf("root --actions list error = %v, want invalid combination", err)
	}
}
