package manager

import (
	"bytes"
	"fmt"
	"log/slog"
	"runtime"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// copyFileTo copies src to dst. When useSudo is true (and not Windows) it shells
// out to `cp`; otherwise it copies via the filesystem abstraction (copyFile),
// which preserves the source file's permission bits. The sudo `cp` path is
// subject to the process umask and does not bit-for-bit preserve unusual modes.
func (m *Manager) copyFileTo(src, dst string, useSudo bool) error {
	if useSudo && runtime.GOOS != platform.OSWindows {
		if _, err := m.runner.RunWithSudo(m.ctx, "cp", src, dst); err != nil {
			return err
		}
		return nil
	}
	return m.copyFile(src, dst)
}

// removePath removes a single file or symlink at path. When useSudo is true (and
// not Windows) it shells out to `rm -f`; otherwise it uses the filesystem
// abstraction.
//
//nolint:unused // consumed by restoreFileCopy below, wired into restore.go in Task 3
func (m *Manager) removePath(path string, useSudo bool) error {
	if useSudo && runtime.GOOS != platform.OSWindows {
		if _, err := m.runner.RunWithSudo(m.ctx, "rm", "-f", path); err != nil {
			return err
		}
		return nil
	}
	return m.fs.Remove(path)
}

// filesEqual reports whether src and dst have identical contents. When useSudo is
// true it uses `cmp -s`, so a root-only target can still be compared: cmp exits 0
// when identical. Any error or non-zero exit is treated as "not equal", which at
// worst triggers a harmless idempotent re-copy. Without sudo it reads both files
// via the filesystem abstraction and byte-compares. A missing dst counts as not
// equal.
func (m *Manager) filesEqual(src, dst string, useSudo bool) (bool, error) {
	if !m.pathExists(dst) {
		return false, nil
	}

	if useSudo && runtime.GOOS != platform.OSWindows {
		res, err := m.runner.RunWithSudo(m.ctx, "cmp", "-s", src, dst)
		if err != nil {
			return false, nil
		}
		return res.ExitCode == 0, nil
	}

	srcData, err := m.fs.ReadFile(src)
	if err != nil {
		return false, err
	}
	dstData, err := m.fs.ReadFile(dst)
	if err != nil {
		return false, err
	}
	return bytes.Equal(srcData, dstData), nil
}

// restoreFileCopy deploys a single file by writing a real copy of srcFile at
// dstFile, used for entries with method: copy. It replaces any pre-existing
// symlink at dstFile (migration from a prior symlink-mode deployment) and is
// idempotent: when dstFile already exists as a real file whose contents match
// srcFile, it performs no write. All actions respect DryRun.
//
//nolint:unused // wired into restore.go's dispatch logic in Task 3
func (m *Manager) restoreFileCopy(subEntry config.SubEntry, srcFile, dstFile string) error {
	if !m.pathExists(srcFile) {
		if m.DryRun {
			m.logger.Info("source file does not exist (dry-run, skipping)", slog.String("path", srcFile))
			return nil
		}
		return NewPathError("restore", srcFile, fmt.Errorf("source file does not exist"))
	}

	switch {
	case m.isSymlink(dstFile):
		m.logger.Info("removing existing symlink", slog.String("path", dstFile))
		if !m.DryRun {
			if err := m.removePath(dstFile, subEntry.Sudo); err != nil {
				return NewPathError("restore", dstFile, fmt.Errorf("removing existing symlink: %w", err))
			}
		}
	case m.pathExists(dstFile):
		equal, err := m.filesEqual(srcFile, dstFile, subEntry.Sudo)
		if err != nil {
			return NewPathError("restore", dstFile, fmt.Errorf("comparing files: %w", err))
		}
		if equal {
			m.logger.Debug("copy already in sync", slog.String("path", dstFile))
			return nil
		}
	}

	m.logger.Info("copying file",
		slog.String("target", dstFile),
		slog.String("source", srcFile))

	if m.DryRun {
		return nil
	}

	if err := m.copyFileTo(srcFile, dstFile, subEntry.Sudo); err != nil {
		return NewPathError("restore", dstFile, fmt.Errorf("copying file: %w", err))
	}

	return nil
}
