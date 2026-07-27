package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/spf13/cobra"
)

func TestTargetCommandPolicyWiring(t *testing.T) {
	originalFactory := createCommandManager
	originalInteractive := interactive
	interactive = false
	t.Cleanup(func() {
		createCommandManager = originalFactory
		interactive = originalInteractive
	})

	sentinel := errors.New("manager construction stopped")
	tests := []struct {
		run        func(*cobra.Command, []string) error
		name       string
		wantArgs   []string
		allowSetup bool
	}{
		{name: "restore", run: runRestore, wantArgs: []string{"nvim", "setup"}, allowSetup: true},
		{name: "backup", run: runBackup, wantArgs: []string{"nvim", "config"}, allowSetup: false},
		{name: "list", run: runList, wantArgs: []string{"shell", "config"}, allowSetup: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotArgs []string
			var gotAllowSetup bool
			called := false
			createCommandManager = func(args []string, allowSetup bool) (*manager.Manager, error) {
				called = true
				gotArgs = append([]string(nil), args...)
				gotAllowSetup = allowSetup
				return nil, sentinel
			}

			err := tt.run(nil, tt.wantArgs)
			if !errors.Is(err, sentinel) {
				t.Fatalf("command error = %v, want manager construction error", err)
			}
			if !called {
				t.Fatal("command did not construct a manager")
			}
			if strings.Join(gotArgs, "\x00") != strings.Join(tt.wantArgs, "\x00") {
				t.Fatalf("manager args = %q, want %q", gotArgs, tt.wantArgs)
			}
			if gotAllowSetup != tt.allowSetup {
				t.Fatalf("manager allowSetup = %v, want %v", gotAllowSetup, tt.allowSetup)
			}
		})
	}
}

func TestInteractiveTargetRejectedBeforeManagerConstruction(t *testing.T) {
	originalFactory := createCommandManager
	originalInteractive := interactive
	interactive = true
	t.Cleanup(func() {
		createCommandManager = originalFactory
		interactive = originalInteractive
	})

	tests := []struct {
		run  func(*cobra.Command, []string) error
		name string
	}{
		{name: "restore", run: runRestore},
		{name: "backup", run: runBackup},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			createCommandManager = func(_ []string, _ bool) (*manager.Manager, error) {
				called = true
				return nil, errors.New("unexpected manager construction")
			}

			err := tt.run(nil, []string{"nvim"})
			if err == nil || !strings.Contains(err.Error(), "cannot be used with --interactive") {
				t.Fatalf("command error = %v, want interactive target conflict", err)
			}
			if called {
				t.Fatal("command constructed a manager before rejecting interactive target")
			}
		})
	}
}
