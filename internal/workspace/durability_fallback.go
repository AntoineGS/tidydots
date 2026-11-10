//go:build !unix && !windows

package workspace

import "fmt"

func syncDirectory(path string) error {
	if syncDirectoryObserver != nil {
		syncDirectoryObserver(path)
	}
	return fmt.Errorf("durable directory sync is unsupported on this platform: %s", path)
}
