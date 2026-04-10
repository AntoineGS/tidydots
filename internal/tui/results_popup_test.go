package tui

import (
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestResultsPopupRendering(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}
	t.Setenv("NO_COLOR", "1")

	m := createLayoutTestModel()
	m.results = []ResultItem{
		{Name: "app-01/config", Message: "Restored successfully", Success: true},
		{Name: "app-02/config", Message: "Failed to restore", Success: false},
		{Name: "app-03/config", Message: "Restored successfully", Success: true},
	}
	m.showingResults = true

	output := m.renderResultsPopup()
	plain := stripAnsiCodes(output)

	// All results should be visible
	if !strings.Contains(plain, "app-01/config") {
		t.Error("expected app-01/config in popup")
	}
	if !strings.Contains(plain, "app-02/config") {
		t.Error("expected app-02/config in popup")
	}
	if !strings.Contains(plain, "app-03/config") {
		t.Error("expected app-03/config in popup")
	}

	// Help text should be present
	if !strings.Contains(plain, "close") {
		t.Error("expected help text with 'close' in popup")
	}
}

func TestResultsPopupScrolling(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}
	t.Setenv("NO_COLOR", "1")

	m := createLayoutTestModel()
	m.height = 20 // Small terminal to force scrolling

	// Create more results than can fit
	for i := 0; i < 30; i++ {
		m.results = append(m.results, ResultItem{
			Name:    "item-" + strings.Repeat("x", 2),
			Message: "done",
			Success: true,
		})
	}
	m.showingResults = true

	contentHeight := m.resultsPopupContentHeight()
	if contentHeight >= len(m.results) {
		t.Fatalf("test setup: contentHeight (%d) should be less than results count (%d)",
			contentHeight, len(m.results))
	}

	// Scroll down should work
	maxOffset := m.resultsMaxScrollOffset()
	if maxOffset <= 0 {
		t.Fatal("expected positive max scroll offset")
	}

	m.resultsScrollOffset = maxOffset
	output := m.renderResultsPopup()
	plain := stripAnsiCodes(output)

	// Last result should be visible when scrolled to bottom
	if !strings.Contains(plain, "item-xx") {
		t.Error("expected last results visible when scrolled to bottom")
	}
}

// TestResultsPopupBorderAlignment verifies that every rendered line of the
// popup has the same visual width, which is the condition for the border to
// render as a solid rectangle. Regression test for a bug where result messages
// containing ambiguous-width characters (e.g. "→") caused the right border to
// zigzag across rows.
func TestResultsPopupBorderAlignment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}
	t.Setenv("NO_COLOR", "1")

	// Exercise a range of terminal widths and both plain and ambiguous-width
	// content to make sure no combination produces uneven rows.
	widths := []int{60, 80, 100, 120}
	messages := []string{
		"Restored successfully",
		"Restored: /home/user/.config/app -> /home/user/dotfiles/app",
		"Restored: /home/user/.config/app → /home/user/dotfiles/app",
	}

	for _, w := range widths {
		for _, msg := range messages {
			m := createLayoutTestModel()
			m.width = w
			m.results = []ResultItem{{Name: "app/config", Message: msg, Success: true}}
			m.showingResults = true

			output := m.renderResultsPopup()
			plain := stripAnsiCodes(output)
			lines := strings.Split(plain, "\n")
			if len(lines) == 0 {
				t.Fatalf("width=%d: popup produced no output", w)
			}

			base := ansi.StringWidth(lines[0])
			for i, line := range lines {
				got := ansi.StringWidth(line)
				if got != base {
					t.Errorf("width=%d msg=%q line %d has visual width %d, want %d\n%s",
						w, msg, i, got, base, plain)
					break
				}
			}
		}
	}
}

func TestResultsPopupNotShownWhenNoResults(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}
	t.Setenv("NO_COLOR", "1")

	m := createLayoutTestModel()
	m.results = nil
	m.showingResults = true // Even if flag is set, no results = no popup

	output := m.View().Content
	plain := stripAnsiCodes(output)

	// Should render normal table, not popup
	if strings.Contains(plain, "Results") && strings.Contains(plain, "close") {
		t.Error("popup should not appear when no results exist")
	}
}
