//go:build !unix && !windows

package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
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
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("workspace root lock exists (%s); remove it only after confirming no tidydots process is running: %s", path, root)
	}
	_, err = file.WriteString("tidydots workspace lock\n")
	closeErr := file.Close()
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return nil, closeErr
	}
	return func() { _ = os.Remove(path) }, nil
}
