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
	if app.IsFiltered {
		return
	}
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
	if m.Applications[appIdx].IsFiltered {
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

// prepareCurrentRowSummary selects the row under the cursor for a transient
// summary operation without changing an existing multi-selection.
func (m *Model) prepareCurrentRowSummary(operation Operation) bool {
	appIdx, subIdx := m.getApplicationAtCursorFromTable()
	if appIdx < 0 {
		return false
	}

	app := m.Applications[appIdx]
	if app.IsFiltered {
		return false
	}
	if subIdx >= 0 {
		m.selectedSubEntries[subEntryKey{app: app.Application.Name, sub: app.SubItems[subIdx].SubEntry.Name}] = true
	} else {
		m.selectedApps[app.Application.Name] = true
	}
	m.multiSelectActive = true
	m.summaryTransientSelection = true
	m.summaryOperation = operation
	m.Screen = ScreenSummary
	return true
}

// clearTransientSummarySelection removes only selections created for a
// single-row summary, leaving ordinary multi-selection state untouched.
func (m *Model) clearTransientSummarySelection() {
	if !m.summaryTransientSelection {
		return
	}
	m.clearSelections()
	m.summaryTransientSelection = false
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

// countHiddenActionSelections returns selected items that action filtering
// would remove, without considering items already hidden by machine filtering.
func (m *Model) countHiddenActionSelections() int {
	count := 0
	for _, app := range m.Applications {
		if m.filterEnabled && app.IsFiltered {
			continue
		}

		name := app.Application.Name
		appActionable := applicationPackageActionable(app)
		for _, sub := range app.SubItems {
			if sub.State.Actionable() {
				appActionable = true
			}
		}

		if m.selectedApps[name] {
			if !appActionable {
				count++
			}
			for _, sub := range app.SubItems {
				if !sub.State.Actionable() {
					count++
				}
			}
			continue
		}

		for _, sub := range app.SubItems {
			if m.selectedSubEntries[subEntryKey{app: name, sub: sub.SubEntry.Name}] && !sub.State.Actionable() {
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

// pruneStaleSelections drops selection keys that no longer name a live
// application or sub-entry (the item was renamed or deleted since it was
// selected). Stale name keys cannot retarget another item the way stale
// indices could, but they would inflate the multi-select banner counts.
func (m *Model) pruneStaleSelections() {
	liveApps := make(map[string]bool, len(m.Applications))
	liveSubs := make(map[subEntryKey]bool)

	for _, app := range m.Applications {
		if app.IsFiltered {
			delete(m.selectedApps, app.Application.Name)
			for _, sub := range app.SubItems {
				delete(m.selectedSubEntries, subEntryKey{app: app.Application.Name, sub: sub.SubEntry.Name})
			}
			continue
		}
		name := app.Application.Name
		liveApps[name] = true

		for _, sub := range app.SubItems {
			liveSubs[subEntryKey{app: name, sub: sub.SubEntry.Name}] = true
		}
	}

	for name := range m.selectedApps {
		if !liveApps[name] {
			delete(m.selectedApps, name)
		}
	}

	for key := range m.selectedSubEntries {
		if !liveSubs[key] {
			delete(m.selectedSubEntries, key)
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
