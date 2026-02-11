package tui

import "fmt"

// Selection helper methods

// toggleAppSelection toggles the selection state of an entire application and all its sub-entries.
// When selecting an app, all its sub-entries are selected. When deselecting, all are deselected.
func (m *Model) toggleAppSelection(appIdx int) {
	if appIdx < 0 || appIdx >= len(m.Applications) {
		return
	}

	// Toggle the app selection state
	newState := !m.selectedApps[appIdx]
	m.selectedApps[appIdx] = newState

	// Toggle all sub-entries to match
	for subIdx := range m.Applications[appIdx].SubItems {
		key := m.makeSubEntryKey(appIdx, subIdx)
		m.selectedSubEntries[key] = newState
	}

	// Clean up maps if deselecting
	if !newState {
		delete(m.selectedApps, appIdx)
		for subIdx := range m.Applications[appIdx].SubItems {
			key := m.makeSubEntryKey(appIdx, subIdx)
			delete(m.selectedSubEntries, key)
		}
	}

	m.updateMultiSelectActive()
}

// toggleSubEntrySelection toggles the selection state of a single sub-entry within an application.
func (m *Model) toggleSubEntrySelection(appIdx, subIdx int) {
	if appIdx < 0 || appIdx >= len(m.Applications) {
		return
	}
	if subIdx < 0 || subIdx >= len(m.Applications[appIdx].SubItems) {
		return
	}

	key := m.makeSubEntryKey(appIdx, subIdx)

	// Toggle the sub-entry selection state
	newState := !m.selectedSubEntries[key]
	m.selectedSubEntries[key] = newState

	// Clean up map if deselecting
	if !newState {
		delete(m.selectedSubEntries, key)
	}

	m.updateMultiSelectActive()
}

// clearSelections clears all selection state, resetting to no selections.
func (m *Model) clearSelections() {
	m.selectedApps = make(map[int]bool)
	m.selectedSubEntries = make(map[string]bool)
	m.multiSelectActive = false
}

// makeSubEntryKey creates a unique key for a sub-entry using the format "appIdx:subIdx".
func (m *Model) makeSubEntryKey(appIdx, subIdx int) string {
	return fmt.Sprintf("%d:%d", appIdx, subIdx)
}

// updateMultiSelectActive updates the multiSelectActive flag based on current selections.
// It sets the flag to true if any selections exist, false otherwise.
func (m *Model) updateMultiSelectActive() {
	m.multiSelectActive = len(m.selectedApps) > 0 || len(m.selectedSubEntries) > 0
}

// isAppSelected returns true if the application at appIdx is selected.
func (m *Model) isAppSelected(appIdx int) bool {
	return m.selectedApps[appIdx]
}

// isSubEntrySelected returns true if the sub-entry is selected.
// A sub-entry is considered selected if it's explicitly selected OR if its parent app is selected.
func (m *Model) isSubEntrySelected(appIdx, subIdx int) bool {
	// Check if parent app is selected (implicit selection)
	if m.selectedApps[appIdx] {
		return true
	}

	// Check if sub-entry is explicitly selected
	key := m.makeSubEntryKey(appIdx, subIdx)
	return m.selectedSubEntries[key]
}

// getSelectionCounts returns the count of selected apps and independent sub-entries.
// Independent sub-entries are those selected without their parent app being selected.
func (m *Model) getSelectionCounts() (appCount int, subEntryCount int) {
	appCount = len(m.selectedApps)

	// Count sub-entries that are NOT under a selected app
	for key := range m.selectedSubEntries {
		var appIdx, subIdx int
		if _, err := fmt.Sscanf(key, "%d:%d", &appIdx, &subIdx); err != nil {
			continue // Skip malformed keys
		}

		// Only count if parent app is not selected
		if !m.selectedApps[appIdx] {
			subEntryCount++
		}
	}

	return appCount, subEntryCount
}

// countHiddenSelections returns the number of selected items that would be hidden
// when filter is enabled. Used to determine if a confirmation dialog is needed.
func (m *Model) countHiddenSelections() int {
	count := 0

	// Count selected apps that are filtered
	for appIdx := range m.selectedApps {
		if appIdx >= 0 && appIdx < len(m.Applications) && m.Applications[appIdx].IsFiltered {
			count++
		}
	}

	// Count selected sub-entries under filtered apps
	for key := range m.selectedSubEntries {
		var appIdx, subIdx int
		if _, err := fmt.Sscanf(key, "%d:%d", &appIdx, &subIdx); err != nil {
			continue
		}

		if appIdx >= 0 && appIdx < len(m.Applications) && m.Applications[appIdx].IsFiltered {
			count++
		}
	}

	return count
}

// clearHiddenSelections removes selections for apps where IsFiltered=true.
// Called after toggling filter ON to keep selections in sync with visible items.
func (m *Model) clearHiddenSelections() {
	// Remove filtered apps from selected apps
	for appIdx := range m.selectedApps {
		if appIdx >= 0 && appIdx < len(m.Applications) && m.Applications[appIdx].IsFiltered {
			delete(m.selectedApps, appIdx)
		}
	}

	// Remove sub-entries under filtered apps
	for key := range m.selectedSubEntries {
		var appIdx, subIdx int
		if _, err := fmt.Sscanf(key, "%d:%d", &appIdx, &subIdx); err != nil {
			continue
		}

		if appIdx >= 0 && appIdx < len(m.Applications) && m.Applications[appIdx].IsFiltered {
			delete(m.selectedSubEntries, key)
		}
	}

	// Update multiSelectActive flag
	m.updateMultiSelectActive()
}

// moveToNextExpandedNode moves the cursor to the next expanded node in the table.
// It wraps around to the beginning if it reaches the end.
func (m *Model) moveToNextExpandedNode() {
	if len(m.tableRows) == 0 {
		return
	}

	// Move to next row
	m.tableCursor++
	if m.tableCursor >= len(m.tableRows) {
		m.tableCursor = 0
	}
}
