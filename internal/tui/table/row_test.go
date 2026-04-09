package table

import (
	"testing"

	"charm.land/bubbles/v2/table"
)

func TestPathStateString(t *testing.T) {
	tests := []struct {
		name  string
		state PathState
		want  string
	}{
		{"StateLoading", StateLoading, "Loading..."},
		{"StateReady", StateReady, "Ready"},
		{"StateAdopt", StateAdopt, "Adopt"},
		{"StateMissing", StateMissing, "Missing"},
		{"StateLinked", StateLinked, "Linked"},
		{"StateOutdated", StateOutdated, "Outdated"},
		{"StateModified", StateModified, "Modified"},
		{"unknown value", PathState(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("PathState(%d).String() = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestPathStateIota(t *testing.T) {
	// Verify iota ordering is stable — any reordering would be a breaking change.
	if StateLoading != 0 {
		t.Errorf("StateLoading should be 0, got %d", StateLoading)
	}
	if StateReady != 1 {
		t.Errorf("StateReady should be 1, got %d", StateReady)
	}
	if StateAdopt != 2 {
		t.Errorf("StateAdopt should be 2, got %d", StateAdopt)
	}
	if StateMissing != 3 {
		t.Errorf("StateMissing should be 3, got %d", StateMissing)
	}
	if StateLinked != 4 {
		t.Errorf("StateLinked should be 4, got %d", StateLinked)
	}
	if StateOutdated != 5 {
		t.Errorf("StateOutdated should be 5, got %d", StateOutdated)
	}
	if StateModified != 6 {
		t.Errorf("StateModified should be 6, got %d", StateModified)
	}
}

func TestRowFields(t *testing.T) {
	r := Row{
		Data:            table.Row{"nvim-config", "Linked", "", "/home/user/.config/nvim"},
		Level:           1,
		TreeChar:        "└─",
		IsExpanded:      false,
		AppIndex:        2,
		AppName:         "nvim",
		SubIndex:        0,
		State:           StateLinked,
		StatusAttention: false,
		InfoAttention:   true,
		InfoState:       StateOutdated,
		BackupPath:      "/home/user/backup/nvim",
	}

	if r.Level != 1 {
		t.Errorf("Row.Level = %d, want 1", r.Level)
	}
	if r.TreeChar != "└─" {
		t.Errorf("Row.TreeChar = %q, want %q", r.TreeChar, "└─")
	}
	if r.IsExpanded {
		t.Error("Row.IsExpanded should be false")
	}
	if r.AppIndex != 2 {
		t.Errorf("Row.AppIndex = %d, want 2", r.AppIndex)
	}
	if r.AppName != "nvim" {
		t.Errorf("Row.AppName = %q, want %q", r.AppName, "nvim")
	}
	if r.SubIndex != 0 {
		t.Errorf("Row.SubIndex = %d, want 0", r.SubIndex)
	}
	if r.State != StateLinked {
		t.Errorf("Row.State = %v, want StateLinked", r.State)
	}
	if r.StatusAttention {
		t.Error("Row.StatusAttention should be false")
	}
	if !r.InfoAttention {
		t.Error("Row.InfoAttention should be true")
	}
	if r.InfoState != StateOutdated {
		t.Errorf("Row.InfoState = %v, want StateOutdated", r.InfoState)
	}
	if r.BackupPath != "/home/user/backup/nvim" {
		t.Errorf("Row.BackupPath = %q, want %q", r.BackupPath, "/home/user/backup/nvim")
	}
	if len(r.Data) != 4 {
		t.Errorf("Row.Data length = %d, want 4", len(r.Data))
	}
	if r.Data[0] != "nvim-config" {
		t.Errorf("Row.Data[0] = %q, want %q", r.Data[0], "nvim-config")
	}
}

func TestRowAppLevel(t *testing.T) {
	// Application-level row (Level=0, SubIndex=-1)
	r := Row{
		Data:      table.Row{"nvim", "Linked", "1/1", ""},
		Level:     0,
		TreeChar:  "▶ ",
		AppIndex:  0,
		AppName:   "nvim",
		SubIndex:  -1,
		State:     StateLinked,
		InfoState: StateLinked,
	}

	if r.Level != 0 {
		t.Errorf("app row Level = %d, want 0", r.Level)
	}
	if r.SubIndex != -1 {
		t.Errorf("app row SubIndex = %d, want -1", r.SubIndex)
	}
	if r.BackupPath != "" {
		t.Errorf("app row BackupPath should be empty, got %q", r.BackupPath)
	}
}
