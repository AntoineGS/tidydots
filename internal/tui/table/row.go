// Package table provides the TableRow type and PathState type used by the
// manage-screen table in the TUI.
package table

import "charm.land/bubbles/v2/table"

// PathState represents the state of a path item for restore operations.
// This is the canonical definition; the root tui package re-exports it as a
// type alias to avoid a circular import while keeping all TUI code compatible.
type PathState int

// Path states for restore operations.
const (
	// StateLoading indicates state is still being detected.
	StateLoading PathState = iota
	// StateReady indicates backup exists and is ready to restore.
	StateReady
	// StateAdopt indicates no backup but target exists (will adopt).
	StateAdopt
	// StateMissing indicates neither backup nor target exists.
	StateMissing
	// StateLinked indicates already symlinked.
	StateLinked
	// StateOutdated indicates linked but template source changed since last render.
	StateOutdated
	// StateModified indicates linked but rendered file has user edits.
	StateModified
)

// String returns the human-readable name of a PathState.
func (s PathState) String() string {
	switch s {
	case StateLoading:
		return "Loading..."
	case StateReady:
		return "Ready"
	case StateAdopt:
		return "Adopt"
	case StateMissing:
		return "Missing"
	case StateLinked:
		return "Linked"
	case StateOutdated:
		return "Outdated"
	case StateModified:
		return "Modified"
	}

	return "Unknown"
}

// Row wraps table.Row with hierarchy metadata used for rendering and
// selection logic in the manage screen.
type Row struct {
	Data            table.Row // Actual display data [name, status, info, path]
	Level           int       // 0 = application, 1 = sub-entry
	TreeChar        string    // "▶ ", "▼ ", "├─", "└─"
	IsExpanded      bool
	AppIndex        int       // Index in filtered array (for display)
	AppName         string    // Application name for lookup in m.Applications
	SubIndex        int       // -1 for application rows
	State           PathState // For badge rendering
	StatusAttention bool      // Status column needs attention
	InfoAttention   bool      // Info column needs attention
	InfoState       PathState // Highest-severity sub-entry state (app rows only)
	BackupPath      string    // Backup/source path for sub-entries (empty for app rows)
}
