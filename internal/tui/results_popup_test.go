package tui

import (
	"runtime"
	"strings"
	"testing"
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
