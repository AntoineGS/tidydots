package manager

import (
	"context"
)

// Restorer defines the interface for restore operations
type Restorer interface {
	Restore() error
	RestoreWithContext(ctx context.Context) error
}

// Backuper defines the interface for backup operations
type Backuper interface {
	Backup() error
	BackupWithContext(ctx context.Context) error
}

// Adopter defines the interface for adopt operations
type Adopter interface {
	Adopt() error
	AdoptWithContext(ctx context.Context) error
}

// Lister defines the interface for listing operations
type Lister interface {
	List() error
}

// DotfileManager combines all manager operations
type DotfileManager interface {
	Restorer
	Backuper
	Lister
}
