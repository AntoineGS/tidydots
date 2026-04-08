package tui

import "github.com/AntoineGS/tidydots/internal/tui/tuishared"

// All keybinding types and variables are re-exported from tuishared so that
// existing code within tui/ continues to compile unchanged.

// SharedKeyMap is an alias for tuishared.SharedKeyMap.
type SharedKeyMap = tuishared.SharedKeyMap

// ListKeyMap is an alias for tuishared.ListKeyMap.
type ListKeyMap = tuishared.ListKeyMap

// MultiSelectKeyMap is an alias for tuishared.MultiSelectKeyMap.
type MultiSelectKeyMap = tuishared.MultiSelectKeyMap

// SearchKeyMap is an alias for tuishared.SearchKeyMap.
type SearchKeyMap = tuishared.SearchKeyMap

// ConfirmKeyMap is an alias for tuishared.ConfirmKeyMap.
type ConfirmKeyMap = tuishared.ConfirmKeyMap

// DetailKeyMap is an alias for tuishared.DetailKeyMap.
type DetailKeyMap = tuishared.DetailKeyMap

// FormNavKeyMap is an alias for tuishared.FormNavKeyMap.
type FormNavKeyMap = tuishared.FormNavKeyMap

// TextEditKeyMap is an alias for tuishared.TextEditKeyMap.
type TextEditKeyMap = tuishared.TextEditKeyMap

// SuggestionKeyMap is an alias for tuishared.SuggestionKeyMap.
type SuggestionKeyMap = tuishared.SuggestionKeyMap

// SummaryKeyMap is an alias for tuishared.SummaryKeyMap.
type SummaryKeyMap = tuishared.SummaryKeyMap

// DiffPickerKeyMap is an alias for tuishared.DiffPickerKeyMap.
type DiffPickerKeyMap = tuishared.DiffPickerKeyMap

// ResultsPopupKeyMap is an alias for tuishared.ResultsPopupKeyMap.
type ResultsPopupKeyMap = tuishared.ResultsPopupKeyMap

// FilePickerKeyMap is an alias for tuishared.FilePickerKeyMap.
type FilePickerKeyMap = tuishared.FilePickerKeyMap

// ModeChooserKeyMap is an alias for tuishared.ModeChooserKeyMap.
type ModeChooserKeyMap = tuishared.ModeChooserKeyMap

// FilesListKeyMap is an alias for tuishared.FilesListKeyMap.
type FilesListKeyMap = tuishared.FilesListKeyMap

// Keybinding instances — re-exported from tuishared.
var (
	SharedKeys       = tuishared.SharedKeys
	ListKeys         = tuishared.ListKeys
	MultiSelectKeys  = tuishared.MultiSelectKeys
	SearchKeys       = tuishared.SearchKeys
	ConfirmKeys      = tuishared.ConfirmKeys
	DetailKeys       = tuishared.DetailKeys
	FormNavKeys      = tuishared.FormNavKeys
	TextEditKeys     = tuishared.TextEditKeys
	SuggestionKeys   = tuishared.SuggestionKeys
	SummaryKeys      = tuishared.SummaryKeys
	DiffPickerKeys   = tuishared.DiffPickerKeys
	ResultsPopupKeys = tuishared.ResultsPopupKeys
	FilePickerKeys   = tuishared.FilePickerKeys
	ModeChooserKeys  = tuishared.ModeChooserKeys
	FilesListKeys    = tuishared.FilesListKeys
)
