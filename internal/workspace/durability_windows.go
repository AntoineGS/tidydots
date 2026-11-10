//go:build windows

package workspace

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func syncDirectory(path string) error {
	if syncDirectoryObserver != nil {
		syncDirectoryObserver(path)
	}
	pointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(pointer, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return fmt.Errorf("open directory for durable flush %q: %w", path, err)
	}
	defer windows.CloseHandle(handle)
	if err := windows.FlushFileBuffers(handle); err != nil {
		return fmt.Errorf("durable directory flush is unsupported for %q: %w", path, err)
	}
	return nil
}
