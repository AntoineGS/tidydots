// Package operations provides pure data types for TUI batch operations and
// results. These types are free of any dependency on the root tui Model and
// can therefore be imported by sub-packages without creating circular imports.
package operations

// ResultItem represents the result of an operation, including whether it
// succeeded and any associated message.
type ResultItem struct {
	Name    string
	Message string
	Success bool
}

// Operation represents the type of operation being performed in the TUI.
type Operation int

// TUI operation types.
const (
	// OpRestore is the restore operation
	OpRestore Operation = iota
	// OpList is the list entries operation
	OpList
	// OpInstallPackages is the install packages operation
	OpInstallPackages
	// OpDelete is the delete entries operation
	OpDelete
)

// String returns the human-readable name of an Operation.
func (o Operation) String() string {
	switch o {
	case OpRestore:
		return "Restore"
	case OpList:
		return "List"
	case OpInstallPackages:
		return "Install Packages"
	case OpDelete:
		return "Delete"
	}

	return "Unknown"
}

// BatchOperationMsg is sent for each step of a batch operation.
// It contains progress information about the current operation.
type BatchOperationMsg struct {
	ItemName    string  // Name of the item being processed
	ItemIndex   int     // Current item index (0-based)
	TotalItems  int     // Total number of items
	Success     bool    // Whether this operation succeeded
	Message     string  // Result message
	CurrentStep string  // Description of current step (e.g., "Restoring nvim-config")
	Progress    float64 // Overall progress (0.0 to 1.0)
}

// BatchCompleteMsg is sent when the entire batch operation completes.
type BatchCompleteMsg struct {
	Results      []ResultItem // Results for all operations
	SuccessCount int          // Count of successful operations
	FailCount    int          // Count of failed operations
}

// OperationCompleteMsg is sent when an operation completes, containing any error
// and results from the operation.
type OperationCompleteMsg struct {
	Err     error
	Results []ResultItem
}
