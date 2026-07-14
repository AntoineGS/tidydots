package tui

// detailTarget resolves the cursor row to the model items the inline detail
// panel describes: (app, sub) for a sub-entry row, (app, nil) for an
// application row, (nil, nil) when the panel is closed or the cursor resolves
// to nothing. It resolves against m.Applications — never against the filtered
// view, whose positions do not line up with the cursor's real indices.
func (m Model) detailTarget() (*ApplicationItem, *SubEntryItem) {
	if !m.showingDetail {
		return nil, nil
	}

	appIdx, subIdx := m.getApplicationAtCursorFromTable()
	if appIdx < 0 {
		return nil, nil
	}

	app := &m.Applications[appIdx]
	if subIdx >= 0 && subIdx < len(app.SubItems) {
		return app, &app.SubItems[subIdx]
	}

	return app, nil
}

// detailContent returns the rendered inline detail panel for the cursor row,
// or "" when there is nothing to show. Both the Update path
// (computeMaxVisibleRows) and the View path (viewProgress) use this single
// implementation so their height math cannot diverge.
func (m Model) detailContent() string {
	app, sub := m.detailTarget()

	switch {
	case sub != nil:
		return m.renderSubEntryInlineDetail(sub, m.width)
	case app != nil:
		return m.renderApplicationInlineDetail(app, m.width)
	default:
		return ""
	}
}
