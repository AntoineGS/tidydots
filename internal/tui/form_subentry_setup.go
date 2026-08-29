package tui

import (
	"fmt"
	"strings"
)

const setupEmptyValue = "(empty)"

func (m Model) renderSubEntryToggleValue(field subEntryFieldType, value string) string {
	if m.getSubEntryFieldType() == field {
		return SelectedMenuItemStyle.Render(value)
	}
	return value
}

func (m Model) renderSubEntrySetupFields() string {
	var b strings.Builder
	fields := []struct {
		field subEntryFieldType
		label string
		empty string
	}{
		{subFieldLinuxCheck, "Check (linux):", setupEmptyValue},
		{subFieldLinuxRun, "Run (linux):", setupEmptyValue},
		{subFieldWindowsCheck, "Check (windows):", setupEmptyValue},
		{subFieldWindowsRun, "Run (windows):", setupEmptyValue},
	}
	for _, field := range fields {
		label := field.label
		if m.getSubEntryFieldType() == field.field {
			label = HelpKeyStyle.Render(label)
		}
		fmt.Fprintf(&b, "  %s\n  %s\n\n", label, m.renderSubEntryFieldValue(field.field, field.empty))
	}
	return b.String()
}
