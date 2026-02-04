package tui

import (
	"fmt"

	"github.com/AntoineGS/dot-manager/internal/config"
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
)

// BatchOperationMsg is sent for each step of a batch operation.
// It contains progress information about the current operation.
type BatchOperationMsg struct {
	ItemName    string  // Name of the item being processed
	ItemIndex   int     // Current item index (0-based)
	TotalItems  int     // Total number of items
	Success     bool    // Whether this operation succeeded
	Message     string  // Result message
	CurrentStep string  // Description of current step (e.g., "Restoring nvim-config")
	Progress    float64 // Overall progress (0.0 to 1.0)
}

// BatchCompleteMsg is sent when the entire batch operation completes.
type BatchCompleteMsg struct {
	Results      []ResultItem // Results for all operations
	SuccessCount int          // Count of successful operations
	FailCount    int          // Count of failed operations
}

// executeBatchRestore executes restore operations for all selected items.
// Returns a command that processes items sequentially and sends progress updates.
//
//nolint:unused // Will be used in Task 16
func (m Model) executeBatchRestore() tea.Cmd {
	// Collect all selected items to restore
	var items []struct {
		appIdx int
		subIdx int
		name   string
	}

	// Add selected apps (all sub-entries)
	for appIdx := range m.selectedApps {
		if appIdx >= 0 && appIdx < len(m.Applications) {
			app := m.Applications[appIdx]
			for subIdx := range app.SubItems {
				items = append(items, struct {
					appIdx int
					subIdx int
					name   string
				}{
					appIdx: appIdx,
					subIdx: subIdx,
					name:   app.Application.Name + "/" + app.SubItems[subIdx].SubEntry.Name,
				})
			}
		}
	}

	// Add standalone selected sub-entries
	for key := range m.selectedSubEntries {
		var appIdx, subIdx int
		if _, err := fmt.Sscanf(key, "%d:%d", &appIdx, &subIdx); err != nil {
			continue
		}

		// Skip if parent app is selected (already added above)
		if m.selectedApps[appIdx] {
			continue
		}

		if appIdx >= 0 && appIdx < len(m.Applications) &&
			subIdx >= 0 && subIdx < len(m.Applications[appIdx].SubItems) {
			app := m.Applications[appIdx]
			items = append(items, struct {
				appIdx int
				subIdx int
				name   string
			}{
				appIdx: appIdx,
				subIdx: subIdx,
				name:   app.Application.Name + "/" + app.SubItems[subIdx].SubEntry.Name,
			})
		}
	}

	// Execute restore operations sequentially
	return func() tea.Msg {
		results := make([]ResultItem, 0, len(items))
		successCount := 0
		failCount := 0

		for _, item := range items {
			// Get sub-entry data
			subItem := &m.Applications[item.appIdx].SubItems[item.subIdx]

			// Perform restore
			success, message := m.performRestoreSubEntry(subItem.SubEntry, subItem.Target)

			// Record result
			results = append(results, ResultItem{
				Name:    item.name,
				Success: success,
				Message: message,
			})

			if success {
				successCount++
			} else {
				failCount++
			}

			// Send progress update (not final message)
			// Note: In a real implementation, we would send BatchOperationMsg here
			// For now, we'll just collect results and send BatchCompleteMsg at the end
		}

		return BatchCompleteMsg{
			Results:      results,
			SuccessCount: successCount,
			FailCount:    failCount,
		}
	}
}

// executeBatchInstall executes package installation for all selected apps.
// Returns a command that processes packages sequentially.
//
//nolint:unused // Will be used in Task 16
func (m Model) executeBatchInstall() tea.Cmd {
	// Collect all selected apps with packages to install
	var packages []PackageItem

	for appIdx := range m.selectedApps {
		if appIdx >= 0 && appIdx < len(m.Applications) {
			app := m.Applications[appIdx]

			// Only install if package exists and is not already installed
			if app.PkgInstalled != nil && !*app.PkgInstalled && app.Application.HasPackage() {
				// Convert Application to PackageItem
				pkg := PackageItem{
					Entry: config.Entry{
						Name:    app.Application.Name,
						Package: app.Application.Package,
					},
					Method:   app.PkgMethod,
					Selected: true,
				}
				packages = append(packages, pkg)
			}
		}
	}

	// If no packages to install, return complete immediately
	if len(packages) == 0 {
		return func() tea.Msg {
			return BatchCompleteMsg{
				Results:      []ResultItem{},
				SuccessCount: 0,
				FailCount:    0,
			}
		}
	}

	// Set pending packages for sequential installation
	// Note: This uses the existing package installation logic
	// The actual installation will be handled by installNextPackage
	return func() tea.Msg {
		// Signal that we need to start package installation
		// This will be handled in the Update method
		return initBatchInstallMsg{packages: packages}
	}
}

// initBatchInstallMsg is an internal message to initialize batch package installation.
//
//nolint:unused // Will be used in Task 16
type initBatchInstallMsg struct {
	packages []PackageItem
}

// executeBatchDelete executes delete operations for all selected items.
// Returns a command that processes deletions in reverse order.
//
//nolint:unused // Will be used in Task 16
func (m Model) executeBatchDelete() tea.Cmd {
	// Collect all items to delete
	type deleteItem struct {
		appIdx int
		subIdx int // -1 for app deletion
		name   string
	}

	var items []deleteItem

	// Add selected apps (entire app deletion)
	for appIdx := range m.selectedApps {
		if appIdx >= 0 && appIdx < len(m.Applications) {
			items = append(items, deleteItem{
				appIdx: appIdx,
				subIdx: -1,
				name:   m.Applications[appIdx].Application.Name,
			})
		}
	}

	// Add standalone selected sub-entries
	for key := range m.selectedSubEntries {
		var appIdx, subIdx int
		if _, err := fmt.Sscanf(key, "%d:%d", &appIdx, &subIdx); err != nil {
			continue
		}

		// Skip if parent app is selected (will be deleted with app)
		if m.selectedApps[appIdx] {
			continue
		}

		if appIdx >= 0 && appIdx < len(m.Applications) &&
			subIdx >= 0 && subIdx < len(m.Applications[appIdx].SubItems) {
			items = append(items, deleteItem{
				appIdx: appIdx,
				subIdx: subIdx,
				name:   m.Applications[appIdx].Application.Name + "/" + m.Applications[appIdx].SubItems[subIdx].SubEntry.Name,
			})
		}
	}

	// Execute deletions
	return func() tea.Msg {
		results := make([]ResultItem, 0, len(items))
		successCount := 0
		failCount := 0

		// Process deletions in reverse order to avoid index shifting
		// (This will be implemented in Task 17)
		for i := len(items) - 1; i >= 0; i-- {
			item := items[i]

			// Placeholder - actual deletion will be implemented in Task 17
			var err error
			if item.subIdx >= 0 {
				err = m.deleteSubEntry(item.appIdx, item.subIdx)
			} else {
				err = m.deleteApplication(item.appIdx)
			}

			success := err == nil
			message := ""
			if success {
				message = "Deleted successfully"
				successCount++
			} else {
				message = fmt.Sprintf("Failed: %v", err)
				failCount++
			}

			results = append([]ResultItem{{
				Name:    item.name,
				Success: success,
				Message: message,
			}}, results...) // Prepend to maintain original order
		}

		return BatchCompleteMsg{
			Results:      results,
			SuccessCount: successCount,
			FailCount:    failCount,
		}
	}
}

// initBatchProgress initializes the progress bar model for batch operations.
//
//nolint:unused // Will be used in Task 15
func initBatchProgress() progress.Model {
	prog := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(60),
	)
	return prog
}
