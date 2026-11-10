package workspace

import "github.com/AntoineGS/tidydots/internal/config"

// SelectEntries returns eligible entries from a merged app. An empty name
// selects all entries; a non-empty name selects its unique owner-qualified
// entry. Both the source app condition and entry condition must match.
func SelectEntries(app Application, entryName string, renderer config.PathRenderer) []Entry {
	selected := make([]Entry, 0, len(app.Entries))
	for _, entry := range app.Entries {
		if entryName != "" && entry.Ref.Name != entryName {
			continue
		}
		if !config.EvaluateWhen(entry.AppWhen, renderer) || !config.EvaluateWhen(entry.Value.When, renderer) {
			continue
		}
		selected = append(selected, entry)
	}
	return selected
}
