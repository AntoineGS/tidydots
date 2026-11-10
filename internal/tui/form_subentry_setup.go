package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

const setupEmptyValue = "(empty)"

var (
	subEntryCheckModeToggleKey = key.NewBinding(
		key.WithKeys("space", "enter"),
		key.WithHelp("space/enter", "toggle"),
	)
	subEntryCheckModeExitCodeKey = key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "exit-code"),
	)
	subEntryCheckModeStatusKey = key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "status"),
	)
	repositoryPrivateKey = key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "private"),
	)
	repositoryPublicKey = key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "public"),
	)
)

func (m Model) renderSubEntryToggleValue(field subEntryFieldType, value string) string {
	if m.getSubEntryFieldType() == field {
		return SelectedMenuItemStyle.Render(value)
	}
	return value
}

func (m Model) renderSubEntryRepositoryField() string {
	if m.subEntryForm == nil {
		return ""
	}
	label := "Repository:"
	if m.getSubEntryFieldType() == subFieldRepository {
		label = HelpKeyStyle.Render(label)
	}
	publicMark, privateMark := CheckboxUnchecked, CheckboxUnchecked
	if m.subEntryForm.RepositoryRole == workspace.Private {
		privateMark = CheckboxChecked
	} else {
		publicMark = CheckboxChecked
	}
	privateLabel := "Private"
	if !m.subEntryForm.PrivateRepositoryEnabled {
		privateLabel = MutedTextStyle.Render("Private unavailable (no private repository enabled)")
	}
	value := fmt.Sprintf("%s Public  %s %s", publicMark, privateMark, privateLabel)
	if m.getSubEntryFieldType() == subFieldRepository {
		value = SelectedMenuItemStyle.Render(value)
	}
	return fmt.Sprintf("  %s  %s\n\n", label, value)
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

	value := config.CheckModeExitCode
	if m.subEntryForm.CheckMode == config.CheckModeStatus {
		value = config.CheckModeStatus
	}
	fmt.Fprintf(&b, "  Check mode:\n  %s\n\n",
		m.renderSubEntryToggleValue(subFieldCheckMode, value))

	rootLabel := "Root only:"
	if m.getSubEntryFieldType() == subFieldIsSudo {
		rootLabel = HelpKeyStyle.Render(rootLabel)
	}
	rootCheck := CheckboxUnchecked
	if m.subEntryForm.IsSudo {
		rootCheck = CheckboxChecked
	}
	fmt.Fprintf(&b, "  %s  %s Yes\n\n", rootLabel, rootCheck)

	return b.String()
}
