package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
)

// OpenReadOnly opens an existing render-history database without running
// migrations or changing SQLite settings. A missing database is not an error.
// SQLite's read-only URI mode prevents creation of the database and WAL
// sidecars; WAL databases that cannot be read with their existing sidecars
// return an error instead of falling back to a writable open.
func OpenReadOnly(ctx context.Context, dbPath string) (*Store, error) {
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		return nil, nil //nolint:nilnil // absent state is a normal inspection result
	} else if err != nil {
		return nil, fmt.Errorf("checking state database: %w", err)
	}

	if err := checkNoRollbackJournal(dbPath); err != nil {
		return nil, err
	}

	snapshotDir, err := os.MkdirTemp("", "tidydots-state-readonly-*")
	if err != nil {
		return nil, fmt.Errorf("creating temporary state snapshot: %w", err)
	}
	snapshotPath := filepath.Join(snapshotDir, "snapshot.db")
	if err := copyDatabaseSnapshot(dbPath, snapshotPath); err != nil {
		_ = os.RemoveAll(snapshotDir) //nolint:errcheck,gosec // best-effort cleanup
		return nil, err
	}

	u := url.URL{Scheme: "file", Path: snapshotPath}
	query := u.Query()
	query.Set("mode", "ro")
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		_ = os.RemoveAll(snapshotDir) //nolint:errcheck,gosec // best-effort cleanup
		return nil, fmt.Errorf("opening state database read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()                //nolint:errcheck,gosec // best-effort cleanup on error path
		_ = os.RemoveAll(snapshotDir) //nolint:errcheck,gosec // best-effort cleanup
		return nil, fmt.Errorf("reading state database: %w", err)
	}
	return &Store{db: db, path: dbPath, cleanupPath: snapshotDir}, nil
}

// copyDatabaseSnapshot copies the main database and its committed WAL without
// opening the source through SQLite (which can update -shm). File metadata is
// checked before and after the copy so a concurrently changing WAL is rejected.
func copyDatabaseSnapshot(source, destination string) error {
	return copyDatabaseSnapshotWithHooks(source, destination, snapshotHooks{})
}

type snapshotHooks struct {
	afterJournalCheck    func(attempt int) error
	afterCopy            func(attempt int) error
	afterUnstableAttempt func(attempt int) error
}

func copyDatabaseSnapshotWithHooks(source, destination string, hooks snapshotHooks) error {
	for attempt := 0; attempt < 3; attempt++ {
		stable, err := copySnapshotAttempt(source, destination, attempt, hooks)
		if err != nil {
			return err
		}
		if stable {
			return nil
		}
	}
	return fmt.Errorf("state database changed while taking read-only snapshot")
}

func copySnapshotAttempt(source, destination string, attempt int, hooks snapshotHooks) (bool, error) {
	if err := checkNoRollbackJournal(source); err != nil {
		return false, err
	}
	if hooks.afterJournalCheck != nil {
		if err := hooks.afterJournalCheck(attempt); err != nil {
			return false, fmt.Errorf("before snapshot attempt %d: %w", attempt+1, err)
		}
	}
	before, err := statSnapshotFiles(source)
	if err != nil {
		return false, err
	}
	if err := copySnapshotFiles(source, destination, before); err != nil {
		return false, err
	}
	if hooks.afterCopy != nil {
		if err := hooks.afterCopy(attempt); err != nil {
			return false, fmt.Errorf("after snapshot attempt %d copy: %w", attempt+1, err)
		}
	}
	if err := checkNoRollbackJournal(source); err != nil {
		return false, err
	}
	after, err := statSnapshotFiles(source)
	if err != nil {
		return false, err
	}
	if snapshotFilesMatch(before, after) {
		if err := checkNoRollbackJournal(source); err != nil {
			return false, err
		}
		return true, nil
	}
	if hooks.afterUnstableAttempt != nil {
		if err := hooks.afterUnstableAttempt(attempt); err != nil {
			return false, fmt.Errorf("after unstable snapshot attempt %d: %w", attempt+1, err)
		}
	}
	return false, nil
}

type snapshotFiles struct {
	database  os.FileInfo
	wal       os.FileInfo
	walExists bool
}

func statSnapshotFiles(path string) (snapshotFiles, error) {
	database, err := os.Stat(path)
	if err != nil {
		return snapshotFiles{}, fmt.Errorf("stating state database: %w", err)
	}
	wal, walExists, err := optionalFileInfo(path + "-wal")
	if err != nil {
		return snapshotFiles{}, fmt.Errorf("stating state WAL: %w", err)
	}
	return snapshotFiles{database: database, wal: wal, walExists: walExists}, nil
}

func copySnapshotFiles(source, destination string, snapshot snapshotFiles) error {
	if err := copyFile(source, destination, snapshot.database.Mode().Perm()); err != nil {
		return fmt.Errorf("copying state database snapshot: %w", err)
	}
	if snapshot.walExists {
		if err := copyFile(source+"-wal", destination+"-wal", 0600); err != nil {
			return fmt.Errorf("copying state WAL snapshot: %w", err)
		}
	} else if err := os.Remove(destination + "-wal"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing stale temporary WAL: %w", err)
	}
	return nil
}

func snapshotFilesMatch(before, after snapshotFiles) bool {
	return sameFileSnapshot(before.database, after.database) && before.walExists == after.walExists &&
		(!before.walExists || sameFileSnapshot(before.wal, after.wal))
}

func checkNoRollbackJournal(source string) error {
	if _, err := os.Lstat(source + "-journal"); err == nil {
		return fmt.Errorf("state database has a rollback journal; refusing inspection that may require recovery")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking state database journal: %w", err)
	}
	return nil
}

func optionalFileInfo(path string) (os.FileInfo, bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	return info, err == nil, err
}

func sameFileSnapshot(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && a.Mode() == b.Mode()
}

func copyFile(source, destination string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }() //nolint:errcheck // input close is best-effort
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
