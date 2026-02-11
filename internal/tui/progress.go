package tui

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/packages"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// pkgCheckResultMsg is sent when a single package install check completes.
type pkgCheckResultMsg struct {
	appIndex  int
	method    string
	installed bool
}

// stateCheckResultMsg is sent when a single sub-entry state check completes.
type stateCheckResultMsg struct {
	appIndex int
	subIndex int
	state    PathState
}

// initApplicationItems creates ApplicationItem list from v3 config
func (m *Model) initApplicationItems() {
	// Get ALL applications, not just filtered ones
	apps := m.Config.Applications

	m.Applications = make([]ApplicationItem, 0, len(apps))

	for _, app := range apps {
		// Check if this app matches the when expression
		isFiltered := !config.EvaluateWhen(app.When, m.Renderer)

		subItems := make([]SubEntryItem, 0, len(app.Entries))

		for _, subEntry := range app.Entries {
			target := subEntry.GetTarget(m.Platform.OS)
			if target == "" {
				continue
			}

			// Expand ~ and env vars in target path for file operations
			expandedTarget := config.ExpandPath(target, m.Platform.EnvVars)

			subItem := SubEntryItem{
				SubEntry: subEntry,
				Target:   expandedTarget,
				Selected: true,
				AppName:  app.Name,
			}

			subItems = append(subItems, subItem)
		}

		// Skip apps with no applicable entries AND no packages
		if len(subItems) == 0 && !app.HasPackage() {
			continue
		}

		appItem := ApplicationItem{
			Application: app,
			Selected:    true,
			Expanded:    false,
			SubItems:    subItems,
			IsFiltered:  isFiltered,
		}

		// Package method and install check deferred to async

		m.Applications = append(m.Applications, appItem)
	}

	// Sort applications alphabetically by name
	sort.Slice(m.Applications, func(i, j int) bool {
		return m.Applications[i].Application.Name < m.Applications[j].Application.Name
	})

	// Initialize table model with the loaded applications
	m.initTableModel()
}

// reinitPreservingState rebuilds application items from config while preserving
// existing states for all apps except the edited one. The edited app gets its
// sub-entry states synchronously refreshed. Pass empty string to preserve all states.
func (m *Model) reinitPreservingState(editedAppName string) {
	type savedAppState struct {
		subStates    map[string]PathState // subEntry name -> state
		pkgMethod    string
		pkgInstalled *bool
		expanded     bool
	}

	// Save existing states by app name
	saved := make(map[string]savedAppState)
	for _, app := range m.Applications {
		subStates := make(map[string]PathState)
		for _, sub := range app.SubItems {
			subStates[sub.SubEntry.Name] = sub.State
		}
		saved[app.Application.Name] = savedAppState{
			subStates:    subStates,
			pkgMethod:    app.PkgMethod,
			pkgInstalled: app.PkgInstalled,
			expanded:     app.Expanded,
		}
	}

	// Rebuild from config
	m.initApplicationItems()

	// Restore preserved states
	for i, app := range m.Applications {
		prev, ok := saved[app.Application.Name]
		if !ok {
			// New app (e.g., just created) - synchronously detect states
			for j := range m.Applications[i].SubItems {
				m.Applications[i].SubItems[j].State = m.detectSubEntryState(&m.Applications[i].SubItems[j])
			}
			continue
		}

		// Restore expanded and package states for all apps
		m.Applications[i].Expanded = prev.expanded
		m.Applications[i].PkgMethod = prev.pkgMethod
		m.Applications[i].PkgInstalled = prev.pkgInstalled

		if app.Application.Name == editedAppName {
			// For the edited app, synchronously refresh sub-entry states
			for j := range m.Applications[i].SubItems {
				m.Applications[i].SubItems[j].State = m.detectSubEntryState(&m.Applications[i].SubItems[j])
			}
		} else {
			// For other apps, restore previous sub-entry states
			for j, sub := range app.SubItems {
				if state, exists := prev.subStates[sub.SubEntry.Name]; exists {
					m.Applications[i].SubItems[j].State = state
				}
			}
		}
	}

	// Rebuild table with restored states
	m.initTableModel()
}

// refreshApplicationStates updates the state of all sub-entry items
func (m *Model) refreshApplicationStates() {
	for i := range m.Applications {
		for j := range m.Applications[i].SubItems {
			m.Applications[i].SubItems[j].State = m.detectSubEntryState(&m.Applications[i].SubItems[j])
		}
	}
}

// getApplicationAtCursorFromTable returns the application and sub-entry indices from table cursor
func (m *Model) getApplicationAtCursorFromTable() (int, int) {
	if m.tableCursor < 0 || m.tableCursor >= len(m.tableRows) {
		return -1, -1
	}

	tableRow := m.tableRows[m.tableCursor]

	// Look up the real index in m.Applications by name
	realAppIdx := -1
	for i, app := range m.Applications {
		if app.Application.Name == tableRow.AppName {
			realAppIdx = i
			break
		}
	}

	if realAppIdx == -1 {
		return -1, -1
	}

	return realAppIdx, tableRow.SubIndex
}

func (m Model) viewProgress() string {
	var b strings.Builder

	// Title
	title := fmt.Sprintf("⏳  %s in progress...", m.Operation.String())
	b.WriteString(TitleStyle.Render(title))
	b.WriteString("\n\n")

	// Show current item if batch operation
	if m.batchTotalItems > 0 {
		// Progress counter
		progressText := fmt.Sprintf("Processing %d of %d items", m.batchCurrentIndex+1, m.batchTotalItems)
		b.WriteString(SubtitleStyle.Render(progressText))
		b.WriteString("\n\n")

		// Current item being processed
		if m.batchCurrentItem != "" {
			b.WriteString(PathNameStyle.Render("Current: "))
			b.WriteString(m.batchCurrentItem)
			b.WriteString("\n\n")
		}

		// Progress bar
		percent := float64(m.batchCurrentIndex) / float64(m.batchTotalItems)
		b.WriteString(m.batchProgress.ViewAs(percent))
		b.WriteString("\n\n")

		// Stats
		statsText := fmt.Sprintf("✓ %d successful  ✗ %d failed", m.batchSuccessCount, m.batchFailCount)
		b.WriteString(MutedTextStyle.Render(statsText))
		b.WriteString("\n")
	} else {
		// Fallback for non-batch operations
		b.WriteString(SpinnerStyle.Render("Processing..."))
		b.WriteString("\n")
	}

	return BaseStyle.Render(b.String())
}

//nolint:gocyclo // UI handler with many states
func (m Model) updateResults(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle search mode input
	if m.Operation == OpList && m.searching {
		switch {
		case key.Matches(msg, SearchKeys.Cancel):
			// Clear search and exit search mode
			m.searching = false
			m.searchText = ""
			m.searchInput.SetValue("")
			m.searchInput.Blur()
			// Rebuild table without search
			m.rebuildTable()

			return m, nil
		case key.Matches(msg, SearchKeys.Confirm):
			// Confirm search and return to navigation mode
			m.searching = false
			m.searchInput.Blur()

			return m, nil
		default:
			// Pass key to text input
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.searchText = m.searchInput.Value()
			// Rebuild table with new search
			m.rebuildTable()

			return m, cmd
		}
	}

	// Handle filter toggle confirmation
	if m.Operation == OpList && m.confirmingFilterToggle {
		switch {
		case key.Matches(msg, ConfirmKeys.Yes):
			// Confirm - toggle filter and clear hidden selections
			m.confirmingFilterToggle = false
			m.filterToggleHiddenCount = 0
			m.filterEnabled = true
			m.clearHiddenSelections()
			m.rebuildTable()
			return m, nil
		case key.Matches(msg, ConfirmKeys.No):
			// Cancel - keep filter off
			m.confirmingFilterToggle = false
			m.filterToggleHiddenCount = 0
			return m, nil
		}
		return m, nil
	}

	// Handle delete confirmation
	if m.Operation == OpList && (m.confirmingDeleteApp || m.confirmingDeleteSubEntry) {
		switch {
		case key.Matches(msg, ConfirmKeys.Yes):
			// Confirm delete
			appIdx, subIdx := m.getApplicationAtCursorFromTable()
			if m.confirmingDeleteApp && appIdx >= 0 {
				m.confirmingDeleteApp = false
				if err := m.deleteApplication(appIdx); err != nil {
					m.err = err
				} else {
					// Rebuild table after deletion
					m.rebuildTable()
				}
			} else if m.confirmingDeleteSubEntry && appIdx >= 0 && subIdx >= 0 {
				m.confirmingDeleteSubEntry = false
				if err := m.deleteSubEntry(appIdx, subIdx); err != nil {
					m.err = err
				} else {
					// Rebuild table after deletion
					m.rebuildTable()
				}
			}

			return m, nil
		case key.Matches(msg, ConfirmKeys.No):
			// Cancel delete
			m.confirmingDeleteApp = false
			m.confirmingDeleteSubEntry = false

			return m, nil
		}

		return m, nil
	}

	// Handle diff picker separately
	if m.Operation == OpList && m.showingDiffPicker {
		return m.updateDiffPicker(msg)
	}

	// Handle detail popup separately
	if m.Operation == OpList && m.showingDetail {
		if m, cmd, handled := m.handleCommonKeys(msg); handled {
			return m, cmd
		}

		switch {
		case key.Matches(msg, DetailKeys.Close):
			// Close detail popup (ESC cancels/closes the popup)
			m.showingDetail = false
			return m, nil
		}

		return m, nil
	}

	// Handle ESC to clear active search or selections (when not in search mode but search text or selections are present)
	if m.Operation == OpList && key.Matches(msg, FormNavKeys.Cancel) && !m.searching {
		// Priority 1: Clear search first if active
		if m.searchText != "" {
			m.searchText = ""
			m.searchInput.SetValue("")
			m.rebuildTable()
			return m, nil
		}
		// Priority 2: Clear selections if any exist
		if m.multiSelectActive {
			m.clearSelections()
			return m, nil
		}
	}

	// Helper to check if we're in a clean list state (no modals/search)
	listClean := m.Operation == OpList && !m.searching && !m.confirmingDeleteApp && !m.confirmingDeleteSubEntry && !m.showingDetail

	switch {
	case key.Matches(msg, ListKeys.Search):
		// Enter search mode
		if listClean {
			m.searching = true
			m.searchInput.Focus()

			return m, nil
		}
	case key.Matches(msg, ListKeys.SortByName):
		// Sort by name
		if listClean {
			if m.sortColumn == SortColumnName {
				m.sortAscending = !m.sortAscending
			} else {
				m.sortColumn = SortColumnName
				m.sortAscending = true
			}
			m.rebuildTable()
			return m, nil
		}
	case key.Matches(msg, ListKeys.SortByStatus):
		// Sort by status
		if listClean {
			if m.sortColumn == SortColumnStatus {
				m.sortAscending = !m.sortAscending
			} else {
				m.sortColumn = SortColumnStatus
				m.sortAscending = true
			}
			m.rebuildTable()
			return m, nil
		}
	case key.Matches(msg, ListKeys.SortByPath):
		// Sort by path
		if listClean {
			if m.sortColumn == SortColumnPath {
				m.sortAscending = !m.sortAscending
			} else {
				m.sortColumn = SortColumnPath
				m.sortAscending = true
			}
			m.rebuildTable()
			return m, nil
		}
	case key.Matches(msg, ListKeys.Filter):
		// Toggle filter
		if listClean {
			// If toggling filter ON (false -> true), check if selections would be hidden
			if !m.filterEnabled && m.multiSelectActive {
				hiddenCount := m.countHiddenSelections()
				if hiddenCount > 0 {
					// Show confirmation dialog - set a new state flag
					m.confirmingFilterToggle = true
					m.filterToggleHiddenCount = hiddenCount
					return m, nil
				}
			}

			// Toggle filter (no confirmation needed or toggling OFF)
			wasEnabled := m.filterEnabled
			m.filterEnabled = !m.filterEnabled
			m.rebuildTable()

			// When turning filter OFF, scan any filtered apps that haven't been checked yet
			if wasEnabled {
				return m, m.checkFilteredStatesCmd()
			}

			return m, nil
		}
	case key.Matches(msg, SharedKeys.Quit):
		// Quit the application
		return m, tea.Quit
	case key.Matches(msg, ListKeys.Up):
		if m.Operation == OpList {
			// Clear any previous restore results when navigating
			m.results = nil
			// Move cursor up
			if m.tableCursor > 0 {
				m.tableCursor--
				m.updateScrollOffset()
			}
			return m, nil
		}

		return m, nil
	case key.Matches(msg, ListKeys.Down):
		if m.Operation == OpList {
			// Clear any previous restore results when navigating
			m.results = nil
			// Move cursor down
			if m.tableCursor < len(m.tableRows)-1 {
				m.tableCursor++
				m.updateScrollOffset()
			}
			return m, nil
		}

		return m, nil
	case key.Matches(msg, ListKeys.Collapse):
		if m.Operation == OpList {
			// Clear any previous restore results when navigating
			m.results = nil
			// Collapse node if expanded
			appIdx, _ := m.getApplicationAtCursorFromTable()
			if appIdx >= 0 && m.Applications[appIdx].Expanded {
				m.Applications[appIdx].Expanded = false
				// Rebuild table to reflect collapsed state
				m.rebuildTable()
			}
			// If not expanded, 'h' does nothing (use 'q' to go back to menu)

			return m, nil
		}

		return m, tea.Quit
	case key.Matches(msg, ListKeys.Expand):
		if m.Operation == OpList {
			// Clear any previous restore results when navigating
			m.results = nil
			// If showing detail, close it; otherwise expand (not toggle)
			if m.showingDetail {
				m.showingDetail = false
			} else {
				appIdx, subIdx := m.getApplicationAtCursorFromTable()
				if appIdx >= 0 && subIdx < 0 {
					// Only expand application rows, not sub-entries
					m.Applications[appIdx].Expanded = true
					// Rebuild table to show expanded children
					m.rebuildTable()
				}
			}

			return m, nil
		}

		return m, tea.Quit
	case key.Matches(msg, ListKeys.Edit):
		// Edit selected Application or SubEntry (only in List view)
		if m.Operation == OpList {
			appIdx, subIdx := m.getApplicationAtCursorFromTable()
			if appIdx >= 0 {
				if subIdx >= 0 {
					// Edit SubEntry
					m.initSubEntryFormEdit(appIdx, subIdx)
				} else {
					// Edit Application
					m.initApplicationFormEdit(appIdx)
				}

				return m, nil
			}
		}
	case key.Matches(msg, ListKeys.AddApp):
		// Add new Application (only in List view)
		if m.Operation == OpList {
			m.initApplicationFormNew()
			return m, nil
		}
	case key.Matches(msg, ListKeys.AddEntry):
		// Add new SubEntry to current Application (only in List view)
		if m.Operation == OpList {
			appIdx, _ := m.getApplicationAtCursorFromTable()
			if appIdx >= 0 {
				m.initSubEntryFormNew(appIdx)
				return m, nil
			}
		}
	case key.Matches(msg, ListKeys.Delete):
		// Ask for delete confirmation (only in List view)
		if m.Operation == OpList {
			// Check if multi-select mode is active
			if m.multiSelectActive {
				// Show summary screen for batch delete
				m.summaryOperation = OpDelete
				m.Screen = ScreenSummary
				return m, nil
			}

			// Single-item delete (original behavior)
			appIdx, subIdx := m.getApplicationAtCursorFromTable()
			if appIdx >= 0 {
				if subIdx >= 0 {
					m.confirmingDeleteSubEntry = true
				} else {
					m.confirmingDeleteApp = true
				}

				return m, nil
			}
		}
	case key.Matches(msg, ListKeys.Install):
		// Install or Diff depending on context (only in List view)
		if m.Operation == OpList {
			// Check if multi-select mode is active
			if m.multiSelectActive {
				// Show summary screen for batch install
				m.summaryOperation = OpInstallPackages
				m.Screen = ScreenSummary
				return m, nil
			}

			appIdx, subIdx := m.getApplicationAtCursorFromTable()

			// On a modified sub-entry: launch diff viewer
			if appIdx >= 0 && subIdx >= 0 && m.Manager != nil {
				subItem := m.Applications[appIdx].SubItems[subIdx]
				if subItem.State == StateModified {
					backupPath := m.resolvePath(subItem.SubEntry.Backup)
					modifiedFiles, err := m.Manager.GetModifiedTemplateFiles(backupPath)
					if err != nil || len(modifiedFiles) == 0 {
						return m, nil
					}

					if len(modifiedFiles) == 1 {
						// Single file: launch editor directly
						return m, launchDiffEditor(modifiedFiles[0])
					}

					// Multiple files: show picker
					m.showingDiffPicker = true
					m.diffPickerCursor = 0
					m.diffPickerFiles = modifiedFiles
					return m, nil
				}
			}

			// On an app row: install package (original behavior)
			if appIdx >= 0 && subIdx < 0 {
				app := m.Applications[appIdx]
				if app.PkgInstalled != nil && !*app.PkgInstalled {
					m.Operation = OpInstallPackages
					m.currentPackageIndex = 0
					m.results = nil
					m.pendingPackages = []PackageItem{{
						Name:     app.Application.Name,
						Package:  app.Application.Package,
						Method:   app.PkgMethod,
						Selected: true,
					}}
					m.Screen = ScreenProgress
					return m, m.installNextPackage()
				}
			}
		}

		return m, nil
	case key.Matches(msg, ListKeys.Restore):
		// Restore selected SubEntry (only in List view for SubEntry rows)
		if m.Operation == OpList {
			// Check if multi-select mode is active
			if m.multiSelectActive {
				// Show summary screen for batch restore
				m.summaryOperation = OpRestore
				m.Screen = ScreenSummary
				return m, nil
			}

			// Single-item restore (original behavior)
			appIdx, subIdx := m.getApplicationAtCursorFromTable()
			if appIdx >= 0 && subIdx >= 0 {
				// Restore single sub-entry
				subItem := &m.Applications[appIdx].SubItems[subIdx]
				success, message := m.performRestoreSubEntry(subItem.SubEntry, subItem.Target)
				if success {
					m.Applications[appIdx].SubItems[subIdx].State = m.detectSubEntryState(subItem)
					m.rebuildTable()
				}
				m.results = []ResultItem{{
					Name:    subItem.SubEntry.Name,
					Success: success,
					Message: message,
				}}
			} else if appIdx >= 0 && subIdx < 0 {
				// Restore all sub-entries for this application
				m.results = nil
				for i := range m.Applications[appIdx].SubItems {
					subItem := &m.Applications[appIdx].SubItems[i]
					if !subItem.SubEntry.IsConfig() {
						continue
					}
					success, message := m.performRestoreSubEntry(subItem.SubEntry, subItem.Target)
					if success {
						m.Applications[appIdx].SubItems[i].State = m.detectSubEntryState(subItem)
					}
					m.results = append(m.results, ResultItem{
						Name:    subItem.SubEntry.Name,
						Success: success,
						Message: message,
					})
				}
				m.rebuildTable()
			}
		}

		return m, nil
	case key.Matches(msg, ListKeys.Toggle):
		// Toggle selection and advance cursor (only in List view)
		if listClean {
			appIdx, subIdx := m.getApplicationAtCursorFromTable()
			if appIdx >= 0 {
				if subIdx >= 0 {
					// Toggle sub-entry selection
					m.toggleSubEntrySelection(appIdx, subIdx)
				} else {
					// Toggle application selection
					m.toggleAppSelection(appIdx)
				}
				// Move to next row
				m.moveToNextExpandedNode()
			}
			return m, nil
		}
	}

	return m, nil
}

func (m Model) viewResults() string {
	// Use table view for List operation
	if m.Operation == OpList {
		// Always use hierarchical viewListTable (supports both v2 and v3)
		return m.viewListTable()
	}

	var b strings.Builder

	// Title
	title := fmt.Sprintf("✓  %s Complete", m.Operation.String())
	b.WriteString(TitleStyle.Render(title))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(ErrorStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		b.WriteString("\n\n")
	}

	// Results summary
	successCount := 0
	failCount := 0

	for _, r := range m.results {
		if r.Success {
			successCount++
		} else {
			failCount++
		}
	}

	summary := fmt.Sprintf("%d successful", successCount)
	if failCount > 0 {
		summary += fmt.Sprintf(", %d failed", failCount)
	}

	if m.DryRun {
		summary = WarningStyle.Render("[DRY RUN] ") + summary
	}

	b.WriteString(SubtitleStyle.Render(summary))
	b.WriteString("\n\n")

	// Results list
	maxVisible := m.viewHeight
	if maxVisible > len(m.results) {
		maxVisible = len(m.results)
	}

	start := m.scrollOffset

	end := start + maxVisible
	if end > len(m.results) {
		end = len(m.results)

		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	topIndicator, bottomIndicator := RenderScrollIndicators(start, end, len(m.results))
	b.WriteString(topIndicator)

	for i := start; i < end; i++ {
		result := m.results[i]

		var icon string
		var nameStyle func(string) string

		if result.Success {
			icon = SuccessStyle.Render("✓ ")
			nameStyle = func(s string) string { return SuccessStyle.Render(s) }
		} else {
			icon = ErrorStyle.Render("✗ ")
			nameStyle = func(s string) string { return ErrorStyle.Render(s) }
		}

		b.WriteString(icon + nameStyle(result.Name))
		b.WriteString("\n")

		// Show message indented
		if result.Message != "" {
			lines := strings.Split(result.Message, "\n")
			for _, line := range lines {
				b.WriteString(IndentSpaces + SubtitleStyle.Render(line))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString(bottomIndicator)

	// Help
	b.WriteString("\n")
	b.WriteString(RenderHelpFromBindings(m.width,
		ListKeys.NewOperation,
		ListKeys.QuitOrEnter,
	))

	return BaseStyle.Render(b.String())
}

// renderHelpForCurrentState returns the help text for the current screen state.
// This allows us to measure the help text height before rendering.
func (m Model) renderHelpForCurrentState() string {
	appIdx, subIdx := m.getApplicationAtCursorFromTable()

	switch {
	case m.confirmingDeleteApp || m.confirmingDeleteSubEntry:
		// Delete confirmation prompt
		var name string
		switch {
		case m.confirmingDeleteApp && appIdx >= 0:
			name = m.Applications[appIdx].Application.Name
		case m.confirmingDeleteSubEntry && appIdx >= 0 && subIdx >= 0:
			name = m.Applications[appIdx].SubItems[subIdx].SubEntry.Name
		}

		if name != "" {
			return WarningStyle.Render(fmt.Sprintf("Delete '%s'? ", name)) +
				RenderHelpFromBindings(m.width, ConfirmKeys.Yes, ConfirmKeys.No)
		}
		return RenderHelpFromBindings(m.width, ConfirmKeys.Yes, ConfirmKeys.No)

	case m.confirmingFilterToggle:
		// Filter toggle confirmation dialog
		itemText := "item(s)"
		if m.filterToggleHiddenCount == 1 {
			itemText = "item"
		}
		prompt := fmt.Sprintf("Enabling filter will hide %d selected %s. Continue? (y/n)",
			m.filterToggleHiddenCount, itemText)
		return WarningStyle.Render(prompt)

	case m.searching:
		return RenderHelpFromBindings(m.width,
			SearchKeys.Confirm,
			SearchKeys.Cancel,
		)

	case m.showingDiffPicker:
		return RenderHelpFromBindings(m.width,
			DiffPickerKeys.Select,
			DiffPickerKeys.Cancel,
		)

	case m.showingDetail:
		return RenderHelpFromBindings(m.width,
			DetailKeys.Close,
			SharedKeys.Quit,
		)

	default:
		// Build help text based on cursor position and multi-select mode
		if m.multiSelectActive {
			return RenderHelpFromBindings(m.width,
				MultiSelectKeys.Toggle,
				MultiSelectKeys.Clear,
				MultiSelectKeys.Restore,
				MultiSelectKeys.Install,
				MultiSelectKeys.Delete,
				SharedKeys.Quit,
			)
		}

		// Normal mode help text
		bindings := []key.Binding{
			ListKeys.Search,
			ListKeys.AddApp,
			ListKeys.AddEntry,
			ListKeys.Edit,
			ListKeys.Delete,
			ListKeys.Restore,
		}

		// Show context-sensitive "i" help
		if subIdx < 0 {
			// App row: install
			bindings = append(bindings, ListKeys.Install)
		} else if appIdx >= 0 && subIdx >= 0 && appIdx < len(m.Applications) &&
			subIdx < len(m.Applications[appIdx].SubItems) &&
			m.Applications[appIdx].SubItems[subIdx].State == StateModified {
			// Modified sub-entry: diff
			diffBinding := key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "diff"))
			bindings = append(bindings, diffBinding)
		}

		bindings = append(bindings, SharedKeys.Quit)
		return RenderHelpFromBindings(m.width, bindings...)
	}
}

//nolint:gocyclo // UI rendering with many states
func (m Model) viewListTable() string {
	var b strings.Builder
	linesUsed := 0

	// Filter banner (always shown) with search input right-aligned on the same line
	highlightedF := lipgloss.NewStyle().
		Foreground(accentColor).
		Bold(true).
		Render("f")

	var filterBanner string
	if m.filterEnabled {
		visibleCount := 0
		totalCount := len(m.Applications)

		for _, app := range m.Applications {
			if !app.IsFiltered {
				visibleCount++
			}
		}

		countInfo := MutedTextStyle.Render(fmt.Sprintf(" (showing %d of %d apps)", visibleCount, totalCount))
		filterBanner = "  " + highlightedF + "ilter: on" + countInfo
	} else {
		filterBanner = "  " + highlightedF + "ilter: off"
	}

	// Append search input after the filter banner on the same line
	if m.searching || m.searchText != "" {
		var searchPart string
		if m.searching {
			searchPart = "/ " + m.searchInput.View()
		} else {
			searchPart = "/ " + FilterInputStyle.Render(m.searchText)
		}

		filterBanner += "    " + searchPart
	}

	b.WriteString(filterBanner)
	b.WriteString("\n")
	linesUsed++

	// Table should already be initialized via Update()/initTableModel()
	// Do not call initTableModel() from View() — mutating state in View is a Bubble Tea anti-pattern

	// Count lines after table
	linesAfterTable := 1 // Blank line or multi-select banner after table

	// Add line for multi-select banner if active
	// (The banner replaces the blank line, so no additional line needed)

	// Detail panel
	appIdx, subIdx := m.getApplicationAtCursorFromTable()
	var detailContent string
	if m.showingDetail && appIdx >= 0 {
		if subIdx >= 0 {
			detailContent = m.renderSubEntryInlineDetail(&m.Applications[appIdx].SubItems[subIdx], m.width)
		} else {
			filtered := m.getSearchedApplications()
			if appIdx < len(filtered) {
				detailContent = m.renderApplicationInlineDetail(&filtered[appIdx], m.width)
			}
		}
		if detailContent != "" {
			linesAfterTable += strings.Count(detailContent, "\n") + 1
		}
	}

	// Diff picker panel
	var diffPickerContent string
	if m.showingDiffPicker {
		diffPickerContent = m.viewDiffPicker()
		linesAfterTable += strings.Count(diffPickerContent, "\n") + 1
	}

	// Result message
	if len(m.results) > 0 {
		linesAfterTable += 2 // Blank line + result
	}

	// Help footer - measure it
	helpText := m.renderHelpForCurrentState()
	helpLines := strings.Count(helpText, "\n") + 1
	linesAfterTable += 1 + helpLines // Blank line + help

	// Calculate available height for table
	// Subtract 2 for BaseStyle vertical padding (1 top + 1 bottom)
	availableForTable := m.height - linesUsed - linesAfterTable - 2
	if availableForTable < 10 {
		availableForTable = 10 // Minimum table height
	}

	// Render table with exact available height
	b.WriteString(m.renderTable(availableForTable))
	b.WriteString("\n")

	// Multi-select banner (replaces one of two blank lines to keep help position consistent)
	if m.multiSelectActive {
		appCount, subCount := m.getSelectionCounts()
		bannerText := fmt.Sprintf("  %d app(s), %d item(s) selected", appCount, subCount)
		b.WriteString(MultiSelectBannerStyle.Render(bannerText))
	}

	// Detail panel
	if detailContent != "" {
		b.WriteString(detailContent)
		b.WriteString("\n")
	}

	// Diff picker panel
	if diffPickerContent != "" {
		b.WriteString("\n")
		b.WriteString(diffPickerContent)
	}

	// Result message
	if len(m.results) > 0 {
		b.WriteString("\n")
		result := m.results[len(m.results)-1]
		var resultText string
		if result.Success {
			resultText = SuccessStyle.Render(fmt.Sprintf("✓ %s: %s", result.Name, result.Message))
		} else {
			resultText = ErrorStyle.Render(fmt.Sprintf("✗ %s: %s", result.Name, result.Message))
		}
		b.WriteString(resultText)
	}

	// Help footer
	b.WriteString("\n")
	b.WriteString(helpText)

	return BaseStyle.Render(b.String())
}

// getSearchedApplications returns searched applications for hierarchical view
func (m Model) getSearchedApplications() []ApplicationItem {
	if m.searchText == "" {
		return m.Applications
	}

	searchLower := strings.ToLower(m.searchText)
	var searched []ApplicationItem

	for _, app := range m.Applications {
		appMatches := strings.Contains(strings.ToLower(app.Application.Name), searchLower) ||
			strings.Contains(strings.ToLower(app.Application.Description), searchLower)

		// Search SubItems
		var matchingSubItems []SubEntryItem

		for _, sub := range app.SubItems {
			subMatches := strings.Contains(strings.ToLower(sub.SubEntry.Name), searchLower) ||
				strings.Contains(strings.ToLower(sub.Target), searchLower)

			// Check backup field
			subMatches = subMatches || strings.Contains(strings.ToLower(sub.SubEntry.Backup), searchLower)

			if appMatches || subMatches {
				matchingSubItems = append(matchingSubItems, sub)
			}
		}

		if appMatches || len(matchingSubItems) > 0 {
			appCopy := app
			appCopy.SubItems = matchingSubItems
			searched = append(searched, appCopy)
		}
	}

	return searched
}

// findConfigApplicationIndex finds the index of an application in m.Config.Applications by name
// This is needed because m.Applications is sorted but m.Config.Applications is not
func (m *Model) findConfigApplicationIndex(appName string) int {
	for i, app := range m.Config.Applications {
		if app.Name == appName {
			return i
		}
	}

	return -1
}

func (m Model) installNextPackage() tea.Cmd {
	if m.currentPackageIndex >= len(m.pendingPackages) {
		return nil
	}

	pkg := m.pendingPackages[m.currentPackageIndex]

	// Handle dry run
	if m.DryRun {
		return func() tea.Msg {
			return PackageInstallMsg{
				Package: pkg,
				Success: true,
				Message: fmt.Sprintf("Would install via %s", pkg.Method),
			}
		}
	}

	// Build the command
	cmd := m.buildInstallCommand(pkg)
	if cmd == nil {
		return func() tea.Msg {
			return PackageInstallMsg{
				Package: pkg,
				Success: false,
				Message: "No installation method available",
			}
		}
	}

	// Use tea.Exec to properly suspend the TUI and give terminal control to the command.
	// This allows sudo to prompt for password correctly.
	// pauseOnFailExec wraps the command to pause on failure so the user can read error output.
	return tea.Exec(&pauseOnFailExec{cmd: cmd}, func(err error) tea.Msg {
		if err != nil {
			return PackageInstallMsg{
				Package: pkg,
				Success: false,
				Message: fmt.Sprintf("Installation failed: %v", err),
				Err:     err,
			}
		}

		return PackageInstallMsg{
			Package: pkg,
			Success: true,
			Message: fmt.Sprintf("Installed via %s", pkg.Method),
		}
	})
}

func (m Model) buildInstallCommand(pkg PackageItem) *exec.Cmd {
	converted := packages.FromPackageSpec(pkg.Name, pkg.Package)
	if converted == nil {
		return nil
	}

	return packages.BuildCommand(context.Background(), *converted, pkg.Method, m.Platform.OS)
}
