//go:build unix

package workspace

import (
	"fmt"
	"os"
)

func syncDirectory(path string) error {
	if syncDirectoryObserver != nil {
		syncDirectoryObserver(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := f.Sync()
	closeErr := f.Close()
	if syncErr != nil {
		return fmt.Errorf("sync directory %q: %w", path, syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close synced directory %q: %w", path, closeErr)
	}
	return nil
}
