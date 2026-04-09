package operations

import (
	"testing"
)

func TestOperationString(t *testing.T) {
	tests := []struct {
		name string
		op   Operation
		want string
	}{
		{"OpRestore", OpRestore, "Restore"},
		{"OpList", OpList, "List"},
		{"OpInstallPackages", OpInstallPackages, "Install Packages"},
		{"OpDelete", OpDelete, "Delete"},
		{"unknown value", Operation(99), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.op.String()
			if got != tt.want {
				t.Errorf("Operation(%d).String() = %q, want %q", tt.op, got, tt.want)
			}
		})
	}
}

func TestOperationIota(t *testing.T) {
	// Verify iota ordering is stable — any reordering would be a breaking change.
	if OpRestore != 0 {
		t.Errorf("OpRestore should be 0, got %d", OpRestore)
	}
	if OpList != 1 {
		t.Errorf("OpList should be 1, got %d", OpList)
	}
	if OpInstallPackages != 2 {
		t.Errorf("OpInstallPackages should be 2, got %d", OpInstallPackages)
	}
	if OpDelete != 3 {
		t.Errorf("OpDelete should be 3, got %d", OpDelete)
	}
}

func TestResultItemFields(t *testing.T) {
	item := ResultItem{
		Name:    "nvim-config",
		Message: "symlink created",
		Success: true,
	}

	if item.Name != "nvim-config" {
		t.Errorf("ResultItem.Name = %q, want %q", item.Name, "nvim-config")
	}
	if item.Message != "symlink created" {
		t.Errorf("ResultItem.Message = %q, want %q", item.Message, "symlink created")
	}
	if !item.Success {
		t.Error("ResultItem.Success should be true")
	}
}

func TestResultItemFailure(t *testing.T) {
	item := ResultItem{
		Name:    "broken-entry",
		Message: "permission denied",
		Success: false,
	}

	if item.Success {
		t.Error("ResultItem.Success should be false for a failure")
	}
	if item.Message != "permission denied" {
		t.Errorf("ResultItem.Message = %q, want %q", item.Message, "permission denied")
	}
}

func TestBatchOperationMsgFields(t *testing.T) {
	msg := BatchOperationMsg{
		ItemName:    "nvim-config",
		ItemIndex:   1,
		TotalItems:  5,
		Success:     true,
		Message:     "done",
		CurrentStep: "Restoring nvim-config",
		Progress:    0.4,
	}

	if msg.ItemIndex != 1 {
		t.Errorf("BatchOperationMsg.ItemIndex = %d, want 1", msg.ItemIndex)
	}
	if msg.TotalItems != 5 {
		t.Errorf("BatchOperationMsg.TotalItems = %d, want 5", msg.TotalItems)
	}
	if msg.Progress != 0.4 {
		t.Errorf("BatchOperationMsg.Progress = %f, want 0.4", msg.Progress)
	}
}

func TestBatchCompleteMsgFields(t *testing.T) {
	msg := BatchCompleteMsg{
		Results: []ResultItem{
			{Name: "a", Success: true},
			{Name: "b", Success: false},
		},
		SuccessCount: 1,
		FailCount:    1,
	}

	if len(msg.Results) != 2 {
		t.Errorf("BatchCompleteMsg.Results length = %d, want 2", len(msg.Results))
	}
	if msg.SuccessCount != 1 {
		t.Errorf("BatchCompleteMsg.SuccessCount = %d, want 1", msg.SuccessCount)
	}
	if msg.FailCount != 1 {
		t.Errorf("BatchCompleteMsg.FailCount = %d, want 1", msg.FailCount)
	}
}

func TestOperationCompleteMsgFields(t *testing.T) {
	msg := OperationCompleteMsg{
		Err: nil,
		Results: []ResultItem{
			{Name: "nvim", Message: "ok", Success: true},
		},
	}

	if msg.Err != nil {
		t.Errorf("OperationCompleteMsg.Err should be nil, got %v", msg.Err)
	}
	if len(msg.Results) != 1 {
		t.Errorf("OperationCompleteMsg.Results length = %d, want 1", len(msg.Results))
	}
}
