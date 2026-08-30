package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	tmpl "github.com/AntoineGS/tidydots/internal/template"
)

func TestSubEntryFormLongWhenLoadAndUnrelatedSave(t *testing.T) {
	when := `{{/*` + strings.Repeat("x", 600) + `*/}}true`
	tests := []struct {
		name  string
		entry config.SubEntry
	}{
		{name: "config", entry: config.SubEntry{Name: "config", When: when, Backup: "./config", Targets: map[string]string{"linux": "~/.config"}}},
		{name: "setup", entry: config.SubEntry{Name: "setup", When: when, Check: map[string]string{"linux": "check"}, Run: map[string]string{"linux": "run"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Version: 3, Applications: []config.Application{{Name: "tool", Entries: []config.SubEntry{tt.entry}}}}
			m := NewModel(cfg, linuxPlatform(), false)
			m.ConfigPath = t.TempDir() + "/tidydots.yaml"
			m.initApplicationItems()
			m.initSubEntryForm(0, 0)
			if got := m.subEntryForm.WhenInput.Value(); got != when {
				t.Fatalf("loaded When length=%d, want %d", len(got), len(when))
			}
			m.subEntryForm.NameInput.SetValue("renamed")
			if err := m.saveSubEntryForm(); err != nil {
				t.Fatalf("saveSubEntryForm() error = %v", err)
			}
			if got := m.Config.Applications[0].Entries[0].When; got != when {
				t.Errorf("unrelated save changed When length=%d, want %d", len(got), len(when))
			}
		})
	}
}

func TestFilteredApplicationVisibleWithoutOperationalChildren(t *testing.T) {
	cfg := &config.Config{Version: 3, Applications: []config.Application{
		{Name: "filtered-config", When: `{{ eq .OS "windows" }}`, Entries: []config.SubEntry{{Name: "config", Backup: "./config", Targets: map[string]string{"linux": "/tmp/config"}}}},
		{Name: "filtered-package", When: `{{ eq .OS "windows" }}`, Package: &config.EntryPackage{Managers: map[string]config.ManagerValue{"apt": {PackageName: "pkg"}}}},
	}}
	m := NewModel(cfg, linuxPlatform(), false)
	m.filterEnabled = false
	m.rebuildTable()

	if len(m.Applications) != 2 || len(m.Applications[0].SubItems) != 0 || len(m.Applications[1].SubItems) != 0 {
		t.Fatalf("filtered applications exposed children: %#v", m.Applications)
	}
	if _, count := m.checkFilteredStatesCmd(); count != 0 {
		t.Fatalf("filtered applications dispatched %d state checks", count)
	}
	for i := range m.Applications {
		m.toggleAppSelection(i)
	}
	if m.multiSelectActive {
		t.Fatal("filtered application became selected")
	}

	for _, msg := range []tea.KeyPressMsg{{Code: 'r'}, {Code: 'R'}, {Code: 'i'}} {
		updated, _ := m.Update(msg)
		got := updated.(Model)
		if got.Screen != ScreenResults || got.Operation != OpList {
			t.Fatalf("key %q acted on filtered app: screen=%v operation=%v", msg.Key().String(), got.Screen, got.Operation)
		}
	}
}

func filteredPackageModel() Model {
	return NewModel(&config.Config{Version: 3, Applications: []config.Application{{
		Name: "filtered", When: `{{ eq .OS "windows" }}`,
		Package: &config.EntryPackage{Managers: map[string]config.ManagerValue{"apt": {PackageName: "pkg"}}},
		Entries: []config.SubEntry{{Name: "config", Backup: "./config", Targets: map[string]string{"linux": "/tmp/config"}}},
	}}}, linuxPlatform(), false)
}

func TestFilteredApplicationAppRowRemainsEditableAndDeletable(t *testing.T) {
	for _, key := range []rune{'e', 'd'} {
		t.Run(string(key), func(t *testing.T) {
			m := filteredPackageModel()
			m.filterEnabled = false
			m.rebuildTable()
			updated, _ := m.Update(tea.KeyPressMsg{Code: key})
			got := updated.(Model)
			if key == 'e' && (got.Screen != ScreenAddForm || got.activeForm != FormApplication) {
				t.Fatalf("edit route: screen=%v form=%v", got.Screen, got.activeForm)
			}
			if key == 'd' && !got.confirmingDeleteApp {
				t.Fatal("delete route did not open application confirmation")
			}
		})
	}
}

func TestFilteredApplicationRemainsNonOperational(t *testing.T) {
	m := filteredPackageModel()
	m.filterEnabled = false
	m.rebuildTable()
	// The package is unresolved, but its condition makes it non-operational.
	if m.hasLoadingItems() {
		t.Fatal("filtered unresolved package kept loading spinner active")
	}
	for _, msg := range []tea.KeyPressMsg{{Code: 'r'}, {Code: 'R'}, {Code: 'i'}, {Code: ' '}} {
		updated, _ := m.Update(msg)
		got := updated.(Model)
		if got.Screen != ScreenResults || got.multiSelectActive || got.summaryTransientSelection {
			t.Fatalf("key %q acted on filtered application: screen=%v selected=%v transient=%v", msg.Key().String(), got.Screen, got.multiSelectActive, got.summaryTransientSelection)
		}
	}
}

func TestFilteredPackageStateIsDormantUntilConditionMatches(t *testing.T) {
	m := filteredPackageModel()
	m.filterEnabled = false
	m.rebuildTable()
	if _, count := m.checkUncheckedPackageStatesCmd(); count != 0 {
		t.Fatalf("filtered package dispatched %d unchecked state checks", count)
	}
	if m.hasLoadingItems() {
		t.Fatal("filtered unresolved package kept loading spinner active")
	}

	m.Config.Applications[0].When = ""
	m.reinitPreservingState("filtered")
	if m.Applications[0].IsFiltered || len(m.Applications[0].SubItems) != 1 {
		t.Fatalf("matching condition did not restore operational app: %#v", m.Applications[0])
	}
	if _, count := m.checkUncheckedPackageStatesCmd(); count != 1 {
		t.Fatalf("matching package dispatched %d unchecked state checks, want 1", count)
	}
}

type whenSaveRenderer struct{}

func (whenSaveRenderer) RenderString(_, expression string) (string, error) {
	if expression == "{{ if }}" {
		return "", errors.New("parse error")
	}
	return "true", nil
}

func TestSaveApplicationValidatesWhenWithRuntimeRenderer(t *testing.T) {
	configPath := t.TempDir() + "/tidydots.yaml"
	m := NewModel(&config.Config{Version: 3}, &platform.Platform{OS: platform.OSLinux}, false)
	m.ConfigPath = configPath
	m.Renderer = tmpl.NewEngine(tmpl.NewContextFromPlatform(m.Platform))
	m.initApplicationForm(-1)
	m.applicationForm.NameInput.SetValue("sprout")
	m.applicationForm.WhenInput.SetValue(`{{ "hello" | toUpper }}`)
	if err := m.saveApplicationForm(); err != nil {
		t.Fatalf("valid Sprout expression rejected: %v", err)
	}
}

func TestSaveApplicationRejectsMalformedWhenBeforeWriting(t *testing.T) {
	configPath := t.TempDir() + "/tidydots.yaml"
	m := NewModel(&config.Config{Version: 3}, &platform.Platform{OS: platform.OSLinux}, false)
	m.ConfigPath = configPath
	m.Renderer = whenSaveRenderer{}
	m.initApplicationForm(-1)
	m.applicationForm.NameInput.SetValue("broken")
	m.applicationForm.WhenInput.SetValue("{{ if }}")
	if err := m.saveApplicationForm(); err == nil {
		t.Fatal("malformed expression was saved")
	}
}

var _ config.PathRenderer = whenSaveRenderer{}
