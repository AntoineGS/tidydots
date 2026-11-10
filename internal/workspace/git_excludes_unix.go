//go:build !windows

package workspace

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func appendGitExcludeNoFollow(path string, previouslyRead []byte, lines []string) (retErr error) {
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_APPEND|syscall.O_CREAT|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path)
	defer func() {
		if closeErr := file.Close(); retErr == nil {
			retErr = closeErr
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("exclude path is not a regular file")
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
