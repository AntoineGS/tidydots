package tui

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// Run starts the interactive TUI with a new manager
func Run(cfg *config.Config, plat *platform.Platform, dryRun bool, configPath string) error {
	return run(cfg, plat, dryRun, configPath, false)
}

// RunWithActionFilter starts the interactive TUI with action filtering enabled.
func RunWithActionFilter(cfg *config.Config, plat *platform.Platform, dryRun bool, configPath string) error {
	return run(cfg, plat, dryRun, configPath, true)
}

func run(cfg *config.Config, plat *platform.Platform, dryRun bool, configPath string, actionFilterEnabled bool) error {
	mgr := manager.New(cfg, plat)
	mgr.DryRun = dryRun

	if err := mgr.InitStateStore(); err != nil {
		// Non-fatal: outdated detection won't work, but TUI is still usable
		fmt.Fprintf(os.Stderr, "Warning: could not initialize state store: %v\n", err)
	}
	defer func() { _ = mgr.Close() }()

	return RunWithManagerAndActionFilter(cfg, plat, mgr, configPath, actionFilterEnabled)
}

// RunWithManager runs the TUI with an existing manager
func RunWithManager(cfg *config.Config, plat *platform.Platform, mgr *manager.Manager, configPath string) error {
	return RunWithManagerAndActionFilter(cfg, plat, mgr, configPath, false)
}

// RunWithManagerAndActionFilter runs the TUI with an existing manager and an
// optional action filter enabled at startup.
func RunWithManagerAndActionFilter(
	cfg *config.Config,
	plat *platform.Platform,
	mgr *manager.Manager,
	configPath string,
	actionFilterEnabled bool,
) error {
	model := NewModelWithManagerAndActionFilter(cfg, plat, mgr, configPath, actionFilterEnabled)

	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// If we completed an operation (not list), show a summary
	m, ok := finalModel.(Model)
	if !ok {
		return fmt.Errorf("unexpected model type")
	}

	if m.Screen == ScreenResults && m.Operation != OpList && len(m.results) > 0 {
		printFinalSummary(m)
	}

	return nil
}

// NewModelWithManager creates a model with a manager for real operations
func NewModelWithManager(cfg *config.Config, plat *platform.Platform, mgr *manager.Manager, configPath string) Model {
	return NewModelWithManagerAndActionFilter(cfg, plat, mgr, configPath, false)
}

// NewModelWithManagerAndActionFilter creates a model with a manager and an
// optional action filter enabled at startup.
func NewModelWithManagerAndActionFilter(
	cfg *config.Config,
	plat *platform.Platform,
	mgr *manager.Manager,
	configPath string,
	actionFilterEnabled bool,
) Model {
	m := NewModelWithActionFilter(cfg, plat, mgr.DryRun, actionFilterEnabled)
	m.Manager = mgr
	m.ConfigPath = configPath
	m.HostnameChoices = append([]string(nil), cfg.Hostnames...)
	if len(m.HostnameChoices) == 0 {
		if appCfg, err := config.LoadAppConfigMetadata(); err == nil {
			m.HostnameChoices = append(m.HostnameChoices, appCfg.Hostnames...)
		}
	}

	return m
}

func printFinalSummary(m Model) {
	successCount := 0
	failCount := 0

	for _, r := range m.results {
		if r.Success {
			successCount++
		} else {
			failCount++
		}
	}

	fmt.Printf("\n%s complete: %d successful", m.Operation.String(), successCount)

	if failCount > 0 {
		fmt.Printf(", %d failed", failCount)
	}

	fmt.Println()
}

// IsTerminal checks if stdout is a terminal
func IsTerminal() bool {
	fileInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}
