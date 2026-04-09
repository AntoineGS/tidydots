package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/AntoineGS/tidydots/internal/tui/tuishared"
)

// All styles are re-exported from tuishared so that existing code within tui/
// continues to compile unchanged.  New sub-packages (forms/, table/, etc.)
// should import tuishared directly.

// Catppuccin Mocha palette — re-exported from tuishared for files that use raw colors.
var (
	primaryColor  = tuishared.PrimaryColor
	accentColor   = tuishared.AccentColor
	errorColor    = tuishared.ErrorColor
	mutedColor    = tuishared.MutedColor
	surface2Color = tuishared.Surface2Color
	blueColor     = tuishared.BlueColor
)

// Ensure lipgloss import is used.
var _ lipgloss.Style

// Style variables re-exported from tuishared.
var (
	BaseStyle                = tuishared.BaseStyle
	TitleStyle               = tuishared.TitleStyle
	SubtitleStyle            = tuishared.SubtitleStyle
	MutedTextStyle           = tuishared.MutedTextStyle
	MenuItemStyle            = tuishared.MenuItemStyle
	SelectedMenuItemStyle    = tuishared.SelectedMenuItemStyle
	ListItemStyle            = tuishared.ListItemStyle
	SelectedListItemStyle    = tuishared.SelectedListItemStyle
	CheckedStyle             = tuishared.CheckedStyle
	UncheckedStyle           = tuishared.UncheckedStyle
	PathNameStyle            = tuishared.PathNameStyle
	PathTargetStyle          = tuishared.PathTargetStyle
	PathBackupStyle          = tuishared.PathBackupStyle
	FolderBadgeStyle         = tuishared.FolderBadgeStyle
	StateBadgeReadyStyle     = tuishared.StateBadgeReadyStyle
	StateBadgeAdoptStyle     = tuishared.StateBadgeAdoptStyle
	StateBadgeMissingStyle   = tuishared.StateBadgeMissingStyle
	StateBadgeLinkedStyle    = tuishared.StateBadgeLinkedStyle
	StateBadgeOutdatedStyle  = tuishared.StateBadgeOutdatedStyle
	StateBadgeModifiedStyle  = tuishared.StateBadgeModifiedStyle
	StateBadgeFilteredStyle  = tuishared.StateBadgeFilteredStyle
	StateBadgeInstalledStyle = tuishared.StateBadgeInstalledStyle
	ProgressStyle            = tuishared.ProgressStyle
	SuccessStyle             = tuishared.SuccessStyle
	ErrorStyle               = tuishared.ErrorStyle
	WarningStyle             = tuishared.WarningStyle
	BoxStyle                 = tuishared.BoxStyle
	ResultBoxStyle           = tuishared.ResultBoxStyle
	HelpStyle                = tuishared.HelpStyle
	HelpKeyStyle             = tuishared.HelpKeyStyle
	StatusBarStyle           = tuishared.StatusBarStyle
	SpinnerStyle             = tuishared.SpinnerStyle
	FilterInputStyle         = tuishared.FilterInputStyle
	FilterHighlightStyle     = tuishared.FilterHighlightStyle
	MultiSelectBannerStyle   = tuishared.MultiSelectBannerStyle
	SelectedRowStyle         = tuishared.SelectedRowStyle
)

// Style function wrappers — delegate to tuishared.

// Rendering helper functions re-exported from tuishared.
var (
	RenderHelp             = tuishared.RenderHelp
	RenderHelpWithWidth    = tuishared.RenderHelpWithWidth
	RenderCursor           = tuishared.RenderCursor
	RenderCheckbox         = tuishared.RenderCheckbox
	RenderScrollIndicators = tuishared.RenderScrollIndicators
	CalculateVisibleRange  = tuishared.CalculateVisibleRange
	RenderOSInfo           = tuishared.RenderOSInfo
)
