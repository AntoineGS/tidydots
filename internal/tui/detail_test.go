package tui

import "testing"

// TestDetailTarget_UnderFilter_ResolvesCursorApp guards the detail panel's
// app resolution. The old inline code indexed filtered[appIdx] with a REAL
// model index: with "zebra" at real index 1 but alone (index 0) in the
// filtered slice, the bounds check appIdx < len(filtered) failed and the
// panel resolved nothing — and with more apps it resolved the WRONG one.
func TestDetailTarget_UnderFilter_ResolvesCursorApp(t *testing.T) {
	m := NewModel(orderProbeConfig(), linuxPlatform(), false)

	// Filter down to "zebra" (real index 1, filtered index 0) and put the
	// cursor on its application row.
	m.searchText = "zebra"
	m.rebuildTable()
	cursorToRow(t, &m, "zebra")
	m.showingDetail = true

	app, sub := m.detailTarget()
	if app == nil {
		t.Fatal("detailTarget returned no application for the cursor's app row")
	}

	if app.Application.Name != "zebra" {
		t.Errorf("detailTarget resolved %q, want %q", app.Application.Name, "zebra")
	}

	if sub != nil {
		t.Errorf("cursor is on an app row; sub should be nil, got %q", sub.SubEntry.Name)
	}
}

func TestDetailTarget_ClosedPanel_ReturnsNothing(t *testing.T) {
	m := NewModel(orderProbeConfig(), linuxPlatform(), false)
	m.showingDetail = false

	if app, sub := m.detailTarget(); app != nil || sub != nil {
		t.Errorf("detailTarget = (%v, %v), want (nil, nil) when the panel is closed", app, sub)
	}
}
