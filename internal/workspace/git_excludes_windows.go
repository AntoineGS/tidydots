//go:build windows

package workspace

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func appendGitExcludeNoFollow(path string, previouslyRead []byte, lines []string) (retErr error) {
	// Windows lacks a portable no-follow append primitive in the Go standard
	// library. Use exclusive creation for missing files; for existing files,
	// validate handle identity and contents before writing any bytes.
	var file *os.File
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		file, err = os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_EXCL, 0600)
	} else if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("exclude path is not a regular non-symlink file")
		}
		file, err = os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0)
		if err == nil {
			opened, statErr := file.Stat()
			if statErr != nil || !os.SameFile(info, opened) {
				_ = file.Close()
				return fmt.Errorf("exclude file changed during registration; retry")
			}
		}
	}
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); retErr == nil {
			retErr = closeErr
		}
	}()
	if err := verifyNoSymlinkPath(path); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	current, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, previouslyRead) {
		return fmt.Errorf("exclude file changed during registration; retry")
	}
	var addition strings.Builder
	if len(current) > 0 && !bytes.HasSuffix(current, []byte("\n")) {
		addition.WriteByte('\n')
	}
	for _, line := range lines {
		addition.WriteString(line)
		addition.WriteByte('\n')
	}
	if _, err := file.WriteString(addition.String()); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
