package tui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

func TestWorkspaceModelMergesAppsWithStableOwnerReferences(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	public := workspaceConfig("public-app", config.SubEntry{Name: "portable", Backup: "./portable", Targets: map[string]string{"linux": "~/.config/portable"}})
	private := workspaceConfig("private-app", config.SubEntry{Name: "secret", Backup: "./secret", Targets: map[string]string{"linux": "~/.config/secret"}})
	writeWorkspaceConfig(t, publicDir, public)
	writeWorkspaceConfig(t, privateDir, private)
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatalf("workspace.Load() error = %v", err)
	}

	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatalf("newWorkspaceModel() error = %v", err)
	}
	if len(m.Applications) != 1 {
		t.Fatalf("merged applications = %d, want one; %#v", len(m.Applications), m.Applications)
	}
	app := m.Applications[0]
	if app.Application.Name != "shared" || len(app.SubItems) != 2 {
		t.Fatalf("merged app = %q with %d entries, want shared with two", app.Application.Name, len(app.SubItems))
	}
	if got := app.SubItems[0].Ref; got != (workspace.EntryRef{Repository: workspace.Public, App: "shared", Name: "portable"}) {
		t.Errorf("public ref = %+v", got)
	}
	if got := app.SubItems[1].Ref; got != (workspace.EntryRef{Repository: workspace.Private, App: "shared", Name: "secret"}) {
		t.Errorf("private ref = %+v", got)
	}
	if m.Config != w.Repositories[0].Config {
		t.Fatal("workspace model replaced public source config with a merged persistable config")
	}

	m.Applications[0].Expanded = true
	rows := flattenApplications(m.Applications, platform.OSLinux, true)
	if len(rows) != 3 {
		t.Fatalf("visible table rows = %d, want app plus two entries", len(rows))
	}
	if got := rows[1].Data[2]; got == "" || !containsText(got, "Public") {
		t.Errorf("public entry badge = %q, want Public", got)
	}
	if got := rows[2].Data[2]; got == "" || !containsText(got, "Private") {
		t.Errorf("private entry badge = %q, want Private", got)
	}
}

func TestComputeWorkspaceStatusUsesBothOwnersWithoutCreatingState(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, workspaceConfig("public-app", config.SubEntry{
		Name: "portable", Backup: "./portable", Targets: map[string]string{"linux": "~/.config/portable"},
	}))
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private-app", config.SubEntry{
		Name: "secret", Backup: "./secret", Targets: map[string]string{"linux": "~/.config/secret"},
	}))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatalf("workspace.Load() error = %v", err)
	}

	report, err := ComputeWorkspaceStatus(w, &platform.Platform{OS: platform.OSLinux}, false)
	if err != nil {
		t.Fatalf("ComputeWorkspaceStatus() error = %v", err)
	}
	if len(report.Applications) != 1 || len(report.Applications[0].Entries) != 2 {
		t.Fatalf("workspace status = %+v, want one app with two entries", report)
	}
	owners := map[string]string{}
	for _, entry := range report.Applications[0].Entries {
		owners[entry.Name] = entry.Repository
	}
	if owners["portable"] != string(workspace.Public) || owners["secret"] != string(workspace.Private) {
		t.Fatalf("status owners = %#v", owners)
	}
	for _, dir := range []string{publicDir, privateDir} {
		if _, err := os.Stat(filepath.Join(dir, ".tidydots.db")); !os.IsNotExist(err) {
			t.Fatalf("status created state database under %s: err=%v", dir, err)
		}
	}
}

func TestWorkspaceAsyncStateUsesStableOwnerRefAndIgnoresStaleGeneration(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, workspaceConfig("public-app", config.SubEntry{
		Name: "portable", Backup: "./portable", Targets: map[string]string{"linux": "~/.config/portable"},
	}))
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private-app", config.SubEntry{
		Name: "secret", Backup: "./secret", Targets: map[string]string{"linux": "~/.config/secret"},
	}))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatalf("workspace.Load() error = %v", err)
	}
	readers := openWorkspaceReaders(w, &platform.Platform{OS: platform.OSLinux}, false)
	defer closeWorkspaceManagers(readers)
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, readers)
	if err != nil {
		t.Fatalf("newWorkspaceModel() error = %v", err)
	}
	privateItem := m.Applications[0].SubItems[1]
	if got := managerForSubEntry(m, privateItem); got != readers[workspace.Private] {
		t.Fatal("private entry did not resolve to its private repository manager")
	}

	stale := stateCheckResultMsg{ref: privateItem.Ref, generation: m.WorkspaceGeneration - 1, state: StateModified}
	updated, _ := m.handleStateCheckResult(stale)
	m = updated.(Model)
	if got := m.Applications[0].SubItems[1].State; got == StateModified {
		t.Fatal("stale async result changed the workspace entry")
	}

	current := stateCheckResultMsg{appIndex: 99, subIndex: 99, ref: privateItem.Ref, generation: m.WorkspaceGeneration, state: StateModified}
	updated, _ = m.handleStateCheckResult(current)
	m = updated.(Model)
	if got := m.Applications[0].SubItems[1].State; got != StateModified {
		t.Fatalf("stable owner reference routed state to %s, want Modified", got)
	}
}

func TestWorkspaceSelectionDistinguishesSameNameAcrossOwners(t *testing.T) {
	m := Model{
		Applications: []ApplicationItem{{
			Application: config.Application{Name: "shared"},
			SubItems: []SubEntryItem{
				{SubEntry: config.SubEntry{Name: "same"}, Ref: workspace.EntryRef{Repository: workspace.Public, App: "shared", Name: "same"}, ShowRepository: true},
				{SubEntry: config.SubEntry{Name: "same"}, Ref: workspace.EntryRef{Repository: workspace.Private, App: "shared", Name: "same"}, ShowRepository: true},
			},
		}},
		selectedApps:       make(map[string]bool),
		selectedSubEntries: make(map[subEntryKey]bool),
	}
	m.toggleSubEntrySelection(0, 1)
	if !m.isSubEntrySelected("shared", "same", string(workspace.Private)) {
		t.Fatal("private duplicate was not selected")
	}
	if m.isSubEntrySelected("shared", "same", string(workspace.Public)) {
		t.Fatal("public duplicate appeared selected with private duplicate")
	}
}

func TestWorkspaceAddApplicationWritesOnlyItsOwningConfig(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	public := workspaceConfig("portable", config.SubEntry{
		Name: "same", Backup: "./public", Targets: map[string]string{"linux": "~/.config/public"},
	})
	private := workspaceConfig("private", config.SubEntry{
		Name: "secret", Backup: "./private", Targets: map[string]string{"linux": "~/.config/private"},
	})
	writeWorkspaceConfig(t, publicDir, public)
	writeWorkspaceConfig(t, privateDir, private)
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatalf("workspace.Load() error = %v", err)
	}
	readers := openWorkspaceReaders(w, &platform.Platform{OS: platform.OSLinux}, false)
	defer closeWorkspaceManagers(readers)
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, readers)
	if err != nil {
		t.Fatalf("newWorkspaceModel() error = %v", err)
	}

	if err := m.saveNewApplication(config.Application{Name: "new-public", Entries: []config.SubEntry{}}); err != nil {
		t.Fatalf("saveNewApplication() error = %v", err)
	}
	reloaded, err := config.Load(w.Repositories[0].ConfigPath)
	if err != nil {
		t.Fatalf("reload public config: %v", err)
	}
	if len(reloaded.Applications) != 2 || reloaded.Applications[0].Entries[0].Backup != "./public" || reloaded.Applications[1].Name != "new-public" {
		t.Fatalf("public source config changed: %+v", reloaded.Applications)
	}
	privateLoaded, err := config.Load(w.Repositories[1].ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(privateLoaded.Applications) != 1 || privateLoaded.Applications[0].Entries[0].Backup != "./private" {
		t.Fatalf("private source config changed: %+v", privateLoaded.Applications)
	}
}

func TestWorkspaceEntryEditRoutesThroughStableOwnerRef(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, workspaceConfig("public", config.SubEntry{Name: "public-entry", Backup: "./public", Targets: map[string]string{"linux": "~/.config/public"}}))
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private", config.SubEntry{Name: "private-entry", Backup: "./private", Targets: map[string]string{"linux": "~/.config/private"}}))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	privateIndex := -1
	for i, item := range m.Applications[0].SubItems {
		if item.Ref.Repository == workspace.Private {
			privateIndex = i
		}
	}
	if privateIndex < 0 {
		t.Fatal("private entry missing from merged view")
	}
	m.initSubEntryForm(0, privateIndex)
	if m.subEntryForm == nil || m.subEntryForm.EntryRef == nil || m.subEntryForm.EntryRef.Repository != workspace.Private {
		t.Fatal("private entry form lost its owner reference")
	}
	updatedWhen := `{{ eq .OS "linux" }}`
	m.subEntryForm.WhenInput.SetValue(updatedWhen)
	if err := m.saveSubEntryForm(); err != nil {
		t.Fatalf("saveSubEntryForm() error = %v", err)
	}
	publicLoaded, err := config.Load(filepath.Join(publicDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	privateLoaded, err := config.Load(filepath.Join(privateDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if publicLoaded.Applications[0].Entries[0].When != "" {
		t.Fatalf("public owner changed: %+v", publicLoaded.Applications[0].Entries[0])
	}
	if privateLoaded.Applications[0].Entries[0].When != updatedWhen {
		t.Fatalf("private owner entry = %+v, want When %q", privateLoaded.Applications[0].Entries[0], updatedWhen)
	}
}

func TestWorkspaceSetupOwnershipMoveShowsConfirmationAndAppliesAsync(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	entry := config.SubEntry{Name: "enable-service", Check: map[string]string{"linux": "command -v service"}, Run: map[string]string{"linux": "install service"}}
	writeWorkspaceConfig(t, publicDir, workspaceConfig("public", entry))
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private"))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := m.Applications[0].SubItems[0].Ref
	if err := m.saveWorkspaceEntry(ref, entry, workspace.Private); err != nil {
		t.Fatalf("saveWorkspaceEntry() error = %v", err)
	}
	if m.Screen != ScreenSummary || m.pendingMove == nil {
		t.Fatalf("move screen = %v, pending plan = %v; want confirmation", m.Screen, m.pendingMove)
	}
	before, err := os.ReadFile(filepath.Join(publicDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	updated, cmd := m.updateSummary(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.Screen != ScreenProgress || cmd == nil {
		t.Fatalf("confirm returned screen=%v cmd=%v, want progress and async command", m.Screen, cmd)
	}
	if got, err := os.ReadFile(filepath.Join(publicDir, "tidydots.yaml")); err != nil || string(got) != string(before) {
		t.Fatalf("manifest changed before command ran: %v", err)
	}
	msg, ok := cmd().(workspaceMoveCompleteMsg)
	if !ok {
		t.Fatalf("move command returned %T, want workspaceMoveCompleteMsg", cmd())
	}
	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.err != nil {
		t.Fatalf("move completion error = %v", m.err)
	}
	publicLoaded, err := config.Load(filepath.Join(publicDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	privateLoaded, err := config.Load(filepath.Join(privateDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(publicLoaded.Applications) != 0 || len(privateLoaded.Applications) != 1 || privateLoaded.Applications[0].Entries[0].Name != entry.Name {
		t.Fatalf("move owners after apply: public=%+v private=%+v", publicLoaded.Applications, privateLoaded.Applications)
	}
}

func TestWorkspaceDeleteEntryUsesOwnerAndScopedApplicationEdit(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	public := workspaceConfig("public", config.SubEntry{Name: "keep", Backup: "./public", Targets: map[string]string{"linux": "~/.config/public"}})
	private := workspaceConfig("private", config.SubEntry{Name: "remove", Backup: "./private", Targets: map[string]string{"linux": "~/.config/private"}}, config.SubEntry{Name: "stay", Backup: "./private-stay", Targets: map[string]string{"linux": "~/.config/private-stay"}})
	public.Applications[0].Description = "public description"
	private.Applications[0].Description = "private description"
	writeWorkspaceConfig(t, publicDir, public)
	writeWorkspaceConfig(t, privateDir, private)
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	privateIndex := -1
	for i, item := range m.Applications[0].SubItems {
		if item.Ref.Repository == workspace.Private && item.Ref.Name == "remove" {
			privateIndex = i
		}
	}
	if err := m.deleteApplicationOrSubEntry(0, privateIndex); err != nil {
		t.Fatalf("delete private entry: %v", err)
	}
	if err := m.saveWorkspaceApplication(workspace.Private, "shared", "shared", "updated private", "", nil); err != nil {
		t.Fatalf("edit private app: %v", err)
	}
	publicLoaded, err := config.Load(filepath.Join(publicDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	privateLoaded, err := config.Load(filepath.Join(privateDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(publicLoaded.Applications) != 1 || publicLoaded.Applications[0].Description != "public description" || len(publicLoaded.Applications[0].Entries) != 1 {
		t.Fatalf("public source changed: %+v", publicLoaded.Applications)
	}
	if len(privateLoaded.Applications) != 1 || privateLoaded.Applications[0].Description != "updated private" || len(privateLoaded.Applications[0].Entries) != 1 || privateLoaded.Applications[0].Entries[0].Name != "stay" {
		t.Fatalf("private source mismatch: %+v", privateLoaded.Applications)
	}
}

func TestWorkspaceMutationValidationIncludesInactiveCrossRepositoryClaims(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	publicEntry := config.SubEntry{Name: "public-entry", Backup: "./public", Targets: map[string]string{"linux": "~/.config/shared"}}
	privateEntry := config.SubEntry{Name: "private-entry", Backup: "./private", When: `{{ eq .OS "windows" }}`, Targets: map[string]string{"linux": "~/.config/shared"}}
	writeWorkspaceConfig(t, publicDir, workspaceConfig("public", publicEntry))
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private", privateEntry))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var publicRef workspace.EntryRef
	for _, item := range m.Applications[0].SubItems {
		if item.Ref.Repository == workspace.Public {
			publicRef = item.Ref
		}
	}
	updated := publicEntry
	updated.When = `{{ eq .OS "linux" }}`
	if err := m.saveWorkspaceEntry(publicRef, updated, workspace.Public); err == nil || !containsText(err.Error(), "target") {
		t.Fatalf("saveWorkspaceEntry error = %v, want complete-workspace target conflict", err)
	}
	loaded, err := config.Load(filepath.Join(publicDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Applications[0].Entries[0].When != "" {
		t.Fatal("conflicting write changed public source config")
	}
}

func TestTask5ReviewInactiveOwnerCannotRestoreFromUnfilteredMergedApp(t *testing.T) {
	targetPrivate := filepath.Join(t.TempDir(), "private-target")
	public := workspaceConfig("public", config.SubEntry{Name: "public-config", Backup: "./public", Targets: map[string]string{"linux": filepath.Join(t.TempDir(), "public-target")}})
	private := workspaceConfig("private", config.SubEntry{Name: "private-config", Backup: "./private", Targets: map[string]string{"linux": targetPrivate}})
	private.Applications[0].When = `{{ false }}`
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, public)
	writeWorkspaceConfig(t, privateDir, private)
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.filterEnabled = false
	m.pendingStateChecks = 0
	m.initApplicationItems()
	m.Applications[0].Expanded = true
	m.rebuildTable()
	var inactive SubEntryItem
	for _, item := range m.Applications[0].SubItems {
		if item.Ref.Repository == workspace.Private {
			inactive = item
		}
	}
	if !inactive.Filtered {
		t.Fatal("fixture private contribution should be inactive")
	}
	// Individual-row restore is rejected even though the inactive row is visible.
	if success, message := m.performRestoreSubEntry(inactive); success || !containsText(message, "inactive") {
		t.Fatalf("inactive restore = %t, %q", success, message)
	}
	for i, row := range m.tableRows {
		if row.Repository == string(workspace.Private) && row.SubIndex >= 0 {
			m.tableCursor = i
			break
		}
	}
	updated, cmd := m.updateResults(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = updated.(Model)
	if cmd != nil || len(m.pendingConfigRestores) != 0 {
		t.Fatal("individual restore key dispatched an inactive contribution")
	}
	if _, err := os.Lstat(targetPrivate); !os.IsNotExist(err) {
		t.Fatalf("individual restore created inactive target: %v", err)
	}
	// App-row dispatch queues only active config contributions.
	for i, row := range m.tableRows {
		if row.AppName == "shared" && row.SubIndex < 0 {
			m.tableCursor = i
			break
		}
	}
	updated, _ = m.updateResults(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = updated.(Model)
	for _, queued := range m.pendingConfigRestores {
		if queued.item.Ref.Repository == workspace.Private {
			t.Fatal("app restore queued inactive private config")
		}
	}
	if len(m.pendingConfigRestores) != 1 || m.pendingConfigRestores[0].item.Ref.Repository != workspace.Public {
		appAtCursor, subAtCursor := m.getApplicationAtCursorFromTable()
		t.Fatalf("queued config restores = %+v; cursor=%d row=%+v app=%d sub=%d filtered=%t; want public active item only", m.pendingConfigRestores, m.tableCursor, m.tableRows[m.tableCursor], appAtCursor, subAtCursor, m.Applications[appAtCursor].IsFiltered)
	}
}

func TestTask5ReviewInactiveEntryConditionCannotRunSetupWhenFilterIsOff(t *testing.T) {
	active := config.SubEntry{Name: "active-setup", Check: map[string]string{"linux": "true"}, Run: map[string]string{"linux": "true"}}
	inactive := config.SubEntry{Name: "inactive-setup", When: `{{ false }}`, Check: map[string]string{"linux": "true"}, Run: map[string]string{"linux": "false"}}
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, workspaceConfig("public", active))
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private", inactive))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.filterEnabled = false
	m.pendingStateChecks = 0
	m.initApplicationItems()
	m.pendingStateChecks = 0
	m.Applications[0].Expanded = true
	m.rebuildTable()
	var inactiveItem SubEntryItem
	for _, item := range m.Applications[0].SubItems {
		if item.Ref.Repository == workspace.Private {
			inactiveItem = item
		}
	}
	if success, message := m.performRestoreSubEntry(inactiveItem); success || !containsText(message, "inactive") {
		t.Fatalf("inactive setup dispatch = %t, %q", success, message)
	}
	if got := m.collectAppSetupItems(0); len(got) != 1 || got[0].sub.Ref.Repository != workspace.Public {
		t.Fatalf("app-row setup queue = %+v; want public active setup only", got)
	}
	for i, row := range m.tableRows {
		if row.Repository == string(workspace.Private) && row.SubIndex >= 0 {
			m.tableCursor = i
			break
		}
	}
	updated, cmd := m.updateResults(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("individual setup restore key dispatched inactive entry")
	}
	for i, row := range m.tableRows {
		if row.SubIndex < 0 {
			m.tableCursor = i
			break
		}
	}
	updated, cmd = m.updateResults(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = updated.(Model)
	if cmd == nil || len(m.pendingSetups) != 1 || m.pendingSetups[0].sub.Ref.Repository != workspace.Public {
		t.Fatalf("app-row setup key queued wrong owner(s): cmd=%t queued=%+v", cmd != nil, m.pendingSetups)
	}
}

func TestTask5ReviewApplicationFormSelectsContributionAndPersistsScopedDraft(t *testing.T) {
	public := workspaceConfig("public", config.SubEntry{Name: "public-config", Backup: "./public", Targets: map[string]string{"linux": "~/.config/public"}})
	private := workspaceConfig("private", config.SubEntry{Name: "private-config", Backup: "./private", Targets: map[string]string{"linux": "~/.config/private"}})
	public.Applications[0].Description, public.Applications[0].When = "Public description", `{{ eq .OS "linux" }}`
	private.Applications[0].Description, private.Applications[0].When = "Private description", `{{ eq .OS "windows" }}`
	private.Applications[0].Package = &config.EntryPackage{Managers: map[string]config.ManagerValue{"apt": {PackageName: "private-pkg"}}}
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, public)
	writeWorkspaceConfig(t, privateDir, private)
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.initApplicationForm(0)
	if got := m.applicationForm.RepositoryRole; got != workspace.Private {
		t.Fatalf("initial owner = %s, want existing package owner private", got)
	}
	m.applicationForm.FocusIndex = 4
	updated, _ := m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = updated.(Model)
	if got := m.applicationForm.RepositoryRole; got != workspace.Public {
		t.Fatalf("left scope selection = %s, want public", got)
	}
	if got := m.applicationForm.DescriptionInput.Value(); got != "Public description" {
		t.Fatalf("scope did not load public draft description: %q", got)
	}
	if got := m.applicationForm.WhenInput.Value(); got != `{{ eq .OS "linux" }}` {
		t.Fatalf("scope did not load public condition: %q", got)
	}
	if got := m.applicationForm.PackageManagers["apt"]; got != "" {
		t.Fatalf("private package leaked into public draft: %q", got)
	}
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(Model)
	if got := m.applicationForm.RepositoryRole; got != workspace.Private {
		t.Fatalf("right scope selection = %s, want private", got)
	}
	if got := m.applicationForm.DescriptionInput.Value(); got != "Private description" {
		t.Fatalf("scope did not load private draft description: %q", got)
	}
	if got := m.applicationForm.WhenInput.Value(); got != `{{ eq .OS "windows" }}` {
		t.Fatalf("scope did not load private condition: %q", got)
	}
	m.applicationForm.DescriptionInput.SetValue("Edited private")
	m.applicationForm.WhenInput.SetValue(`{{ eq .OS "linux" }}`)
	m.applicationForm.NameInput.SetValue("renamed-shared")
	m.applicationForm.FocusIndex = 1
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: 's', Text: "s"})
	m = updated.(Model)
	if m.applicationForm != nil {
		t.Fatalf("form save error: %s", m.applicationForm.Err)
	}
	publicReload, err := config.Load(filepath.Join(publicDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	privateReload, err := config.Load(filepath.Join(privateDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if publicReload.Applications[0].Name != "renamed-shared" || publicReload.Applications[0].Description != "Public description" || publicReload.Applications[0].When != `{{ eq .OS "linux" }}` {
		t.Fatalf("public scoped metadata changed: %+v", publicReload.Applications[0])
	}
	if privateReload.Applications[0].Name != "renamed-shared" || privateReload.Applications[0].Description != "Edited private" || privateReload.Applications[0].When != `{{ eq .OS "linux" }}` {
		t.Fatalf("private scoped metadata was not persisted: %+v", privateReload.Applications[0])
	}
	if len(publicReload.Applications[0].Entries) != 1 || publicReload.Applications[0].Entries[0].Name != "public-config" || len(privateReload.Applications[0].Entries) != 1 || privateReload.Applications[0].Entries[0].Name != "private-config" {
		t.Fatalf("atomic application rename did not preserve owner entries: public=%+v private=%+v", publicReload.Applications[0].Entries, privateReload.Applications[0].Entries)
	}
	if got := privateReload.Applications[0].Package.Managers["apt"].PackageName; got != "private-pkg" {
		t.Fatalf("private package ownership lost: %q", got)
	}
}

func TestTask5ReviewInspectorRefreshUsesNewReadOnlySnapshot(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	public := workspaceConfig("public", config.SubEntry{Name: "template", Backup: "./source", Files: []string{"f.tmpl"}, Targets: map[string]string{"linux": target}})
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, public)
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private"))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	platformInfo := &platform.Platform{OS: platform.OSLinux}
	readers := openWorkspaceReaders(w, platformInfo, false)
	m, err := newWorkspaceModel(w, platformInfo, false, false, readers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeWorkspaceManagers(m.WriteManagers)
		closeAllWorkspaceReaders(m.RepositoryManagers, m.readerTracker)
	})
	if err := os.MkdirAll(filepath.Join(publicDir, "source"), 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(publicDir, "source", "f.tmpl")
	if err := os.WriteFile(source, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	item := m.Applications[0].SubItems[0]
	if ok, message := m.performConfigRestore(item, false); !ok {
		t.Fatal(message)
	}
	// Refresh the inspector to establish a real immutable pre-operation baseline.
	oldReader := m.RepositoryManagers[workspace.Public]
	if err := os.WriteFile(source, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	if ok, message := m.performConfigRestore(item, false); !ok {
		t.Fatal(message)
	}
	if err := m.refreshWorkspace(nil, ""); err != nil {
		t.Fatal(err)
	}
	if m.RepositoryManagers[workspace.Public] == oldReader {
		t.Fatal("workspace refresh reused the old immutable inspector")
	}
	if got := m.detectSubEntryState(&m.Applications[0].SubItems[0]); got != StateLinked {
		t.Fatalf("refreshed template state = %v, want Linked", got)
	}
}

func TestTask5ReviewEntrySaveRetainsUnchangedPackageResult(t *testing.T) {
	public := workspaceConfig("public", config.SubEntry{Name: "config", Backup: "./source", Targets: map[string]string{"linux": filepath.Join(t.TempDir(), "target")}})
	public.Applications[0].Package = &config.EntryPackage{Managers: map[string]config.ManagerValue{"git": {Git: &config.GitPackage{URL: "https://example.invalid/repo.git", Targets: map[string]string{"linux": filepath.Join(t.TempDir(), "pkg")}}}}}
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, public)
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private"))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	installed := true
	m.Applications[0].PkgInstalled, m.Applications[0].PkgMethod = &installed, TypeGit
	ref := m.Applications[0].SubItems[0].Ref
	entry := m.Applications[0].SubItems[0].SubEntry
	entry.Backup = "./changed"
	if err := m.saveWorkspaceEntry(ref, entry, workspace.Public); err != nil {
		t.Fatal(err)
	}
	if m.Applications[0].PkgInstalled == nil || !*m.Applications[0].PkgInstalled || m.Applications[0].PkgMethod != TypeGit {
		t.Fatalf("unchanged package status was lost after entry save: installed=%v method=%q", m.Applications[0].PkgInstalled, m.Applications[0].PkgMethod)
	}
}

func TestTask5ReviewConfigRestoreKeyDispatchesExecInsteadOfBlockingUpdate(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	public := workspaceConfig("public", config.SubEntry{Name: "config", Backup: "./source", Targets: map[string]string{"linux": target}})
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, public)
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private"))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.Applications[0].Expanded = true
	m.pendingStateChecks = 0
	m.rebuildTable()
	for i, row := range m.tableRows {
		if row.SubIndex >= 0 {
			m.tableCursor = i
			break
		}
	}
	updated, cmd := m.updateResults(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = updated.(Model)
	if cmd == nil || len(m.pendingConfigRestores) != 1 || !m.processing {
		appAtCursor, subAtCursor := m.getApplicationAtCursorFromTable()
		t.Fatalf("restore key did not dispatch queued command: cmd=%v queue=%d processing=%t cursor=%d row=%+v app=%d sub=%d", cmd != nil, len(m.pendingConfigRestores), m.processing, m.tableCursor, m.tableRows[m.tableCursor], appAtCursor, subAtCursor)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("restore side effect ran inside Update: %v", err)
	}
}

func TestTask5ReviewMoveSelectionFollowsCommittedOwnerChange(t *testing.T) {
	entry := config.SubEntry{Name: "setup", Check: map[string]string{"linux": "true"}, Run: map[string]string{"linux": "true"}}
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, workspaceConfig("public", entry))
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private"))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.toggleSubEntrySelection(0, 0)
	oldRef := m.Applications[0].SubItems[0].Ref
	if err := m.saveWorkspaceEntry(oldRef, entry, workspace.Private); err != nil {
		t.Fatal(err)
	}
	if err := m.Workspace.ApplyMove(context.Background(), m.pendingMove, false); err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(workspaceMoveCompleteMsg{})
	m = updated.(Model)
	if !m.isSubEntrySelected("shared", "setup", string(workspace.Private)) {
		t.Fatal("committed owner move dropped entry selection")
	}
}

func TestTask5ReviewCommittedMoveErrorReloadsInspectableDiskOwnership(t *testing.T) {
	entry := config.SubEntry{Name: "setup", Check: map[string]string{"linux": "true"}, Run: map[string]string{"linux": "true"}}
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, workspaceConfig("public", entry))
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private"))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref := m.Applications[0].SubItems[0].Ref
	if err := m.saveWorkspaceEntry(ref, entry, workspace.Private); err != nil {
		t.Fatal(err)
	}
	if err := m.Workspace.ApplyMove(context.Background(), m.pendingMove, false); err != nil {
		t.Fatal(err)
	}
	updated, _ := m.Update(workspaceMoveCompleteMsg{err: &workspace.TransactionError{ID: "task5-committed-cleanup", Outcome: workspace.OutcomeCommittedCleanupPending, Cause: errors.New("cleanup pending")}})
	m = updated.(Model)
	if m.Applications[0].SubItems[0].Ref.Repository != workspace.Private {
		t.Fatal("committed cleanup outcome left cached ownership stale")
	}
	if m.err == nil || !containsText(m.err.Error(), "task5-committed-cleanup") || !containsText(m.err.Error(), "committed") {
		t.Fatalf("transaction warning/ID not retained: %v", m.err)
	}
}

func TestTask5ReviewWorkspaceHostnameFallbackUsesLocalMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := []string{"local-host-a", "local-host-b"}
	if err := config.SaveAppConfig(&config.AppConfig{Hostnames: want}); err != nil {
		t.Fatal(err)
	}
	public := workspace.Repository{Role: workspace.Public, Config: &config.Config{Version: 3}}
	private := workspace.Repository{Role: workspace.Private, Config: &config.Config{Version: 3}}
	w := &workspace.Workspace{Repositories: []*workspace.Repository{&public, &private}}
	if got := workspaceHostnames(w); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("hostname fallback = %v, want %v", got, want)
	}
}

func TestTask5StaleOwnerSnapshotBlocksRestoreSetupAndPackageSideEffects(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	liveDir := filepath.Join(t.TempDir(), "live-copy")
	if err := os.MkdirAll(liveDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "f"), []byte("private live bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	setupMarker := filepath.Join(t.TempDir(), "setup-ran")
	packageMarker := filepath.Join(t.TempDir(), "package-ran")
	pkg := &config.EntryPackage{Custom: map[string]string{"linux": "touch " + packageMarker}}
	configEntry := config.SubEntry{Name: "copy-config", Backup: "./backup", Files: []string{"f"}, Method: config.MethodCopy, Targets: map[string]string{"linux": liveDir}}
	setupEntry := config.SubEntry{Name: "setup", Check: map[string]string{"linux": "test -f " + setupMarker}, Run: map[string]string{"linux": "touch " + setupMarker}}
	public := workspaceConfig("public", configEntry, setupEntry)
	public.Applications[0].Package = pkg
	writeWorkspaceConfig(t, publicDir, public)
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private"))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.pendingStateChecks = 0
	var copyItem, setupItem SubEntryItem
	for _, item := range m.Applications[0].SubItems {
		switch item.SubEntry.Name {
		case "copy-config":
			copyItem = item
		case "setup":
			setupItem = item
		}
	}
	if copyItem.Ref.Repository != workspace.Public || setupItem.Ref.Repository != workspace.Public {
		t.Fatal("fixture items were not Public-owned")
	}

	// Simulate a different process completing the ownership move after this
	// model loaded its Public snapshot: the old copy definition/package/setup
	// now belong to Private, while its existing live target contains private data.
	publicAfter := workspaceConfig("public")
	privateAfter := workspaceConfig("private", configEntry, setupEntry)
	privateAfter.Applications[0].Package = pkg
	writeWorkspaceConfig(t, publicDir, publicAfter)
	writeWorkspaceConfig(t, privateDir, privateAfter)

	if success, message := m.performConfigRestore(copyItem, false); success || !containsText(message, "refresh") {
		t.Fatalf("stale copy restore = %t, %q; want refresh guidance", success, message)
	}
	if got, err := os.ReadFile(filepath.Join(liveDir, "f")); err != nil || string(got) != "private live bytes" {
		t.Fatalf("stale public copy changed private live bytes: %q, %v", got, err)
	}
	if success, message := m.runSetupForItem(setupItem); success || !containsText(message, "refresh") {
		t.Fatalf("stale setup = %t, %q; want refresh guidance", success, message)
	}
	if _, err := os.Stat(setupMarker); !os.IsNotExist(err) {
		t.Fatalf("stale setup command ran: %v", err)
	}
	packageExec := m.newPackageExec(PackageItem{Name: "shared", Package: pkg, Repository: workspace.Public})
	if packageExec == nil {
		t.Fatal("expected package execution")
	}
	if err := packageExec.Run(); err == nil || !containsText(err.Error(), "refresh") {
		t.Fatalf("stale package install error = %v; want refresh guidance", err)
	}
	if _, err := os.Stat(packageMarker); !os.IsNotExist(err) {
		t.Fatalf("stale package command ran: %v", err)
	}
	if len(m.WriteManagers) != 0 {
		t.Fatalf("stale owner initialized writable managers: %v", m.WriteManagers)
	}
}

func TestTask5ManualRefreshReloadsWorkspaceOwnership(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	entry := config.SubEntry{Name: "owned", Backup: "./backup", Targets: map[string]string{"linux": filepath.Join(t.TempDir(), "target")}}
	writeWorkspaceConfig(t, publicDir, workspaceConfig("shared", entry))
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private"))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeAllWorkspaceReaders(m.RepositoryManagers, m.readerTracker) })
	if got := m.Applications[0].SubItems[0].Ref.Repository; got != workspace.Public {
		t.Fatalf("initial owner = %s; want Public", got)
	}

	writeWorkspaceConfig(t, publicDir, workspaceConfig("shared"))
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private", entry))
	cmd := m.refreshAllStates()
	if cmd == nil {
		t.Fatalf("manual refresh failed: %v", m.err)
	}
	// Run the batched checks so their read leases are released in this test.
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, child := range batch {
			child()
		}
	}
	if len(m.Applications) != 1 {
		t.Fatalf("refreshed applications = %d; want one merged application", len(m.Applications))
	}
	var refreshedOwner workspace.Role
	for _, app := range m.Applications {
		for _, item := range app.SubItems {
			if item.SubEntry.Name == "owned" {
				refreshedOwner = item.Ref.Repository
			}
		}
	}
	if refreshedOwner != workspace.Private {
		t.Fatalf("manual refresh retained stale owner %q; want Private", refreshedOwner)
	}
}

func TestWorkspacePreflightAllowsSameRepositoryFolderComposition(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	public := workspaceConfig("public",
		config.SubEntry{Name: "folder", Backup: "./folder", Targets: map[string]string{"linux": "~/.config/composed"}},
		config.SubEntry{Name: "child", Backup: "./child", Files: []string{"nested/file"}, Targets: map[string]string{"linux": "~/.config/composed"}},
	)
	private := workspaceConfig("private", config.SubEntry{Name: "private", Backup: "./private", Targets: map[string]string{"linux": "~/.config/private"}})
	writeWorkspaceConfig(t, publicDir, public)
	writeWorkspaceConfig(t, privateDir, private)
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.validateWorkspaceTargets(); err != nil {
		t.Fatalf("cross-repository preflight rejected intentional same-owner composition: %v", err)
	}
}

func TestWorkspaceTransactionErrorsSurfaceRecoveryOutcomeAndID(t *testing.T) {
	for _, tc := range []struct {
		outcome workspace.TransactionOutcome
		text    string
	}{
		{workspace.OutcomeRolledBack, "rolled back"},
		{workspace.OutcomePending, "needs recovery"},
		{workspace.OutcomeCommittedCleanupPending, "committed, but cleanup needs recovery"},
	} {
		err := transactionOutcomeError(&workspace.TransactionError{ID: "txn-123", Outcome: tc.outcome, Cause: errors.New("injected failure")})
		if err == nil || !containsText(err.Error(), tc.text) || !containsText(err.Error(), "txn-123") {
			t.Errorf("transaction outcome %q produced error %v", tc.outcome, err)
		}
	}
}

func TestWorkspaceRestoreGuardRunsBeforeWritableStateInitialization(t *testing.T) {
	publicDir, privateDir := t.TempDir(), t.TempDir()
	entry := config.SubEntry{Name: "config", Backup: "./backup", Targets: map[string]string{"linux": "~/.config/guarded"}}
	writeWorkspaceConfig(t, publicDir, workspaceConfig("public", entry))
	writeWorkspaceConfig(t, privateDir, workspaceConfig("private"))
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	journalDir := filepath.Join(t.TempDir(), "journal")
	if err := os.MkdirAll(journalDir, 0700); err != nil {
		t.Fatal(err)
	}
	pending, err := json.Marshal(map[string]any{"id": "pending-restore", "public_root": w.Repositories[0].Root, "private_root": w.Repositories[1].Root})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalDir, "pending-restore.json"), pending, 0600); err != nil {
		t.Fatal(err)
	}
	w.JournalDir = journalDir
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	ok, message := m.performRestoreSubEntry(m.Applications[0].SubItems[0])
	if ok || !containsText(message, "pending transaction") {
		t.Fatalf("restore = %t, %q; want pending transaction refusal", ok, message)
	}
	if _, err := os.Stat(filepath.Join(publicDir, ".tidydots.db")); !os.IsNotExist(err) {
		t.Fatalf("writer state database initialized before guard: %v", err)
	}
}

func workspaceConfig(appName string, entries ...config.SubEntry) *config.Config {
	return &config.Config{Version: 3, Applications: []config.Application{{Name: "shared", Description: appName, Entries: entries}}}
}

func writeWorkspaceConfig(t *testing.T, root string, cfg *config.Config) {
	t.Helper()
	if err := config.Save(cfg, filepath.Join(root, "tidydots.yaml")); err != nil {
		t.Fatalf("save workspace config: %v", err)
	}
}
