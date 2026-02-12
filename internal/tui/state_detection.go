package tui

import (
	"path/filepath"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/AntoineGS/tidydots/internal/platform"
	tea "github.com/charmbracelet/bubbletea"
)

// detectSubEntryState determines the state of a sub-entry item
func (m *Model) detectSubEntryState(item *SubEntryItem) PathState {
	targetPath := config.ExpandPath(item.Target, m.Platform.EnvVars)
	backupPath := m.resolvePath(item.SubEntry.Backup)

	st := detectConfigState(backupPath, targetPath, item.SubEntry.IsFolder(), item.SubEntry.Files)

	if st == StateLinked && item.SubEntry.IsConfig() && item.SubEntry.IsFolder() && m.Manager != nil {
		if m.Manager.HasOutdatedTemplates(backupPath) {
			return StateOutdated
		}
		if m.Manager.HasModifiedRenderedFiles(backupPath) {
			return StateModified
		}
	}

	return st
}

// checkPackageStatesCmd returns a tea.Cmd that detects package methods and checks install statuses.
// Each package gets its own concurrent command so results stream in individually.
func (m Model) checkPackageStatesCmd() tea.Cmd {
	var cmds []tea.Cmd
	osType := m.Platform.OS

	for i, app := range m.Applications {
		if app.IsFiltered || !app.Application.HasPackage() {
			continue
		}
		appIndex := i
		pkg := app.Application.Package
		name := app.Application.Name
		cmds = append(cmds, func() tea.Msg {
			method := getPackageInstallMethodFromPackage(pkg, osType)
			installed := false
			if method != TypeNone {
				installed = isPackageInstalledFromPackage(pkg, method, name)
			}
			return pkgCheckResultMsg{appIndex: appIndex, method: method, installed: installed}
		})
	}

	return tea.Batch(cmds...)
}

// checkSubEntryStatesCmd returns a tea.Cmd that checks all sub-entry states.
// Each sub-entry gets its own concurrent command so results stream in individually.
// Filtered apps are skipped; use checkFilteredStatesCmd when the filter is toggled off.
func (m Model) checkSubEntryStatesCmd() tea.Cmd {
	var cmds []tea.Cmd
	plat := m.Platform
	cfg := m.Config
	mgr := m.Manager

	for i, app := range m.Applications {
		if app.IsFiltered {
			continue
		}
		for j, sub := range app.SubItems {
			appIndex := i
			subIndex := j
			subItem := sub
			cmds = append(cmds, func() tea.Msg {
				state := detectSubEntryStateStatic(subItem, plat, cfg, mgr)
				return stateCheckResultMsg{appIndex: appIndex, subIndex: subIndex, state: state}
			})
		}
	}

	return tea.Batch(cmds...)
}

// checkUncheckedPackageStatesCmd triggers async checks for apps whose package state hasn't been resolved yet.
// Called after saving a new or edited application to resolve "Loading..." status.
func (m Model) checkUncheckedPackageStatesCmd() tea.Cmd {
	var cmds []tea.Cmd
	osType := m.Platform.OS

	for i, app := range m.Applications {
		if !app.Application.HasPackage() || app.PkgInstalled != nil {
			continue
		}
		appIndex := i
		pkg := app.Application.Package
		name := app.Application.Name
		cmds = append(cmds, func() tea.Msg {
			method := getPackageInstallMethodFromPackage(pkg, osType)
			installed := false
			if method != TypeNone {
				installed = isPackageInstalledFromPackage(pkg, method, name)
			}
			return pkgCheckResultMsg{appIndex: appIndex, method: method, installed: installed}
		})
	}

	return tea.Batch(cmds...)
}

// checkFilteredStatesCmd triggers async checks for filtered apps that haven't been scanned yet.
// Called when the user toggles the filter off, revealing previously-hidden apps.
func (m Model) checkFilteredStatesCmd() tea.Cmd {
	var cmds []tea.Cmd
	osType := m.Platform.OS
	plat := m.Platform
	cfg := m.Config
	mgr := m.Manager

	for i, app := range m.Applications {
		if !app.IsFiltered {
			continue
		}

		// Package check (only if not already resolved)
		if app.Application.HasPackage() && app.PkgInstalled == nil {
			appIndex := i
			pkg := app.Application.Package
			name := app.Application.Name
			cmds = append(cmds, func() tea.Msg {
				method := getPackageInstallMethodFromPackage(pkg, osType)
				installed := false
				if method != TypeNone {
					installed = isPackageInstalledFromPackage(pkg, method, name)
				}
				return pkgCheckResultMsg{appIndex: appIndex, method: method, installed: installed}
			})
		}

		// Sub-entry state checks (only if still at StateLoading)
		for j, sub := range app.SubItems {
			if sub.State != StateLoading {
				continue
			}
			appIndex := i
			subIndex := j
			subItem := sub
			cmds = append(cmds, func() tea.Msg {
				state := detectSubEntryStateStatic(subItem, plat, cfg, mgr)
				return stateCheckResultMsg{appIndex: appIndex, subIndex: subIndex, state: state}
			})
		}
	}

	return tea.Batch(cmds...)
}

// detectSubEntryStateStatic determines the state of a sub-entry item without using Model receiver.
// This is safe to call from goroutines since it takes explicit dependencies.
func detectSubEntryStateStatic(item SubEntryItem, plat *platform.Platform, cfg *config.Config, mgr *manager.Manager) PathState {
	targetPath := config.ExpandPath(item.Target, plat.EnvVars)
	backupPath := resolvePathStatic(item.SubEntry.Backup, cfg, plat.EnvVars)

	st := detectConfigState(backupPath, targetPath, item.SubEntry.IsFolder(), item.SubEntry.Files)

	if st == StateLinked && item.SubEntry.IsConfig() && item.SubEntry.IsFolder() && mgr != nil {
		if mgr.HasOutdatedTemplates(backupPath) {
			return StateOutdated
		}
		if mgr.HasModifiedRenderedFiles(backupPath) {
			return StateModified
		}
	}

	return st
}

// resolvePathStatic resolves relative paths against BackupRoot and expands ~ without using Model receiver.
func resolvePathStatic(path string, cfg *config.Config, envVars map[string]string) string {
	expandedPath := config.ExpandPath(path, envVars)

	if filepath.IsAbs(expandedPath) {
		return expandedPath
	}

	expandedBackupRoot := config.ExpandPath(cfg.BackupRoot, envVars)
	return filepath.Join(expandedBackupRoot, expandedPath)
}
