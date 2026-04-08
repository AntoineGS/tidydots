package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// updateResultsPopup handles key events when the results popup is visible.
func (m Model) updateResultsPopup(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m, cmd, handled := m.handleCommonKeys(msg); handled {
		return m, cmd
	}

	switch {
	case key.Matches(msg, ResultsPopupKeys.Close):
		m.showingResults = false
		m.resultsScrollOffset = 0

	case key.Matches(msg, ResultsPopupKeys.Up):
		if m.resultsScrollOffset > 0 {
			m.resultsScrollOffset--
		}

	case key.Matches(msg, ResultsPopupKeys.Down):
		if m.resultsScrollOffset < m.resultsMaxScrollOffset() {
			m.resultsScrollOffset++
		}
	}

	return m, nil
}

// resultsPopupContentHeight returns the maximum number of result lines visible
// in the popup at once.
func (m Model) resultsPopupContentHeight() int {
	// Popup is ~60% of terminal height, minus borders (2) and help area (2: blank + help line)
	height := int(float64(m.height)*0.6) - 2 - 2
	if height < 3 {
		return 3
	}
	return height
}

// resultsMaxScrollOffset returns the maximum scroll offset for the results popup.
func (m Model) resultsMaxScrollOffset() int {
	maxOffset := len(m.results) - m.resultsPopupContentHeight()
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

// renderResultsPopup renders the results popup as a centered overlay.
func (m Model) renderResultsPopup() string {
	// Popup width: 65% of terminal width, min 40, max width-4
	popupWidth := int(float64(m.width) * 0.65)
	if popupWidth < 40 {
		popupWidth = 40
	}
	if popupWidth > m.width-4 {
		popupWidth = m.width - 4
	}

	contentHeight := m.resultsPopupContentHeight()
	offset := m.resultsScrollOffset
	end := offset + contentHeight
	if end > len(m.results) {
		end = len(m.results)
	}

	// Build result lines
	var lines []string
	for _, result := range m.results[offset:end] {
		var line string
		if result.Success {
			line = SuccessStyle.Render(fmt.Sprintf("✓ %s: %s", result.Name, result.Message))
		} else {
			line = ErrorStyle.Render(fmt.Sprintf("✗ %s: %s", result.Name, result.Message))
		}
		lines = append(lines, line)
	}

	var b strings.Builder
	b.WriteString(strings.Join(lines, "\n"))

	// Scroll info if results exceed content height
	if len(m.results) > contentHeight {
		scrollInfo := MutedTextStyle.Render(
			fmt.Sprintf("(%d-%d of %d)", offset+1, end, len(m.results)),
		)
		b.WriteString("\n")
		b.WriteString(scrollInfo)
	}

	// Help line at bottom
	b.WriteString("\n")
	b.WriteString(RenderHelpFromBindings(popupWidth,
		ResultsPopupKeys.Up,
		ResultsPopupKeys.Down,
		ResultsPopupKeys.Close,
	))

	// Box with rounded border, primaryColor border, padding 1,2
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	title := titleStyle.Render("Results")

	popup := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Padding(1, 2).
		Width(popupWidth).
		Render(title + "\n\n" + b.String())

	// Center on screen
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, popup)
}
