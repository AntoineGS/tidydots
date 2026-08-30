package main

import (
	"fmt"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/spf13/cobra"
)

const maxTargetArgs = 2

func validateTargetArgs(cmd *cobra.Command, args []string) error {
	return cobra.MaximumNArgs(maxTargetArgs)(cmd, args)
}

func validateInteractiveTarget(enabled bool, args []string) error {
	if enabled && len(args) > 0 {
		return fmt.Errorf("target arguments cannot be used with --interactive")
	}
	return nil
}

func selectConfigTarget(
	cfg *config.Config,
	renderer config.PathRenderer,
	args []string,
	allowSetup bool,
) (*config.Config, error) {
	if len(args) == 0 {
		return cfg, nil
	}
	if len(args) > maxTargetArgs {
		return nil, fmt.Errorf("target accepts at most %d arguments", maxTargetArgs)
	}

	appName := args[0]
	var selectedApp *config.Application
	for i := range cfg.Applications {
		if cfg.Applications[i].Name == appName {
			app := cfg.Applications[i]
			selectedApp = &app
			break
		}
	}
	if selectedApp == nil {
		return nil, fmt.Errorf("application %q not found", appName)
	}
	if !config.EvaluateWhen(selectedApp.When, renderer) {
		return nil, fmt.Errorf("application %q does not match current conditions", appName)
	}

	if len(args) == maxTargetArgs {
		entryName := args[1] //nolint:gosec // argument count is bounded above and zero is handled before indexing
		var selectedEntry *config.SubEntry
		for i := range selectedApp.Entries {
			if selectedApp.Entries[i].Name == entryName {
				entry := selectedApp.Entries[i]
				selectedEntry = &entry
				break
			}
		}
		if selectedEntry == nil {
			return nil, fmt.Errorf("entry %q not found in application %q", entryName, appName)
		}
		if !config.EvaluateWhen(selectedEntry.When, renderer) {
			return nil, fmt.Errorf("entry %q in application %q does not match current conditions", entryName, appName)
		}
		if selectedEntry.IsSetup() && !allowSetup {
			return nil, fmt.Errorf("entry %q in application %q is a setup entry and is not supported by this command", entryName, appName)
		}
		selectedApp.Entries = []config.SubEntry{*selectedEntry}
		selectedApp.Package = nil
	}

	selectedConfig := *cfg
	selectedConfig.Applications = []config.Application{*selectedApp}
	return &selectedConfig, nil
}
