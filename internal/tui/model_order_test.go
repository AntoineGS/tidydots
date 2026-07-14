package tui

import (
	"slices"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
)

// orderProbeConfig returns two apps whose alphabetical order ("alpha" before
// "zebra") inverts under a descending name sort, which is what exposed the
// in-place sort: getSearchedApplications used to return m.Applications itself
// when no search filter was active, so initTableModel's SortStableFunc
// reordered the model, and every index-keyed structure silently retargeted.
func orderProbeConfig() *config.Config {
	return &config.Config{
		Version:    3,
		BackupRoot: "/repo",
		Applications: []config.Application{
			{
				Name: "alpha",
				Entries: []config.SubEntry{
					{Name: "conf-a", Backup: "./a", Targets: map[string]string{"linux": "~/.a"}},
				},
			},
			{
				Name: "zebra",
				Entries: []config.SubEntry{
					{Name: "conf-z", Backup: "./z", Targets: map[string]string{"linux": "~/.z"}},
				},
			},
		},
	}
}

func applicationNames(m *Model) []string {
	names := make([]string, len(m.Applications))
	for i, app := range m.Applications {
		names[i] = app.Application.Name
	}

	return names
}

func TestModelOrder_ImmutableAcrossSortAndFilter(t *testing.T) {
	m := NewModel(orderProbeConfig(), linuxPlatform(), false)
	want := applicationNames(&m) // [alpha zebra], set once by initApplicationItems

	// Descending name sort inverts the view; the model must not follow.
	m.sortColumn = SortColumnName
	m.sortAscending = false
	m.rebuildTable()

	if got := applicationNames(&m); !slices.Equal(got, want) {
		t.Fatalf("descending sort reordered m.Applications: got %v, want %v", got, want)
	}

	// A filter cycle and a different sort column must not reorder it either.
	m.searchText = "conf"
	m.rebuildTable()
	m.searchText = ""
	m.sortColumn = SortColumnStatus
	m.rebuildTable()

	if got := applicationNames(&m); !slices.Equal(got, want) {
		t.Fatalf("filter cycle + status sort reordered m.Applications: got %v, want %v", got, want)
	}
}

func TestSelection_SurvivesResort(t *testing.T) {
	m := NewModel(orderProbeConfig(), linuxPlatform(), false)

	// Select "alpha" (index 0), then flip to a descending sort. Before the
	// fix the sort moved "zebra" to index 0, so the selection — and the batch
	// built from it — silently pointed at an app the user never chose.
	m.toggleAppSelection(0)

	m.sortColumn = SortColumnName
	m.sortAscending = false
	m.rebuildTable()

	items := m.collectBatchRestoreItems()
	if len(items) != 1 {
		t.Fatalf("collectBatchRestoreItems returned %d items, want 1", len(items))
	}

	if items[0].name != "alpha/conf-a" {
		t.Errorf("batch item = %q, want %q (selection drifted to a different app)",
			items[0].name, "alpha/conf-a")
	}
}
