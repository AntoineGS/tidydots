package tui

// Selection helper methods.
//
// Selections are keyed by NAME, not by position. Config validation enforces
// unique application names and unique entry names within an application
// (internal/config/validate.go), so names are a stable identity: they survive
// the view's sort (which reorders a copy), the search filter (which compacts
// a copy), and reinitPreservingState (which rebuilds and re-sorts
// m.Applications after config edits). A position key survives none of those.

// subEntryKey identifies one sub-entry by its application and entry names.
type subEntryKey struct {
	app string
	sub string
}

// toggleAppSelection toggles the selection state of an entire application and all its sub-entries.
// When selecting an app, all its sub-entries are selected. When deselecting, all are deselected.
func (m *Model) toggleAppSelection(appIdx int) {
	if appIdx < 0 || appIdx >= len(m.Applications) {
		return
	}

	app := m.Applications[appIdx]
	name := app.Application.Name

	newState := !m.selectedApps[name]
	m.selectedApps[name] = newState

	for _, sub := range app.SubItems {
		m.selectedSubEntries[subEntryKey{app: name, sub: sub.SubEntry.Name}] = newState
	}

	// Clean up maps if deselecting
	if !newState {
		delete(m.selectedApps, name)
		for _, sub := range app.SubItems {
			delete(m.selectedSubEntries, subEntryKey{app: name, sub: sub.SubEntry.Name})
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

	key := subEntryKey{
		app: m.Applications[appIdx].Application.Name,
		sub: m.Applications[appIdx].SubItems[subIdx].SubEntry.Name,
	}

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
	m.selectedApps = make(map[string]bool)
	m.selectedSubEntries = make(map[subEntryKey]bool)
	m.multiSelectActive = false
}

// updateMultiSelectActive updates the multiSelectActive flag based on current selections.
// It sets the flag to true if any selections exist, false otherwise.
func (m *Model) updateMultiSelectActive() {
	m.multiSelectActive = len(m.selectedApps) > 0 || len(m.selectedSubEntries) > 0
}

// isAppSelected returns true if the named application is selected.
func (m *Model) isAppSelected(appName string) bool {
	return m.selectedApps[appName]
}

// isSubEntrySelected returns true if the sub-entry is selected.
// A sub-entry is considered selected if it's explicitly selected OR if its parent app is selected.
func (m *Model) isSubEntrySelected(appName, subName string) bool {
	if m.selectedApps[appName] {
		return true
	}

	return m.selectedSubEntries[subEntryKey{app: appName, sub: subName}]
}

// getSelectionCounts returns the count of selected apps and independent sub-entries.
// Independent sub-entries are those selected without their parent app being selected.
func (m *Model) getSelectionCounts() (appCount int, subEntryCount int) {
	appCount = len(m.selectedApps)

	for key := range m.selectedSubEntries {
		if !m.selectedApps[key.app] {
			subEntryCount++
		}
	}

	return appCount, subEntryCount
}

// countHiddenSelections returns the number of selected items that would be hidden
// when filter is enabled. Used to determine if a confirmation dialog is needed.
func (m *Model) countHiddenSelections() int {
	count := 0

	for _, app := range m.Applications {
		if !app.IsFiltered {
			continue
		}

		name := app.Application.Name
		if m.selectedApps[name] {
			count++
		}

		for _, sub := range app.SubItems {
			if m.selectedSubEntries[subEntryKey{app: name, sub: sub.SubEntry.Name}] {
				count++
			}
		}
	}

	return count
}

// clearHiddenSelections removes selections for apps where IsFiltered=true.
// Called after toggling filter ON to keep selections in sync with visible items.
func (m *Model) clearHiddenSelections() {
	for _, app := range m.Applications {
		if !app.IsFiltered {
			continue
		}

		name := app.Application.Name
		delete(m.selectedApps, name)

		for _, sub := range app.SubItems {
			delete(m.selectedSubEntries, subEntryKey{app: name, sub: sub.SubEntry.Name})
		}
	}

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
