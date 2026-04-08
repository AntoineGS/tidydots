package tui

import "github.com/AntoineGS/tidydots/internal/tui/tuishared"

// Key binding constants for TUI navigation and interaction — re-exported from tuishared.
const (
	KeyEnter     = tuishared.KeyEnter
	KeyEsc       = tuishared.KeyEsc
	KeyCtrlC     = tuishared.KeyCtrlC
	KeyCtrlS     = tuishared.KeyCtrlS
	KeyDown      = tuishared.KeyDown
	KeyTab       = tuishared.KeyTab
	KeyShiftTab  = tuishared.KeyShiftTab
	KeyBackspace = tuishared.KeyBackspace
	KeyDelete    = tuishared.KeyDelete
	KeyLeft      = tuishared.KeyLeft
	KeyRight     = tuishared.KeyRight
)

// UI element constants — re-exported from tuishared.
const (
	PlaceholderWhen             = tuishared.PlaceholderWhen
	PlaceholderNeovim           = tuishared.PlaceholderNeovim
	PlaceholderGitURL           = tuishared.PlaceholderGitURL
	PlaceholderGitBranch        = tuishared.PlaceholderGitBranch
	PlaceholderGitLinux         = tuishared.PlaceholderGitLinux
	PlaceholderGitWindows       = tuishared.PlaceholderGitWindows
	PlaceholderInstallerLinux   = tuishared.PlaceholderInstallerLinux
	PlaceholderInstallerWindows = tuishared.PlaceholderInstallerWindows
	PlaceholderInstallerBinary  = tuishared.PlaceholderInstallerBinary
	PlaceholderDep              = tuishared.PlaceholderDep
	IndentSpaces                = tuishared.IndentSpaces
	CheckboxUnchecked           = tuishared.CheckboxUnchecked
	CheckboxChecked             = tuishared.CheckboxChecked
)

// Entry type constants — re-exported from tuishared.
const (
	TypeGit       = tuishared.TypeGit
	TypeInstaller = tuishared.TypeInstaller
	TypeFolder    = tuishared.TypeFolder
	TypeNone      = tuishared.TypeNone
)

// Application status constants — re-exported from tuishared.
const (
	StatusInstalled = tuishared.StatusInstalled
	StatusMissing   = tuishared.StatusMissing
	StatusFiltered  = tuishared.StatusFiltered
	StatusOutdated  = tuishared.StatusOutdated
	StatusModified  = tuishared.StatusModified
	StatusUnknown   = tuishared.StatusUnknown
	StatusLoading   = tuishared.StatusLoading
)

// Sort column constants — re-exported from tuishared.
const (
	SortColumnName   = tuishared.SortColumnName
	SortColumnStatus = tuishared.SortColumnStatus
	SortColumnPath   = tuishared.SortColumnPath
)

// Scrolling behavior constants — re-exported from tuishared.
const (
	ScrollOffsetMargin = tuishared.ScrollOffsetMargin
)

// OS constants — re-exported from tuishared.
const (
	OSLinux   = tuishared.OSLinux
	OSWindows = tuishared.OSWindows
)

// Git package field indices — re-exported from tuishared.
const (
	GitFieldURL     = tuishared.GitFieldURL
	GitFieldBranch  = tuishared.GitFieldBranch
	GitFieldLinux   = tuishared.GitFieldLinux
	GitFieldWindows = tuishared.GitFieldWindows
	GitFieldSudo    = tuishared.GitFieldSudo
	GitFieldCount   = tuishared.GitFieldCount
)

// Installer package field indices — re-exported from tuishared.
const (
	InstallerFieldLinux   = tuishared.InstallerFieldLinux
	InstallerFieldWindows = tuishared.InstallerFieldWindows
	InstallerFieldBinary  = tuishared.InstallerFieldBinary
	InstallerFieldCount   = tuishared.InstallerFieldCount
)

// Text input size constants — re-exported from tuishared.
const (
	CharLimitName    = tuishared.CharLimitName
	CharLimitDesc    = tuishared.CharLimitDesc
	CharLimitPath    = tuishared.CharLimitPath
	CharLimitURL     = tuishared.CharLimitURL
	CharLimitWhen    = tuishared.CharLimitWhen
	CharLimitPkgName = tuishared.CharLimitPkgName
	CharLimitBranch  = tuishared.CharLimitBranch
	CharLimitBinary  = tuishared.CharLimitBinary
	CharLimitDep     = tuishared.CharLimitDep
	CharLimitFile    = tuishared.CharLimitFile
	InputWidthNarrow = tuishared.InputWidthNarrow
	InputWidthWide   = tuishared.InputWidthWide
)
