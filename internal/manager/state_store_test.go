package manager

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

func TestInitStateStoreDryRunDoesNotCreateDatabase(t *testing.T) {
	root := t.TempDir()
	mgr := New(&config.Config{BackupRoot: root}, &platform.Platform{OS: platform.OSLinux})
	mgr.DryRun = true
	if err := mgr.InitStateStore(); err != nil {
		t.Fatal(err)
	}
	if mgr.stateStore != nil {
		t.Fatal("missing database should not initialize a state store")
	}
	if _, err := os.Stat(filepath.Join(root, ".tidydots.db")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created state database: %v", err)
	}
}

func TestInitReadOnlyStateStoreMissingDatabase(t *testing.T) {
	root := t.TempDir()
	mgr := New(&config.Config{BackupRoot: root}, &platform.Platform{OS: platform.OSLinux})
	if err := mgr.InitReadOnlyStateStore(); err != nil {
		t.Fatal(err)
	}
	if mgr.stateStore != nil {
		t.Fatal("missing database should not initialize a state store")
	}
	if err := mgr.Close(); err != nil {
		t.Fatal(err)
	}
}
