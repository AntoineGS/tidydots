package tui

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// createLayoutTestModel creates a model with 40 apps for layout testing.
// It uses non-existent paths for deterministic status across environments.
func createLayoutTestModel() *Model {
	const appCount = 40
	apps := make([]config.Application, appCount)
	for i := 0; i < appCount; i++ {
		apps[i] = config.Application{
			Name:        fmt.Sprintf("app-%02d", i+1),
			Description: fmt.Sprintf("Application %d", i+1),
			Entries: []config.SubEntry{
				{
					Name:    "config",
					Backup:  fmt.Sprintf("./app-%02d", i+1),
					Targets: map[string]string{"linux": fmt.Sprintf("/tmp/tidydots-test-nonexistent/.config/app-%02d", i+1)},
				},
			},
		}
	}

	cfg := &config.Config{
		Version:      3,
		BackupRoot:   "/home/user/backup",
		Applications: apps,
	}
	plat := &platform.Platform{OS: platform.OSLinux}
	m := NewModel(cfg, plat, false)
	m.width = 100
	m.height = 30
	m.Screen = ScreenResults
	m.Operation = OpList
	m.tableCursor = 0
	return &m
}

// countOutputLines returns the number of lines in a rendered view output.
func countOutputLines(output string) int {
	return strings.Count(output, "\n") + 1
}

// TestLayoutHeightConsistency verifies that the total rendered output height
// does not exceed the terminal height, regardless of dynamic elements.
func TestLayoutHeightConsistency(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("snapshot golden files are platform-specific")
	}

	lipgloss.SetColorProfile(termenv.Ascii)

	tests := []struct {
		name  string
		setup func(m *Model)
	}{
		{
			name:  "no_dynamic_elements",
			setup: func(m *Model) {},
		},
		{
			name: "with_result_message",
			setup: func(m *Model) {
				m.results = []ResultItem{
					{Name: "app-01/config", Message: "Restored successfully", Success: true},
				}
			},
		},
		{
			name: "with_error_message",
			setup: func(m *Model) {
				m.results = []ResultItem{
					{Name: "app-01/config", Message: "Failed to restore", Success: false},
				}
			},
		},
		{
			name: "with_multi_select_banner",
			setup: func(m *Model) {
				m.selectedApps[0] = true
				m.multiSelectActive = true
			},
		},
		{
			name: "with_detail_panel",
			setup: func(m *Model) {
				m.showingDetail = true
			},
		},
		{
			name: "with_result_and_detail",
			setup: func(m *Model) {
				m.results = []ResultItem{
					{Name: "app-01/config", Message: "Restored successfully", Success: true},
				}
				m.showingDetail = true
			},
		},
		{
			name: "with_result_and_multi_select",
			setup: func(m *Model) {
				m.results = []ResultItem{
					{Name: "app-01/config", Message: "Restored successfully", Success: true},
				}
				m.selectedApps[0] = true
				m.multiSelectActive = true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createLayoutTestModel()
			tt.setup(m)

			output := m.View()
			plain := stripAnsiCodes(output)
			lineCount := countOutputLines(plain)

			// The rendered output should never exceed terminal height
			if lineCount > m.height {
				t.Errorf("rendered output (%d lines) exceeds terminal height (%d lines)\n"+
					"Output:\n%s", lineCount, m.height, plain)
			}
		})
	}
}

// TestScrollOffsetConsistencyBetweenUpdateAndView verifies that the scroll
// offset calculation in updateScrollOffset() (used in Update) is consistent
// with the viewport calculation in renderTable() (used in View).
// Both now use computeMaxVisibleRows() as a single source of truth.
func TestScrollOffsetConsistencyBetweenUpdateAndView(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}

	lipgloss.SetColorProfile(termenv.Ascii)

	tests := []struct {
		name   string
		height int
		cursor int
		setup  func(m *Model)
	}{
		{
			name:   "no_dynamic_elements",
			height: 30,
			cursor: 20,
			setup:  func(m *Model) {},
		},
		{
			name:   "with_result_message",
			height: 30,
			cursor: 20,
			setup: func(m *Model) {
				m.results = []ResultItem{
					{Name: "app-01/config", Message: "Restored", Success: true},
				}
			},
		},
		{
			name:   "with_detail_panel",
			height: 30,
			cursor: 20,
			setup: func(m *Model) {
				m.showingDetail = true
			},
		},
		{
			name:   "small_terminal",
			height: 20,
			cursor: 15,
			setup:  func(m *Model) {},
		},
		{
			name:   "small_terminal_with_result",
			height: 20,
			cursor: 15,
			setup: func(m *Model) {
				m.results = []ResultItem{
					{Name: "app-01/config", Message: "Restored", Success: true},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createLayoutTestModel()
			m.height = tt.height
			m.tableCursor = tt.cursor
			tt.setup(m)

			// Run updateScrollOffset (what Update() does)
			m.updateScrollOffset()
			maxVisible := m.computeMaxVisibleRows()

			// Now render the view to see what renderTable actually uses
			output := m.View()
			plain := stripAnsiCodes(output)

			// The cursor row should be visible in the rendered output
			cursorAppName := fmt.Sprintf("app-%02d", tt.cursor+1)
			if !strings.Contains(plain, cursorAppName) {
				t.Errorf("cursor at row %d (%s) is not visible in rendered output.\n"+
					"computeMaxVisibleRows: %d (height=%d)\n"+
					"Output:\n%s",
					tt.cursor, cursorAppName, maxVisible, m.height, plain)
			}
		})
	}
}

// TestResultMessageDoesNotShiftCursor verifies that when a result message
// appears (e.g. after a restore), the cursor still points to the correct
// row and that row remains visible.
func TestResultMessageDoesNotShiftCursor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}

	lipgloss.SetColorProfile(termenv.Ascii)

	m := createLayoutTestModel()
	m.tableCursor = 15

	// Render without result message
	m.updateScrollOffset()
	outputBefore := m.View()
	plainBefore := stripAnsiCodes(outputBefore)

	// Now add a result message (simulating a restore operation)
	m.results = []ResultItem{
		{Name: "app-16/config", Message: "Restored successfully", Success: true},
	}

	// Render with result message
	outputAfter := m.View()
	plainAfter := stripAnsiCodes(outputAfter)

	// The cursor's app should still be visible
	cursorAppName := "app-16"
	if !strings.Contains(plainBefore, cursorAppName) {
		t.Errorf("cursor app %s not visible before result message", cursorAppName)
	}
	if !strings.Contains(plainAfter, cursorAppName) {
		t.Errorf("cursor app %s not visible after result message appeared", cursorAppName)
	}

	// The result message should be visible
	if !strings.Contains(plainAfter, "Restored successfully") {
		t.Errorf("result message not visible in output")
	}

	// Total height should not exceed terminal
	lineCountAfter := countOutputLines(plainAfter)
	if lineCountAfter > m.height {
		t.Errorf("output with result message (%d lines) exceeds terminal height (%d)",
			lineCountAfter, m.height)
	}
}

// TestScrollIndicatorRowIndexMapping verifies that cursor highlighting
// is applied to the correct row when scroll indicators are present.
//
// The bug: StyleFunc uses `actualRow = row + scrollOffset` but when
// hasMoreAbove is true, row 0 is the "↑ N more above" indicator,
// so data rows start at index 1. The formula should account for this.
func TestScrollIndicatorRowIndexMapping(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}

	lipgloss.SetColorProfile(termenv.Ascii)

	m := createLayoutTestModel()

	// Position cursor in the middle so we have scroll indicators on both sides
	m.tableCursor = 20
	m.updateScrollOffset()

	output := m.View()
	plain := stripAnsiCodes(output)

	// The cursor should be on app-21 (0-indexed cursor 20 = app-21)
	cursorApp := "app-21"

	// Verify app-21 is visible
	if !strings.Contains(plain, cursorApp) {
		t.Errorf("cursor app %s should be visible in output", cursorApp)
	}

	// Verify scroll indicators are present (both above and below)
	if !strings.Contains(plain, "more above") {
		t.Errorf("expected '↑ N more above' indicator")
	}
	if !strings.Contains(plain, "more below") {
		t.Errorf("expected '↓ N more below' indicator")
	}
}

// TestLayoutWithVaryingTerminalHeights verifies the layout adapts correctly
// to different terminal sizes without overflowing.
func TestLayoutWithVaryingTerminalHeights(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}

	lipgloss.SetColorProfile(termenv.Ascii)

	heights := []int{15, 20, 25, 30, 40, 50}

	for _, height := range heights {
		t.Run(fmt.Sprintf("height_%d", height), func(t *testing.T) {
			m := createLayoutTestModel()
			m.height = height
			m.tableCursor = 5

			// Test with no dynamic elements
			output := m.View()
			plain := stripAnsiCodes(output)
			lineCount := countOutputLines(plain)

			if lineCount > height {
				t.Errorf("height=%d: output is %d lines (exceeds terminal)\n%s",
					height, lineCount, plain)
			}

			// Test with result message
			m.results = []ResultItem{
				{Name: "test", Message: "done", Success: true},
			}
			outputWithResult := m.View()
			plainWithResult := stripAnsiCodes(outputWithResult)
			lineCountWithResult := countOutputLines(plainWithResult)

			if lineCountWithResult > height {
				t.Errorf("height=%d with result: output is %d lines (exceeds terminal)\n%s",
					height, lineCountWithResult, plainWithResult)
			}
		})
	}
}

// TestCursorNavigationAfterResultMessage verifies that navigating the cursor
// after a result message appears still works correctly and maintains proper
// scroll position.
func TestCursorNavigationAfterResultMessage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}

	lipgloss.SetColorProfile(termenv.Ascii)

	m := createLayoutTestModel()
	m.tableCursor = 20

	// Add result message
	m.results = []ResultItem{
		{Name: "app-21/config", Message: "Restored", Success: true},
	}

	// Simulate moving cursor down
	for i := 0; i < 5; i++ {
		m.tableCursor++
		m.updateScrollOffset()

		output := m.View()
		plain := stripAnsiCodes(output)

		cursorApp := fmt.Sprintf("app-%02d", m.tableCursor+1)
		if !strings.Contains(plain, cursorApp) {
			t.Errorf("after moving down %d times to cursor %d: %s not visible in output",
				i+1, m.tableCursor, cursorApp)
		}

		lineCount := countOutputLines(plain)
		if lineCount > m.height {
			t.Errorf("after moving down %d times: output %d lines exceeds height %d",
				i+1, lineCount, m.height)
		}
	}
}

// TestScrollOffsetDivergence verifies that computeMaxVisibleRows() produces
// the same result as an inline replication of the layout calculation.
// This ensures the shared method stays correct as dynamic elements change.
func TestScrollOffsetDivergence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}

	lipgloss.SetColorProfile(termenv.Ascii)

	tests := []struct {
		name  string
		setup func(m *Model)
	}{
		{
			name:  "baseline_no_extras",
			setup: func(m *Model) {},
		},
		{
			name: "with_result_message",
			setup: func(m *Model) {
				m.results = []ResultItem{
					{Name: "test", Message: "done", Success: true},
				}
			},
		},
		{
			name: "with_detail_panel",
			setup: func(m *Model) {
				m.showingDetail = true
			},
		},
		{
			name: "with_multi_select",
			setup: func(m *Model) {
				m.selectedApps[0] = true
				m.multiSelectActive = true
			},
		},
		{
			name: "with_result_and_detail",
			setup: func(m *Model) {
				m.results = []ResultItem{
					{Name: "test", Message: "done", Success: true},
				}
				m.showingDetail = true
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createLayoutTestModel()
			m.height = 30
			m.tableCursor = 25
			tt.setup(m)

			// Both updateScrollOffset and viewListTable now use computeMaxVisibleRows().
			// Verify the method produces the expected result by replicating the calculation inline.
			maxVisibleFromMethod := m.computeMaxVisibleRows()

			// Replicate what computeMaxVisibleRows should compute
			linesAboveTable := 1 // filter banner
			linesAfterTable := 1 // blank line
			if m.showingDetail {
				appIdx, subIdx := m.getApplicationAtCursorFromTable()
				if appIdx >= 0 {
					var dc string
					if subIdx >= 0 {
						dc = m.renderSubEntryInlineDetail(&m.Applications[appIdx].SubItems[subIdx], m.width)
					} else {
						filtered := m.getSearchedApplications()
						if appIdx < len(filtered) {
							dc = m.renderApplicationInlineDetail(&filtered[appIdx], m.width)
						}
					}
					if dc != "" {
						linesAfterTable += strings.Count(dc, "\n") + 1
					}
				}
			}
			if len(m.results) > 0 {
				linesAfterTable += 2
			}
			helpText := m.renderHelpForCurrentState()
			helpLines := strings.Count(helpText, "\n") + 1
			linesAfterTable += 1 + helpLines
			actualAvailable := m.height - linesAboveTable - linesAfterTable - 2
			actualMaxVisibleRows := actualAvailable - 4
			if actualMaxVisibleRows < 3 {
				actualMaxVisibleRows = 3
			}

			divergence := maxVisibleFromMethod - actualMaxVisibleRows

			t.Logf("computeMaxVisibleRows: %d, inline calculation: %d, divergence: %d",
				maxVisibleFromMethod, actualMaxVisibleRows, divergence)

			if divergence != 0 {
				t.Errorf("computeMaxVisibleRows() = %d but inline calc = %d (divergence %d)",
					maxVisibleFromMethod, actualMaxVisibleRows, divergence)
			}
		})
	}
}

// TestExpandedAppLayoutShift verifies that expanding an app (which adds
// sub-entry rows to the table) doesn't cause the layout to overflow.
func TestExpandedAppLayoutShift(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}

	lipgloss.SetColorProfile(termenv.Ascii)

	// Create apps where one has many sub-entries
	apps := make([]config.Application, 20)
	for i := 0; i < 20; i++ {
		if i == 5 {
			// App with many sub-entries
			entries := make([]config.SubEntry, 8)
			for j := 0; j < 8; j++ {
				entries[j] = config.SubEntry{
					Name:    fmt.Sprintf("entry-%d", j+1),
					Backup:  fmt.Sprintf("./app-06/entry-%d", j+1),
					Targets: map[string]string{"linux": fmt.Sprintf("/tmp/nonexistent/entry-%d", j+1)},
				}
			}
			apps[i] = config.Application{
				Name:        "app-06",
				Description: "App with many entries",
				Entries:     entries,
			}
		} else {
			apps[i] = config.Application{
				Name:        fmt.Sprintf("app-%02d", i+1),
				Description: fmt.Sprintf("Application %d", i+1),
				Entries: []config.SubEntry{
					{
						Name:    "config",
						Backup:  fmt.Sprintf("./app-%02d", i+1),
						Targets: map[string]string{"linux": fmt.Sprintf("/tmp/nonexistent/app-%02d", i+1)},
					},
				},
			}
		}
	}

	cfg := &config.Config{
		Version:      3,
		BackupRoot:   "/home/user/backup",
		Applications: apps,
	}
	plat := &platform.Platform{OS: platform.OSLinux}
	m := NewModel(cfg, plat, false)
	m.width = 100
	m.height = 30
	m.Screen = ScreenResults
	m.Operation = OpList

	// Render collapsed
	m.tableCursor = 5 // on app-06
	outputCollapsed := m.View()
	plainCollapsed := stripAnsiCodes(outputCollapsed)
	linesCollapsed := countOutputLines(plainCollapsed)

	if linesCollapsed > m.height {
		t.Errorf("collapsed: %d lines exceeds height %d", linesCollapsed, m.height)
	}

	// Expand app-06
	m.Applications[5].Expanded = true
	m.initTableModel()

	outputExpanded := m.View()
	plainExpanded := stripAnsiCodes(outputExpanded)
	linesExpanded := countOutputLines(plainExpanded)

	if linesExpanded > m.height {
		t.Errorf("expanded: %d lines exceeds height %d\n%s", linesExpanded, m.height, plainExpanded)
	}

	// Now add a result message while expanded
	m.results = []ResultItem{
		{Name: "app-06/entry-1", Message: "Restored", Success: true},
	}

	outputExpandedWithResult := m.View()
	plainExpandedWithResult := stripAnsiCodes(outputExpandedWithResult)
	linesExpandedWithResult := countOutputLines(plainExpandedWithResult)

	if linesExpandedWithResult > m.height {
		t.Errorf("expanded with result: %d lines exceeds height %d\n%s",
			linesExpandedWithResult, m.height, plainExpandedWithResult)
	}
}
