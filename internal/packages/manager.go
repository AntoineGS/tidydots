package packages

import (
	"context"

	"github.com/AntoineGS/tidydots/internal/platform"
)

// Manager handles package installation with platform detection and manager selection.
// It detects available package managers on the system, selects a preferred manager
// based on configuration and OS, and provides methods to install packages using
// the appropriate installation method. It supports dry-run mode for previewing
// operations and verbose mode for detailed output.
type Manager struct {
	ctx          context.Context
	Config       *Config
	OS           string
	Preferred    PackageManager
	Available    []PackageManager
	availableSet map[PackageManager]bool
	DryRun       bool
	Verbose      bool
}

// NewManager creates a new package Manager with the given configuration.
// It detects available package managers on the system and selects a preferred
// manager based on the configuration priority, default manager setting, or
// OS-specific defaults. The osType parameter specifies the target OS (linux/windows),
// and dryRun/verbose control the execution mode.
func NewManager(cfg *Config, osType string, dryRun, verbose bool) *Manager {
	m := &Manager{
		ctx:     context.Background(),
		Config:  cfg,
		OS:      osType,
		DryRun:  dryRun,
		Verbose: verbose,
	}
	m.detectAvailableManagers()
	m.selectPreferredManager()

	return m
}

// WithContext returns a new Manager with the given context for cancellation support.
func (m *Manager) WithContext(ctx context.Context) *Manager {
	m2 := *m
	m2.ctx = ctx
	return &m2
}

func (m *Manager) detectAvailableManagers() {
	m.availableSet = make(map[PackageManager]bool)
	for _, mgr := range platform.DetectAvailableManagers() {
		pm := PackageManager(mgr)
		m.Available = append(m.Available, pm)
		m.availableSet[pm] = true
	}
}

func (m *Manager) selectPreferredManager() {
	// Use configured priority if available
	if len(m.Config.ManagerPriority) > 0 {
		for _, mgr := range m.Config.ManagerPriority {
			if m.HasManager(mgr) {
				m.Preferred = mgr
				return
			}
		}
	}

	// Use default if set and available
	if m.Config.DefaultManager != "" && m.HasManager(m.Config.DefaultManager) {
		m.Preferred = m.Config.DefaultManager
		return
	}

	// Auto-select based on OS
	if m.OS == platform.OSWindows {
		for _, mgr := range []PackageManager{Winget, Scoop, Choco} {
			if m.HasManager(mgr) {
				m.Preferred = mgr
				return
			}
		}
	} else {
		// Linux/macOS priority
		for _, mgr := range []PackageManager{Yay, Paru, Pacman, Apt, Dnf, Brew} {
			if m.HasManager(mgr) {
				m.Preferred = mgr
				return
			}
		}
	}
}

// HasManager checks if a package manager is available on the system.
// It returns true if the specified manager was detected during initialization.
func (m *Manager) HasManager(mgr PackageManager) bool {
	return m.availableSet[mgr]
}
