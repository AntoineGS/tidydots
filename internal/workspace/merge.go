package workspace

import (
	"errors"
	"fmt"
)

// Applications returns merged apps, preserving public order and then appending
// private-only apps. Within a merged app, entries retain repository order.
func (w *Workspace) Applications() ([]Application, error) {
	byName := make(map[string]int)
	apps := make([]Application, 0)
	entryOwners := make(map[string]EntryRef)
	packageOwners := make(map[string]Role)
	for _, repository := range w.Repositories {
		if repository == nil || repository.Config == nil {
			return nil, errors.New("workspace contains an uninitialized repository")
		}
		for _, sourceApp := range repository.Config.Applications {
			index, exists := byName[sourceApp.Name]
			if !exists {
				index = len(apps)
				byName[sourceApp.Name] = index
				apps = append(apps, Application{Name: sourceApp.Name})
			}
			merged := &apps[index]
			if repository.Role == Public {
				if sourceApp.Description != "" {
					merged.Description = sourceApp.Description
				}
			} else if merged.Description == "" && sourceApp.Description != "" {
				merged.Description = sourceApp.Description
			}
			if sourceApp.Package != nil {
				if previous, duplicate := packageOwners[sourceApp.Name]; duplicate {
					return nil, fmt.Errorf("application %q has package definitions in both %s and %s repositories", sourceApp.Name, previous, repository.Role)
				}
				packageOwners[sourceApp.Name] = repository.Role
				merged.Package = sourceApp.Package
				merged.PackageOwner = repository.Role
			}
			for _, sourceEntry := range sourceApp.Entries {
				ref := EntryRef{Repository: repository.Role, App: sourceApp.Name, Name: sourceEntry.Name}
				key := sourceApp.Name + "\x00" + sourceEntry.Name
				if previous, duplicate := entryOwners[key]; duplicate {
					return nil, fmt.Errorf("application %q entry %q is defined in both %s and %s repositories", sourceApp.Name, sourceEntry.Name, previous.Repository, repository.Role)
				}
				entryOwners[key] = ref
				merged.Entries = append(merged.Entries, Entry{Ref: ref, Value: sourceEntry, AppWhen: sourceApp.When})
			}
		}
	}
	return apps, nil
}
