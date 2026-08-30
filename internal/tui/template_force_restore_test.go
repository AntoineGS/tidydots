package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
)

func TestExecuteBatchForceRestoreOverwritesRenderedEdits(t *testing.T) {
	backupRoot := t.TempDir()
	targetRoot := t.TempDir()
	backupDir := filepath.Join(backupRoot, "config")
	if err := os.MkdirAll(backupDir, 0750); err != nil {
		t.Fatal(err)
	}
	tmplPath := filepath.Join(backupDir, "file.tmpl")
	renderedPath := filepath.Join(backupDir, "file.tmpl.rendered")
	if err := os.WriteFile(tmplPath, []byte("fresh template output"), 0600); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Version:    3,
		BackupRoot: backupRoot,
		Applications: []config.Application{{
			Name: "app",
			Entries: []config.SubEntry{{
				Name:    "config",
				Backup:  "config",
				Targets: map[string]string{"linux": targetRoot},
			}},
		}},
	}
	mgr := manager.New(cfg, linuxPlatform())
	if err := mgr.InitStateStore(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mgr.Close(); err != nil {
			t.Errorf("close manager: %v", err)
		}
	})
	mgr.ForceRender = true
	m := NewModel(cfg, linuxPlatform(), false)
	m.Manager = mgr
	if success, message := m.performRestoreSubEntry(m.Applications[0].SubItems[0]); !success {
		t.Fatalf("initial restore failed: %s", message)
	}
	if err := os.WriteFile(renderedPath, []byte("user edits"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmplPath, []byte("fresh template output"), 0600); err != nil {
		t.Fatal(err)
	}

	m.selectedSubEntries = map[subEntryKey]bool{{app: "app", sub: "config"}: true}
	msg, ok := m.executeBatchRestore(true)().(batchRestoreConfigsDoneMsg)
	if !ok {
		t.Fatal("executeBatchRestore returned unexpected message")
	}
	if !msg.results[0].Success {
		t.Fatalf("forced restore failed: %+v", msg.results[0])
	}
	got, err := os.ReadFile(renderedPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "fresh template output" {
		t.Fatalf("forced rendered output = %q, want %q", got, "fresh template output")
	}
	if !m.Manager.ForceRender {
		t.Fatal("ForceRender prior true value was not restored after Force Restore")
	}
}

func TestExecuteBatchForceRestoreRestoresManagerSettingOnFailure(t *testing.T) {
	cfg := setupOnlyConfig(config.SubEntry{
		Name:    "missing",
		Backup:  "missing",
		Targets: map[string]string{"linux": t.TempDir()},
	})
	mgr := manager.New(cfg, linuxPlatform())
	t.Cleanup(func() {
		if err := mgr.Close(); err != nil {
			t.Errorf("close manager: %v", err)
		}
	})
	mgr.ForceRender = true
	m := NewModel(cfg, linuxPlatform(), false)
	m.Manager = mgr
	m.selectedSubEntries = map[subEntryKey]bool{{app: "vicinae", sub: "missing"}: true}

	msg, ok := m.executeBatchRestore(true)().(batchRestoreConfigsDoneMsg)
	if !ok {
		t.Fatal("executeBatchRestore returned unexpected message")
	}
	if len(msg.results) != 1 || msg.results[0].Success {
		t.Fatalf("missing source result = %+v, want one failure", msg.results)
	}
	if !m.Manager.ForceRender {
		t.Fatal("ForceRender prior true value was not restored after failed Force Restore")
	}
}

func TestExecuteConfirmedForceRestoreShowsForceOperation(t *testing.T) {
	m := NewModel(setupOnlyConfig(configSubEntry()), linuxPlatform(), false)
	m.Manager = manager.New(m.Config, linuxPlatform())
	m.summaryOperation = OpForceRestore
	m.selectedSubEntries = map[subEntryKey]bool{{app: "vicinae", sub: "config-file"}: true}

	updated, cmd := m.executeConfirmedOperation()
	got := updated.(Model)
	if got.Operation != OpForceRestore {
		t.Fatalf("visible operation = %v, want Force Restore", got.Operation)
	}
	if got.Screen != ScreenProgress {
		t.Fatalf("screen = %v, want progress", got.Screen)
	}
	if cmd == nil {
		t.Fatal("force restore did not dispatch a command")
	}
}

func TestBatchRestoreCompletionClearsTransientSummarySelection(t *testing.T) {
	m := NewModel(setupOnlyConfig(configSubEntry()), linuxPlatform(), false)
	m.summaryTransientSelection = true
	m.selectedSubEntries = map[subEntryKey]bool{{app: "vicinae", sub: "config-file"}: true}
	m.multiSelectActive = true

	updated, _ := m.Update(BatchCompleteMsg{})
	got := updated.(Model)
	if got.summaryTransientSelection {
		t.Fatal("config-only completion left transient summary selection marked")
	}
}

func TestForceRestoreConfigCompletionLabelsResults(t *testing.T) {
	m := NewModel(setupOnlyConfig(configSubEntry()), linuxPlatform(), false)
	m.Operation = OpForceRestore

	updated, _ := m.Update(BatchCompleteMsg{Results: []ResultItem{{Name: "config", Success: true}}})
	got := updated.(Model)
	if got.Operation != OpList {
		t.Fatalf("operation = %v, want OpList after completion", got.Operation)
	}
	if !strings.Contains(stripAnsiCodes(got.renderResultsPopup()), "Force Restore Results") {
		t.Fatal("config-only force restore results popup lacks Force Restore label")
	}
}

func TestForceRestoreSetupCompletionLabelsResults(t *testing.T) {
	m := NewModel(setupOnlyConfig(setupSubEntry()), linuxPlatform(), false)
	m.Operation = OpForceRestore
	m.setupBatch = true
	item := setupRunItem{sub: m.Applications[0].SubItems[0]}
	m.pendingSetups = []setupRunItem{item}

	updated, _ := m.handleSetupRunResult(setupRunMsg{
		item: item, success: true, message: "Setup complete",
	})
	got := updated.(Model)
	if got.Operation != OpList {
		t.Fatalf("operation = %v, want OpList after completion", got.Operation)
	}
	if !strings.Contains(stripAnsiCodes(got.renderResultsPopup()), "Force Restore Results") {
		t.Fatal("setup-ending force restore results popup lacks Force Restore label")
	}
}
