package manager

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/fsys"
	"github.com/AntoineGS/tidydots/internal/platform"
)

type matchingSymlinkStatFailureFS struct {
	fsys.FS
	source string
}

func (f matchingSymlinkStatFailureFS) Stat(path string) (fs.FileInfo, error) {
	if filepath.Clean(path) == f.source {
		return nil, &fs.PathError{Op: "stat", Path: path, Err: fs.ErrPermission}
	}
	return f.FS.Stat(path)
}

func TestRestoreMatchingSymlink(t *testing.T) {
	skipIfNoSymlink(t)
	for _, folder := range []bool{false, true} {
		for _, dryRun := range []bool{false, true} {
			for _, tc := range []struct {
				name   string
				exists bool
				alias  bool
				denied bool
			}{
				{name: "missing"},
				{name: "existing", exists: true},
				{name: "dangling_alias", alias: true},
				{name: "existing_alias", exists: true, alias: true},
				{name: "permission_denied", exists: true, denied: true},
			} {
				t.Run(fmt.Sprintf("folder=%t/dry_run=%t/%s", folder, dryRun, tc.name), func(t *testing.T) {
					root := t.TempDir()
					source := filepath.Join(root, "source")
					target := filepath.Join(root, "target")
					realSource := source
					if tc.alias {
						realSource = filepath.Join(root, "rendered")
						if err := os.Symlink(realSource, source); err != nil {
							t.Fatal(err)
						}
					}
					if tc.exists {
						if folder {
							if err := os.Mkdir(realSource, 0o700); err != nil {
								t.Fatal(err)
							}
						} else {
							preservationWrite(t, realSource, "config")
						}
					}
					if !folder {
						if err := os.Mkdir(target, 0o700); err != nil {
							t.Fatal(err)
						}
						target = filepath.Join(target, "source")
					}
					if err := os.Symlink(source, target); err != nil {
						t.Fatal(err)
					}
					before, err := os.Lstat(target)
					if err != nil {
						t.Fatal(err)
					}
					mgr := New(&config.Config{BackupRoot: root}, &platform.Platform{OS: platform.OSLinux})
					mgr.DryRun = dryRun
					if tc.denied {
						// Inject only Stat failure: permissions are not reliable under root or on Windows.
						mgr = mgr.WithFS(matchingSymlinkStatFailureFS{FS: mgr.fs, source: source})
					}
					entry := config.SubEntry{Name: "app"}
					if folder {
						err = mgr.RestoreFolder(entry, source, target)
					} else {
						entry.Files = []string{"source"}
						err = mgr.RestoreFiles(entry, root, filepath.Dir(target))
					}
					if !tc.exists || tc.denied {
						if err == nil {
							t.Error("expected source validation error, got nil")
						} else if !strings.Contains(err.Error(), source) || !strings.Contains(err.Error(), target) {
							t.Errorf("error lacks source/target context: %v", err)
						}
						if tc.denied && !errors.Is(err, fs.ErrPermission) {
							t.Errorf("expected wrapped permission error, got %v", err)
						}
					} else if err != nil {
						t.Errorf("valid matching symlink should be a no-op: %v", err)
					}
					after, statErr := os.Lstat(target)
					if statErr != nil || !os.SameFile(before, after) {
						t.Fatalf("existing symlink replaced or removed: %v", statErr)
					}
					link, err := os.Readlink(target)
					if err != nil || link != source {
						t.Errorf("symlink = %q, %v; want %q", link, err, source)
					}
				})
			}
		}
	}
}
