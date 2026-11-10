//go:build unix

package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func acquireRootLock(ctx context.Context, root string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := journalDirectory("")
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "locks")
	if _, err := verifyProtectedDirectory(dir, root); err != nil {
		return nil, err
	}
	if err := ensureDurablePrivateDir(dir); err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(root))
	path := filepath.Join(dir, hex.EncodeToString(sum[:])+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("workspace root is locked by another tidydots process: %s", root)
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}
