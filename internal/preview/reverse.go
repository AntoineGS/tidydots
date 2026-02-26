package preview

import (
	"slices"
	"strings"
)

// lineTypeText is the line type for plain text lines that are editable.
const lineTypeText = "text"

// insertOp represents a line insertion in the rendered buffer.
type insertOp struct {
	AfterRenderedLine int    `json:"after_rendered_line"`
	Text              string `json:"text"`
}

// renderedEditPayload contains structural edits from the rendered buffer.
type renderedEditPayload struct {
	Inserts []insertOp `json:"inserts,omitempty"`
	Deletes []int      `json:"deletes,omitempty"`
}

// templateUpdateResponse is sent back to the editor after a structural edit.
type templateUpdateResponse struct {
	TemplateUpdate templateUpdatePayload `json:"template_update"`
}

// templateUpdatePayload contains the updated template content.
type templateUpdatePayload struct {
	Content    string `json:"content"`
	CursorLine int    `json:"cursor_line"`
}

// ApplyRenderedEdit modifies a template source based on structural edits
// (inserts/deletes) from the rendered buffer.
// Returns the updated template source and a cursor line hint.
func ApplyRenderedEdit(
	tmplSource string,
	reverseMap map[int]int,
	lineTypes map[int]string,
	edit renderedEditPayload,
) (string, int, error) {
	lines := strings.Split(tmplSource, "\n")

	lines, deleteCursor := applyDeletes(lines, edit.Deletes, reverseMap, lineTypes)
	lines, cursorLine := applyInserts(lines, edit.Inserts, reverseMap)

	if len(edit.Inserts) == 0 && deleteCursor > 0 {
		cursorLine = deleteCursor
	}

	cursorLine = clamp(cursorLine, 1, len(lines))

	return strings.Join(lines, "\n"), cursorLine, nil
}

// applyDeletes removes text lines from the template based on rendered line deletions.
// Only lines classified as "text" are deletable; directive and expression lines are skipped.
// Returns the modified lines and a cursor line hint (1-based, the line before the first deletion).
func applyDeletes(lines []string, deletes []int, reverseMap map[int]int, lineTypes map[int]string) ([]string, int) {
	deleteSet := collectDeletableIndices(deletes, reverseMap, lineTypes, len(lines))

	indices := make([]int, 0, len(deleteSet))
	for idx := range deleteSet {
		indices = append(indices, idx)
	}

	slices.SortFunc(indices, func(a, b int) int { return b - a })

	// Cursor goes to the line before the first deletion (ascending order).
	cursorLine := 0
	if len(indices) > 0 {
		firstIdx := indices[len(indices)-1] // smallest index (sorted descending)
		cursorLine = max(firstIdx+1, 1)     // 1-based, same position (lines above shift down)
	}

	for _, idx := range indices {
		lines = slices.Delete(lines, idx, idx+1)
	}

	return lines, cursorLine
}

// collectDeletableIndices maps rendered delete lines to 0-based template line indices,
// keeping only those classified as text.
func collectDeletableIndices(deletes []int, reverseMap map[int]int, lineTypes map[int]string, lineCount int) map[int]bool {
	set := make(map[int]bool)

	for _, renderedLine := range deletes {
		tmplLine, ok := reverseMap[renderedLine]
		if !ok || lineTypes[tmplLine] != lineTypeText {
			continue
		}

		idx := tmplLine - 1
		if idx >= 0 && idx < lineCount {
			set[idx] = true
		}
	}

	return set
}

// applyInserts adds new lines to the template based on rendered buffer insertions.
// Inserts are processed in descending position order to keep earlier indices stable.
// Returns the modified lines and the cursor line (1-based) of the last insertion.
func applyInserts(lines []string, inserts []insertOp, reverseMap map[int]int) ([]string, int) {
	cursorLine := 1

	sorted := slices.Clone(inserts)
	slices.SortFunc(sorted, func(a, b insertOp) int {
		return b.AfterRenderedLine - a.AfterRenderedLine
	})

	for _, ins := range sorted {
		insertIdx := resolveInsertIndex(ins, reverseMap, len(lines))
		lines = slices.Insert(lines, insertIdx, ins.Text)
		cursorLine = insertIdx + 1
	}

	return lines, cursorLine
}

// resolveInsertIndex determines the 0-based index at which to insert a new line.
// AfterRenderedLine == 0 means insert before line 1 (index 0).
func resolveInsertIndex(ins insertOp, reverseMap map[int]int, lineCount int) int {
	var tmplLineAfter int

	if ins.AfterRenderedLine == 0 {
		tmplLineAfter = 0
	} else if mapped, ok := reverseMap[ins.AfterRenderedLine]; ok {
		tmplLineAfter = mapped
	} else {
		tmplLineAfter = lineCount
	}

	return clamp(tmplLineAfter, 0, lineCount)
}

// clamp restricts v to the range [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}

	return v
}
