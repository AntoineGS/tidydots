package preview

import (
	"testing"

	tmpl "github.com/AntoineGS/tidydots/internal/template"
)

func TestApplyRenderedEdit_DeleteLine(t *testing.T) {
	tmplSource := "header\n{{ if eq .OS \"linux\" }}\nlinux\n{{ end }}\nfooter"
	forwardMap := map[int]int{1: 1, 2: 2, 3: 3, 4: 4, 5: 5}
	lineTypes := tmpl.ClassifyLineTypes(tmplSource)
	reverseMap := tmpl.BuildReverseMap(forwardMap, lineTypes)

	edit := renderedEditPayload{
		Deletes: []int{3},
	}

	updated, cursorLine, err := ApplyRenderedEdit(tmplSource, reverseMap, lineTypes, edit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "header\n{{ if eq .OS \"linux\" }}\n{{ end }}\nfooter"
	if updated != want {
		t.Errorf("updated =\n%q\nwant:\n%q", updated, want)
	}
	if cursorLine < 1 {
		t.Errorf("cursorLine = %d, want >= 1", cursorLine)
	}
}

func TestApplyRenderedEdit_InsertLine(t *testing.T) {
	tmplSource := "header\nfooter"
	forwardMap := map[int]int{1: 1, 2: 2}
	lineTypes := tmpl.ClassifyLineTypes(tmplSource)
	reverseMap := tmpl.BuildReverseMap(forwardMap, lineTypes)

	edit := renderedEditPayload{
		Inserts: []insertOp{{AfterRenderedLine: 1, Text: "middle"}},
	}

	updated, cursorLine, err := ApplyRenderedEdit(tmplSource, reverseMap, lineTypes, edit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "header\nmiddle\nfooter"
	if updated != want {
		t.Errorf("updated =\n%q\nwant:\n%q", updated, want)
	}
	if cursorLine != 2 {
		t.Errorf("cursorLine = %d, want 2", cursorLine)
	}
}

func TestApplyRenderedEdit_InsertAtStart(t *testing.T) {
	tmplSource := "existing"
	forwardMap := map[int]int{1: 1}
	lineTypes := tmpl.ClassifyLineTypes(tmplSource)
	reverseMap := tmpl.BuildReverseMap(forwardMap, lineTypes)

	edit := renderedEditPayload{
		Inserts: []insertOp{{AfterRenderedLine: 0, Text: "prepended"}},
	}

	updated, _, err := ApplyRenderedEdit(tmplSource, reverseMap, lineTypes, edit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "prepended\nexisting"
	if updated != want {
		t.Errorf("updated =\n%q\nwant:\n%q", updated, want)
	}
}

func TestApplyRenderedEdit_InsertAtEnd(t *testing.T) {
	tmplSource := "first\nsecond"
	forwardMap := map[int]int{1: 1, 2: 2}
	lineTypes := tmpl.ClassifyLineTypes(tmplSource)
	reverseMap := tmpl.BuildReverseMap(forwardMap, lineTypes)

	edit := renderedEditPayload{
		Inserts: []insertOp{{AfterRenderedLine: 2, Text: "appended"}},
	}

	updated, _, err := ApplyRenderedEdit(tmplSource, reverseMap, lineTypes, edit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "first\nsecond\nappended"
	if updated != want {
		t.Errorf("updated =\n%q\nwant:\n%q", updated, want)
	}
}

func TestApplyRenderedEdit_DeleteReadOnlyLineSkipped(t *testing.T) {
	tmplSource := "Hello {{ .User }}"
	forwardMap := map[int]int{1: 1}
	lineTypes := tmpl.ClassifyLineTypes(tmplSource)
	reverseMap := tmpl.BuildReverseMap(forwardMap, lineTypes)

	edit := renderedEditPayload{
		Deletes: []int{1},
	}

	updated, _, err := ApplyRenderedEdit(tmplSource, reverseMap, lineTypes, edit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if updated != tmplSource {
		t.Errorf("updated =\n%q\nwant:\n%q (should be unchanged)", updated, tmplSource)
	}
}

func TestApplyRenderedEdit_MultipleDeletes(t *testing.T) {
	tmplSource := "line1\nline2\nline3\nline4"
	forwardMap := map[int]int{1: 1, 2: 2, 3: 3, 4: 4}
	lineTypes := tmpl.ClassifyLineTypes(tmplSource)
	reverseMap := tmpl.BuildReverseMap(forwardMap, lineTypes)

	edit := renderedEditPayload{
		Deletes: []int{2, 4},
	}

	updated, _, err := ApplyRenderedEdit(tmplSource, reverseMap, lineTypes, edit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "line1\nline3"
	if updated != want {
		t.Errorf("updated =\n%q\nwant:\n%q", updated, want)
	}
}
