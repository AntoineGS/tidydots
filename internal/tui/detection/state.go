// Package detection provides pure detection functions for the TUI package.
// These functions have no dependency on the Model type and can be safely
// called from goroutines.
package detection

import (
	"os"
	"path/filepath"

	"github.com/AntoineGS/tidydots/internal/manager"
	tuitable "github.com/AntoineGS/tidydots/internal/tui/table"
)

// DetectConfigState determines the state of a config entry given its paths and file list.
// This is a pure function that takes paths and returns a PathState. It only uses
// os.Lstat, manager.PathExists, and filepath.Join. It does NOT reference Model.
func DetectConfigState(backupPath, targetPath string, isFolder bool, files []string) tuitable.PathState {
	if isFolder {
		if info, err := os.Lstat(targetPath); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return tuitable.StateLinked
			}
		}

		backupExists := manager.PathExists(backupPath)
		targetExists := manager.PathExists(targetPath)

		if backupExists {
			return tuitable.StateReady
		}

		if targetExists {
			return tuitable.StateAdopt
		}

		return tuitable.StateMissing
	}

	// File-based config
	allLinked := true
	anyBackup := false
	anyTarget := false
	checkedAnyFile := false

	for _, file := range files {
		srcFile := filepath.Join(backupPath, file)
		dstFile := filepath.Join(targetPath, file)

		if !manager.PathExists(srcFile) {
			continue
		}

		checkedAnyFile = true
		anyBackup = true

		if info, err := os.Lstat(dstFile); err == nil {
			anyTarget = true
			if info.Mode()&os.ModeSymlink == 0 {
				allLinked = false
			}
		} else {
			allLinked = false
		}
	}

	if allLinked && checkedAnyFile {
		return tuitable.StateLinked
	}

	if anyBackup {
		return tuitable.StateReady
	}

	if anyTarget {
		return tuitable.StateAdopt
	}

	return tuitable.StateMissing
}
