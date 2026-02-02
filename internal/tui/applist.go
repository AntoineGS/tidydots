package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AntoineGS/dot-manager/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

// initApplicationItems creates ApplicationItem list from v3 config
func (m *Model) initApplicationItems() {
	apps := m.Config.GetFilteredApplications(m.FilterCtx)

	m.Applications = make([]ApplicationItem, 0, len(apps))

	for _, app := range apps {
		subItems := make([]SubEntryItem, 0, len(app.Entries))

		for _, subEntry := range app.Entries {
			target := subEntry.GetTarget(m.Platform.OS)
			if target == "" {
				continue
			}

			subItem := SubEntryItem{
				SubEntry: subEntry,
				Target:   target,
				Selected: true,
				AppName:  app.Name,
			}

			subItems = append(subItems, subItem)
		}

		// Skip apps with no applicable entries
		if len(subItems) == 0 {
			continue
		}

		appItem := ApplicationItem{
			Application: app,
			Selected:    true,
			Expanded:    false,
			SubItems:    subItems,
		}

		// Add package info
		if app.HasPackage() {
			spec := config.PackageSpec{
				Name:     app.Name,
				Managers: app.Package.Managers,
				Custom:   app.Package.Custom,
				URL:      app.Package.URL,
			}
			method := getPackageInstallMethod(spec, m.Platform.OS)
			appItem.PkgMethod = method
			if method != "none" {
				installed := isPackageInstalled(spec, method)
				appItem.PkgInstalled = &installed
			}
		}

		m.Applications = append(m.Applications, appItem)
	}

	// Detect states for all sub-items
	m.refreshApplicationStates()
}

// refreshApplicationStates updates the state of all sub-entry items
func (m *Model) refreshApplicationStates() {
	for i := range m.Applications {
		for j := range m.Applications[i].SubItems {
			m.Applications[i].SubItems[j].State = m.detectSubEntryState(&m.Applications[i].SubItems[j])
		}
	}
}

// detectSubEntryState determines the state of a sub-entry item
func (m *Model) detectSubEntryState(item *SubEntryItem) PathState {
	// Similar to detectPathState but for SubEntry
	targetPath := item.Target

	if item.SubEntry.IsGit() {
		// Git entry logic (same as before)
		if pathExists(targetPath) {
			gitDir := filepath.Join(targetPath, ".git")
			if pathExists(gitDir) {
				return StateLinked
			}
			return StateAdopt
		}
		return StateReady
	}

	// Config entry logic
	backupPath := m.resolvePath(item.SubEntry.Backup)

	if item.SubEntry.IsFolder() {
		if info, err := os.Lstat(targetPath); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return StateLinked
			}
		}

		backupExists := pathExists(backupPath)
		targetExists := pathExists(targetPath)

		if backupExists {
			return StateReady
		}
		if targetExists {
			return StateAdopt
		}
		return StateMissing
	}

	// File-based config
	allLinked := true
	anyBackup := false
	anyTarget := false

	for _, file := range item.SubEntry.Files {
		srcFile := filepath.Join(backupPath, file)
		dstFile := filepath.Join(targetPath, file)

		if info, err := os.Lstat(dstFile); err == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				allLinked = false
			}
		} else {
			allLinked = false
		}

		if pathExists(srcFile) {
			anyBackup = true
		}
		if pathExists(dstFile) {
			anyTarget = true
		}
	}

	if allLinked && len(item.SubEntry.Files) > 0 {
		return StateLinked
	}
	if anyBackup {
		return StateReady
	}
	if anyTarget {
		return StateAdopt
	}
	return StateMissing
}

func (m Model) updateApplicationSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		// Go back to menu
		m.Screen = ScreenMenu
		return m, nil
	case "up", "k":
		if m.appCursor > 0 {
			m.appCursor--
			if m.appCursor < m.scrollOffset {
				m.scrollOffset = m.appCursor
			}
		}
	case "down", "j":
		visibleCount := m.getVisibleRowCount()
		if m.appCursor < visibleCount-1 {
			m.appCursor++
			if m.appCursor >= m.scrollOffset+m.viewHeight {
				m.scrollOffset = m.appCursor - m.viewHeight + 1
			}
		}
	case "left", "h":
		// Collapse node if expanded
		appIdx, subIdx := m.getApplicationAtCursor()
		if appIdx >= 0 {
			// If on sub-entry or on expanded app, collapse it
			if m.Applications[appIdx].Expanded {
				m.Applications[appIdx].Expanded = false
				// If we were on a sub-entry, move cursor to parent app
				if subIdx >= 0 {
					// Calculate the app row position
					visualRow := 0
					for i := 0; i < appIdx; i++ {
						visualRow++
						if m.Applications[i].Expanded {
							visualRow += len(m.Applications[i].SubItems)
						}
					}
					m.appCursor = visualRow
				}
			}
		}
	case "enter", "tab", "right", "l":
		// Toggle expansion
		appIdx, _ := m.getApplicationAtCursor()
		if appIdx >= 0 {
			m.Applications[appIdx].Expanded = !m.Applications[appIdx].Expanded
		}
	case " ", "x":
		// Toggle selection
		appIdx, subIdx := m.getApplicationAtCursor()
		if appIdx >= 0 {
			if subIdx < 0 {
				// Toggle entire application
				m.Applications[appIdx].Selected = !m.Applications[appIdx].Selected
				// Propagate to sub-items
				for i := range m.Applications[appIdx].SubItems {
					m.Applications[appIdx].SubItems[i].Selected = m.Applications[appIdx].Selected
				}
			} else {
				// Toggle individual sub-item
				m.Applications[appIdx].SubItems[subIdx].Selected = !m.Applications[appIdx].SubItems[subIdx].Selected
			}
		}
	case "a":
		// Select all
		for i := range m.Applications {
			m.Applications[i].Selected = true
			for j := range m.Applications[i].SubItems {
				m.Applications[i].SubItems[j].Selected = true
			}
		}
	case "n":
		// Select none
		for i := range m.Applications {
			m.Applications[i].Selected = false
			for j := range m.Applications[i].SubItems {
				m.Applications[i].SubItems[j].Selected = false
			}
		}
	}
	return m, nil
}

// getApplicationAtCursor returns the app index and sub-entry index at current cursor
// Returns (-1, -1) if cursor is out of bounds
// Returns (appIdx, -1) if cursor is on application row
// Returns (appIdx, subIdx) if cursor is on sub-entry row
func (m *Model) getApplicationAtCursor() (int, int) {
	visualRow := 0

	for appIdx, app := range m.Applications {
		if visualRow == m.appCursor {
			return appIdx, -1
		}
		visualRow++

		if app.Expanded {
			for subIdx := range app.SubItems {
				if visualRow == m.appCursor {
					return appIdx, subIdx
				}
				visualRow++
			}
		}
	}

	return -1, -1
}

// getVisibleRowCount returns total number of visible rows in the 2-level table
func (m *Model) getVisibleRowCount() int {
	count := 0
	for _, app := range m.Applications {
		count++ // Application row
		if app.Expanded {
			count += len(app.SubItems) // Sub-entry rows (no separate separator)
		}
	}
	return count
}

func (m Model) viewApplicationSelect() string {
	var b strings.Builder

	// Title
	b.WriteString(TitleStyle.Render("󰋗  Manage"))
	b.WriteString("\n\n")

	// Calculate column widths for alignment
	maxAppNameWidth := 0
	maxStateBadgeWidth := 0
	maxSubNameWidth := 0
	maxTypeWidth := 0
	maxSourceWidth := 0
	maxTargetWidth := 0

	for _, app := range m.Applications {
		if len(app.Application.Name) > maxAppNameWidth {
			maxAppNameWidth = len(app.Application.Name)
		}

		// Calculate state badge text width
		aggregateState := m.getAggregateState(app)
		stateText := aggregateState.String()
		if len(stateText) > maxStateBadgeWidth {
			maxStateBadgeWidth = len(stateText)
		}

		if app.Expanded {
			for _, subItem := range app.SubItems {
				if len(subItem.SubEntry.Name) > maxSubNameWidth {
					maxSubNameWidth = len(subItem.SubEntry.Name)
				}

				// Calculate type info length
				typeInfo := ""
				if subItem.SubEntry.IsGit() {
					typeInfo = "git"
				} else if subItem.SubEntry.IsFolder() {
					typeInfo = "folder"
				} else {
					fileCount := len(subItem.SubEntry.Files)
					if fileCount == 1 {
						typeInfo = "1 file"
					} else {
						typeInfo = fmt.Sprintf("%d files", fileCount)
					}
				}
				if len(typeInfo) > maxTypeWidth {
					maxTypeWidth = len(typeInfo)
				}

				// Source path (truncated)
				sourcePath := ""
				if subItem.SubEntry.IsGit() {
					sourcePath = truncatePath(subItem.SubEntry.Repo, 30)
				} else {
					sourcePath = truncatePath(m.resolvePath(subItem.SubEntry.Backup), 30)
				}
				if len(sourcePath) > maxSourceWidth {
					maxSourceWidth = len(sourcePath)
				}

				// Target path (truncated)
				targetPath := truncatePath(subItem.Target, 30)
				if len(targetPath) > maxTargetWidth {
					maxTargetWidth = len(targetPath)
				}
			}
		}
	}

	// Render 2-level table with proper alignment
	startIdx := m.scrollOffset
	endIdx := m.scrollOffset + m.viewHeight
	visualRow := 0

	for _, app := range m.Applications {
		// Render application row
		if visualRow >= startIdx && visualRow < endIdx {
			isSelected := visualRow == m.appCursor

			nameStyle := ListItemStyle
			if isSelected {
				nameStyle = SelectedListItemStyle
			}

			// Aggregate state for app (show worst state among sub-entries)
			aggregateState := m.getAggregateState(app)
			stateText := aggregateState.String()
			stateBadge := renderStateBadge(aggregateState)

			// Add spacing after badge for alignment
			// Badge visual widths: margin(1) + padding(1) + text + padding(1)
			// Ready=8, Adopt=8, Missing=10, Linked=9
			badgeTextLen := len(stateText)
			extraSpaces := maxStateBadgeWidth - badgeTextLen
			badgePadding := strings.Repeat(" ", extraSpaces)

			// Entry count
			entryCount := fmt.Sprintf("%d entries", len(app.SubItems))

			// Pad app name for alignment
			paddedAppName := app.Application.Name
			if len(paddedAppName) < maxAppNameWidth {
				paddedAppName = paddedAppName + strings.Repeat(" ", maxAppNameWidth-len(paddedAppName))
			}

			// For level-1 rows: only add cursor when selected
			if isSelected {
				line := fmt.Sprintf("> %s%s%s  %s",
					nameStyle.Render(paddedAppName),
					stateBadge,
					badgePadding,
					MutedTextStyle.Render(entryCount))
				b.WriteString(line)
			} else {
				line := fmt.Sprintf("%s%s%s  %s",
					nameStyle.Render(paddedAppName),
					stateBadge,
					badgePadding,
					MutedTextStyle.Render(entryCount))
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
		visualRow++

		// Render sub-entry rows if expanded
		if app.Expanded {
			for subIdx, subItem := range app.SubItems {
				if visualRow >= startIdx && visualRow < endIdx {
					isSelected := visualRow == m.appCursor

					// Tree connector: ├─ for non-last items, └─ for last item
					treePrefix := "├─"
					if subIdx == len(app.SubItems)-1 {
						treePrefix = "└─"
					}

					// File count or type info
					typeInfo := ""
					if subItem.SubEntry.IsGit() {
						typeInfo = "git"
					} else if subItem.SubEntry.IsFolder() {
						typeInfo = "folder"
					} else {
						fileCount := len(subItem.SubEntry.Files)
						if fileCount == 1 {
							typeInfo = "1 file"
						} else {
							typeInfo = fmt.Sprintf("%d files", fileCount)
						}
					}

					// Backup/source path
					sourcePath := ""
					if subItem.SubEntry.IsGit() {
						sourcePath = truncatePath(subItem.SubEntry.Repo, 30)
					} else {
						sourcePath = truncatePath(m.resolvePath(subItem.SubEntry.Backup), 30)
					}

					// Target path
					targetPath := truncatePath(subItem.Target, 30)

					// Pad columns for alignment
					paddedName := subItem.SubEntry.Name
					if len(paddedName) < maxSubNameWidth {
						paddedName = paddedName + strings.Repeat(" ", maxSubNameWidth-len(paddedName))
					}

					paddedType := typeInfo
					if len(paddedType) < maxTypeWidth {
						paddedType = paddedType + strings.Repeat(" ", maxTypeWidth-len(paddedType))
					}

					paddedSource := sourcePath
					if len(paddedSource) < maxSourceWidth {
						paddedSource = paddedSource + strings.Repeat(" ", maxSourceWidth-len(paddedSource))
					}

					paddedTarget := targetPath
					if len(paddedTarget) < maxTargetWidth {
						paddedTarget = paddedTarget + strings.Repeat(" ", maxTargetWidth-len(paddedTarget))
					}

					// For level-2 rows: add cursor only when selected
					var line string
					if isSelected {
						line = fmt.Sprintf("> %s %s  %s  %s  %s",
							treePrefix,
							SelectedListItemStyle.Render(paddedName),
							MutedTextStyle.Render(paddedType),
							PathBackupStyle.Render(paddedSource),
							PathTargetStyle.Render(paddedTarget))
					} else {
						line = fmt.Sprintf("  %s %s  %s  %s  %s",
							treePrefix,
							paddedName,
							MutedTextStyle.Render(paddedType),
							PathBackupStyle.Render(paddedSource),
							PathTargetStyle.Render(paddedTarget))
					}

					b.WriteString(line)
					b.WriteString("\n")
				}
				visualRow++
			}
		}
	}

	// Help
	b.WriteString("\n")
	b.WriteString(RenderHelp(
		"enter/→/l", "expand",
		"←/h", "collapse",
		"q", "back",
	))

	return BaseStyle.Render(b.String())
}

// getAggregateState returns the "worst" state among all sub-entries
// Priority: Missing > Adopt > Ready > Linked
func (m Model) getAggregateState(app ApplicationItem) PathState {
	if len(app.SubItems) == 0 {
		return StateMissing
	}

	hasLinked := false
	hasReady := false
	hasAdopt := false
	hasMissing := false

	for _, sub := range app.SubItems {
		switch sub.State {
		case StateLinked:
			hasLinked = true
		case StateReady:
			hasReady = true
		case StateAdopt:
			hasAdopt = true
		case StateMissing:
			hasMissing = true
		}
	}

	// Return worst state
	if hasMissing {
		return StateMissing
	}
	if hasAdopt {
		return StateAdopt
	}
	if hasReady {
		return StateReady
	}
	if hasLinked {
		return StateLinked
	}

	return StateMissing
}

