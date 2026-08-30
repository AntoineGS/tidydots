package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestForceRestoreCurrentRowRoutesThroughSummary(t *testing.T) {
	tests := []struct {
		name   string
		cursor int
	}{
		{name: "application", cursor: 0},
		{name: "sub-entry", cursor: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := twoAppModel()
			m.Applications[0].Expanded = true
			m.rebuildTable()
			m.pendingStateChecks = 0
			m.tableCursor = tt.cursor

			updated, _ := m.Update(tea.KeyPressMsg{Code: 'R'})
			got := updated.(Model)
			if got.Screen != ScreenSummary || got.summaryOperation != OpForceRestore {
				t.Fatalf("uppercase R routed to screen=%v operation=%v", got.Screen, got.summaryOperation)
			}
			if !got.summaryTransientSelection {
				t.Fatal("single-row Force Restore did not mark its selection transient")
			}
			if tt.name == "application" {
				if !reflect.DeepEqual(got.selectedApps, map[string]bool{"app1": true}) {
					t.Fatalf("app row selected apps = %#v, want app1 only", got.selectedApps)
				}
				if len(got.selectedSubEntries) != 0 {
					t.Fatalf("app row selected sub-entries = %#v, want none", got.selectedSubEntries)
				}
			} else {
				if len(got.selectedApps) != 0 {
					t.Fatalf("entry row selected apps = %#v, want none", got.selectedApps)
				}
				want := map[subEntryKey]bool{{app: "app1", sub: "config1"}: true}
				if !reflect.DeepEqual(got.selectedSubEntries, want) {
					t.Fatalf("entry row selected sub-entries = %#v, want config1 only", got.selectedSubEntries)
				}
			}
		})
	}
}

func TestForceRestoreSelectionRoutesThroughSummaryWithoutChangingSelection(t *testing.T) {
	m := twoAppModel()
	m.selectedApps["app2"] = true
	m.selectedSubEntries[subEntryKey{app: "app2", sub: "config3"}] = true
	m.multiSelectActive = true
	m.rebuildTable()
	m.pendingStateChecks = 0
	wantApps := cloneSelectedApps(m.selectedApps)
	wantSubEntries := cloneSelectedSubEntries(m.selectedSubEntries)

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'R'})
	got := updated.(Model)

	if got.Screen != ScreenSummary || got.summaryOperation != OpForceRestore {
		t.Fatalf("uppercase R routed to screen=%v operation=%v", got.Screen, got.summaryOperation)
	}
	if got.summaryTransientSelection {
		t.Fatal("pre-existing selection was marked transient")
	}
	if !reflect.DeepEqual(got.selectedApps, wantApps) || !reflect.DeepEqual(got.selectedSubEntries, wantSubEntries) {
		t.Fatalf("pre-existing selections changed: apps=%#v sub-entries=%#v", got.selectedApps, got.selectedSubEntries)
	}
}

func TestForceRestoreSummaryWarningAndTransientCancellation(t *testing.T) {
	m := twoAppModel()
	m.Applications[0].Expanded = true
	m.rebuildTable()
	m.pendingStateChecks = 0
	m.tableCursor = 1

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'R'})
	m = updated.(Model)
	view := m.View().Content
	for _, want := range []string{"Force Restore - Confirmation", "manual edits", ".tmpl.rendered", "discarded"} {
		if !strings.Contains(view, want) {
			t.Errorf("summary output missing %q", want)
		}
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := updated.(Model)
	if got.Screen != ScreenResults || got.multiSelectActive || got.summaryTransientSelection {
		t.Fatalf("transient cancellation left screen=%v active=%v transient=%v", got.Screen, got.multiSelectActive, got.summaryTransientSelection)
	}
	if len(got.selectedApps) != 0 || len(got.selectedSubEntries) != 0 {
		t.Fatal("transient selection maps were not cleared")
	}
}

func TestForceRestoreSummaryCancelPreservesOrdinarySelections(t *testing.T) {
	m := twoAppModel()
	m.selectedApps["app2"] = true
	m.selectedSubEntries[subEntryKey{app: "app2", sub: "config3"}] = true
	m.multiSelectActive = true
	m.summaryOperation = OpForceRestore
	m.Screen = ScreenSummary
	wantApps := cloneSelectedApps(m.selectedApps)
	wantSubEntries := cloneSelectedSubEntries(m.selectedSubEntries)

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := updated.(Model)
	if got.Screen != ScreenResults || !got.multiSelectActive || got.summaryTransientSelection {
		t.Fatalf("ordinary cancellation changed selection state: screen=%v active=%v transient=%v", got.Screen, got.multiSelectActive, got.summaryTransientSelection)
	}
	if !reflect.DeepEqual(got.selectedApps, wantApps) || !reflect.DeepEqual(got.selectedSubEntries, wantSubEntries) {
		t.Fatalf("ordinary selections changed: apps=%#v sub-entries=%#v", got.selectedApps, got.selectedSubEntries)
	}
}

func TestForceRestoreSummaryIgnoresSecondUppercaseR(t *testing.T) {
	m := twoAppModel()
	m.Applications[0].Expanded = true
	m.rebuildTable()
	m.pendingStateChecks = 0
	m.tableCursor = 1

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'R'})
	m = updated.(Model)
	before := m
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'R'})
	got := updated.(Model)

	if cmd != nil {
		t.Fatal("second uppercase R returned an execution command")
	}
	if got.Screen != before.Screen || got.summaryOperation != before.summaryOperation ||
		got.summaryTransientSelection != before.summaryTransientSelection ||
		got.summaryDoublePress != before.summaryDoublePress || got.processing != before.processing {
		t.Fatalf("second uppercase R changed confirmation state: before=%+v after=%+v", before, got)
	}
}

func cloneSelectedApps(selected map[string]bool) map[string]bool {
	clone := make(map[string]bool, len(selected))
	for name, value := range selected {
		clone[name] = value
	}
	return clone
}

func cloneSelectedSubEntries(selected map[subEntryKey]bool) map[subEntryKey]bool {
	clone := make(map[subEntryKey]bool, len(selected))
	for key, value := range selected {
		clone[key] = value
	}
	return clone
}
