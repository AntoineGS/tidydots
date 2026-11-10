package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

func task5NewReviewModel(t *testing.T, public, private *config.Config) (*Model, string, string) {
	t.Helper()
	publicDir, privateDir := t.TempDir(), t.TempDir()
	writeWorkspaceConfig(t, publicDir, public)
	writeWorkspaceConfig(t, privateDir, private)
	w, err := workspace.Load(publicDir, privateDir)
	if err != nil {
		t.Fatal(err)
	}
	w.JournalDir = filepath.Join(t.TempDir(), "journal")
	m, err := newWorkspaceModel(w, &platform.Platform{OS: platform.OSLinux, Hostname: "task5-review"}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.pendingStateChecks = 0
	t.Cleanup(func() {
		closeWorkspaceManagers(m.WriteManagers)
		closeAllWorkspaceReaders(m.RepositoryManagers, m.readerTracker)
	})
	return &m, publicDir, privateDir
}

func task5UpdateApplication(m *Model, msg tea.KeyPressMsg) tea.Cmd {
	updated, cmd := m.updateApplicationForm(msg)
	*m = updated.(Model)
	return cmd
}

func task5UpdateText(m *Model, value string) {
	for _, r := range value {
		task5UpdateApplication(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func task5FindEntry(t *testing.T, m *Model, role workspace.Role, name string) SubEntryItem {
	t.Helper()
	for _, app := range m.Applications {
		for _, item := range app.SubItems {
			if item.Ref.Repository == role && item.SubEntry.Name == name {
				return item
			}
		}
	}
	t.Fatalf("entry %s/%s not found", role, name)
	return SubEntryItem{}
}

func TestTask5QueuedSetupRejectsOwnerOrCommandChangesAfterRestoreRefresh(t *testing.T) {
	for _, tc := range []struct {
		name      string
		moveOwner bool
	}{
		{name: "owner changed", moveOwner: true},
		{name: "same owner command changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "obsolete-command-ran")
			configEntry := config.SubEntry{Name: "config", Backup: "./source", Files: []string{"f"}, Targets: map[string]string{"linux": filepath.Join(t.TempDir(), "target")}}
			oldSetup := config.SubEntry{Name: "queued", Check: map[string]string{"linux": "test -f " + marker}, Run: map[string]string{"linux": "touch " + marker}}
			public := workspaceConfig("shared")
			private := workspaceConfig("shared")
			m, publicDir, privateDir := task5NewReviewModel(t, public, private)
			writeWorkspaceConfig(t, publicDir, workspaceConfig("shared", configEntry, oldSetup))
			if err := m.refreshWorkspace(nil, ""); err != nil {
				t.Fatal(err)
			}
			configItem := task5FindEntry(t, m, workspace.Public, "config")
			setupItem := task5FindEntry(t, m, workspace.Public, "queued")
			m.pendingConfigRestores = []configRestoreItem{{item: configItem, name: "config"}}
			m.configRestoreSetups = []setupRunItem{{name: "queued", sub: setupItem}}

			replacement := oldSetup
			replacement.Run = map[string]string{"linux": "true"}
			publicAfter := workspaceConfig("shared", configEntry, oldSetup)
			privateAfter := workspaceConfig("shared")
			if tc.moveOwner {
				publicAfter = workspaceConfig("shared", configEntry)
				privateAfter = workspaceConfig("shared", replacement)
			}
			if !tc.moveOwner {
				publicAfter = workspaceConfig("shared", configEntry, replacement)
			}
			writeWorkspaceConfig(t, publicDir, publicAfter)
			writeWorkspaceConfig(t, privateDir, privateAfter)

			updated, _ := m.handleConfigRestoreResult(configRestoreMsg{queued: m.pendingConfigRestores[0], success: true, message: "config restored"})
			model := updated.(Model)
			m = &model
			if len(m.pendingSetups) != 1 {
				t.Fatalf("queued setup count after automatic refresh = %d; want 1", len(m.pendingSetups))
			}
			if err := m.validateFreshWorkspace(); err != nil {
				t.Fatalf("refreshed workspace should be fresh: %v", err)
			}
			if success, message := m.runSetupForItem(m.pendingSetups[0].sub); success || !strings.Contains(message, "stale") {
				t.Fatalf("obsolete queued setup = (%t, %q), want stale-snapshot rejection", success, message)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("obsolete setup command ran: %v", err)
			}
		})
	}
}

func TestTask5QueuedConfigRestoreRejectsChangedDefinitionAfterRefresh(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	entry := config.SubEntry{Name: "config", Backup: "./old-source", Files: []string{"f"}, Targets: map[string]string{"linux": target}}
	m, publicDir, _ := task5NewReviewModel(t, workspaceConfig("shared", entry), workspaceConfig("private"))
	oldItem := task5FindEntry(t, m, workspace.Public, "config")
	changed := entry
	changed.Backup = "./new-source"
	writeWorkspaceConfig(t, publicDir, workspaceConfig("shared", changed))
	if err := m.refreshWorkspace(nil, ""); err != nil {
		t.Fatal(err)
	}
	if success, message := m.performConfigRestore(oldItem, false); success || !strings.Contains(message, "stale") {
		t.Fatalf("obsolete queued config restore = (%t, %q), want stale-definition rejection", success, message)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("obsolete restore touched target: %v", err)
	}
}

func TestTask5QueuedPackageInstallRejectsChangedDefinitionAfterRefresh(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "obsolete-package-command-ran")
	oldPackage := &config.EntryPackage{Custom: map[string]string{"linux": "touch " + marker}}
	public := workspaceConfig("shared")
	public.Applications[0].Package = oldPackage
	m, publicDir, _ := task5NewReviewModel(t, public, workspaceConfig("private"))
	item := PackageItem{Name: "shared", Package: oldPackage, Repository: workspace.Public}
	queued := m.newPackageExec(item)
	changed := &config.EntryPackage{Custom: map[string]string{"linux": "true"}}
	updatedPublic := workspaceConfig("shared")
	updatedPublic.Applications[0].Package = changed
	writeWorkspaceConfig(t, publicDir, updatedPublic)
	if err := m.refreshWorkspace(nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := queued.Run(); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("obsolete queued package install error = %v; want stale-definition rejection", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("obsolete package command ran: %v", err)
	}
}

func TestTask5PackageInstallCompletionRetainsPreflightDiagnostics(t *testing.T) {
	for _, scenario := range []string{"stale snapshot", "stale manifest", "mutation guard refusal"} {
		t.Run(scenario, func(t *testing.T) {
			pkg := &config.EntryPackage{Custom: map[string]string{"linux": "printf should-not-run"}}
			public := workspaceConfig("shared")
			public.Applications[0].Package = pkg
			m, publicDir, _ := task5NewReviewModel(t, public, workspaceConfig("private"))
			item := PackageItem{Name: "shared", Package: pkg, Repository: workspace.Public}
			exec := m.newPackageExec(item)
			if exec == nil {
				t.Fatal("expected package executor")
			}

			var release func()
			switch scenario {
			case "stale snapshot":
				changed := workspaceConfig("shared")
				changed.Applications[0].Package = &config.EntryPackage{Custom: map[string]string{"linux": "true"}}
				writeWorkspaceConfig(t, publicDir, changed)
				if err := m.refreshWorkspace(nil, ""); err != nil {
					t.Fatal(err)
				}
			case "stale manifest":
				changed := workspaceConfig("changed description")
				changed.Applications[0].Package = pkg
				writeWorkspaceConfig(t, publicDir, changed)
			case "mutation guard refusal":
				roots := make([]string, 0, len(m.Workspace.Repositories))
				for _, repo := range m.Workspace.Repositories {
					roots = append(roots, repo.Root)
				}
				var err error
				release, err = workspace.MutationGuard(t.Context(), m.Workspace.JournalDir, roots...)
				if err != nil {
					t.Fatalf("hold mutation guard: %v", err)
				}
			}
			if release != nil {
				defer release()
			}

			err := exec.Run()
			if err == nil {
				t.Fatal("expected package preflight refusal")
			}
			m.pendingPackages = []PackageItem{item}
			completion := PackageInstallMsg{
				Package: item,
				Success: exec.result.Success,
				Message: exec.result.Message,
				Err:     err,
			}
			updated, _ := m.Update(completion)
			resultModel := updated.(Model)
			if len(resultModel.results) != 1 {
				t.Fatalf("completion results = %+v; want one package result", resultModel.results)
			}
			got := resultModel.results[0]
			if got.Success || !strings.Contains(got.Message, err.Error()) {
				t.Fatalf("package result = %+v; want visible diagnostic %q", got, err)
			}
			if got.Err == nil || got.Err.Error() != err.Error() {
				t.Fatalf("package result error = %v; want retained error %v", got.Err, err)
			}
		})
	}
}

func TestTask5LoadingChecksSkipInactiveSetupRows(t *testing.T) {
	publicSetup := config.SubEntry{Name: "active-setup", Check: map[string]string{"linux": "true"}, Run: map[string]string{"linux": "true"}}
	privateSetup := config.SubEntry{Name: "inactive-setup", Check: map[string]string{"linux": "true"}, Run: map[string]string{"linux": "true"}}
	private := workspaceConfig("shared", privateSetup)
	private.Applications[0].When = `{{ false }}`
	m, _, _ := task5NewReviewModel(t, workspaceConfig("shared", publicSetup), private)
	m.filterEnabled = false
	if err := m.initWorkspaceApplicationItems(); err != nil {
		t.Fatal(err)
	}
	for i := range m.Applications {
		for j := range m.Applications[i].SubItems {
			m.Applications[i].SubItems[j].State = StateLoading
		}
	}
	cmd, count := m.checkLoadingSubEntryStatesCmd()
	if count != 1 || cmd == nil {
		t.Fatalf("loading dispatcher scheduled %d checks; want only the active Public sibling", count)
	}
	result, ok := cmd().(stateCheckResultMsg)
	if !ok || result.ref.Repository != workspace.Public {
		t.Fatalf("scheduled check result = %#v; want active Public setup", result)
	}
}

func TestTask5ApplicationScopeDraftUsesRealNavigationAndSaveKeys(t *testing.T) {
	public := workspaceConfig("shared", config.SubEntry{Name: "public-entry", Backup: "./public", Targets: map[string]string{"linux": filepath.Join(t.TempDir(), "public-target")}})
	private := workspaceConfig("shared", config.SubEntry{Name: "private-entry", Backup: "./private", Targets: map[string]string{"linux": filepath.Join(t.TempDir(), "private-target")}})
	public.Applications[0].Description = "public description"
	public.Applications[0].When = `{{ eq .OS "linux" }}`
	private.Applications[0].Description = "private description"
	private.Applications[0].When = `{{ eq .OS "windows" }}`
	private.Applications[0].Package = &config.EntryPackage{Managers: map[string]config.ManagerValue{"apt": {PackageName: "private-pkg"}}}
	m, publicDir, privateDir := task5NewReviewModel(t, public, private)
	m.HostnameChoices = nil
	m.initApplicationForm(0)

	// Start on Name, navigate to Repository scope, remain on the nondefault
	// Private contribution, then use form navigation to reach editable fields.
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.applicationForm.FocusIndex != 4 {
		t.Fatalf("Up from Name focused %d; want Repository scope", m.applicationForm.FocusIndex)
	}
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.applicationForm.FocusIndex == 4 {
		t.Fatal("Down from Repository scope did not return to an editable field")
	}
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyDown}) // Description
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	task5UpdateText(m, " revised")
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})

	// Navigate through the package list with keys and change Private's native
	// apt package value; no direct focus/cursor assignments are used.
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyTab})
	aptIndex := -1
	for i, managerName := range displayPackageManagers {
		if managerName == "apt" {
			aptIndex = i
			break
		}
	}
	if aptIndex < 0 {
		t.Fatal("apt is missing from the package manager fields")
	}
	for i := 0; i < aptIndex; i++ {
		task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	for range len("private-pkg") {
		task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	task5UpdateText(m, "private-edited-pkg")
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.applicationForm.PackageManagers["apt"]; got != "private-edited-pkg" {
		t.Fatalf("keyboard-edited apt package = %q", got)
	}

	// Edit the Private when condition through the When field, then edit the
	// shared name and switch scopes. Its unsaved scoped draft must survive.
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyTab}) // When
	task5UpdateApplication(m, tea.KeyPressMsg{Code: 'e', Text: "e"})
	for range len(`{{ eq .OS "windows" }}`) {
		task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	}
	task5UpdateText(m, `{{ eq .OS "linux" }}`)
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyTab})  // scope
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyDown}) // wrap to Name
	task5UpdateApplication(m, tea.KeyPressMsg{Code: 'e', Text: "e"})
	task5UpdateText(m, "-renamed")
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyUp}) // scope
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.applicationForm.RepositoryRole != workspace.Public {
		t.Fatalf("scope did not switch to Public: %q", m.applicationForm.RepositoryRole)
	}
	if got := m.applicationForm.NameInput.Value(); got != "shared-renamed" {
		t.Fatalf("shared name was lost during scope switch: %q", got)
	}
	if got := m.applicationForm.DescriptionInput.Value(); got != "public description" {
		t.Fatalf("switch did not load Public description: %q", got)
	}
	if got := m.applicationForm.PackageManagers["apt"]; got != "" {
		t.Fatalf("Private package leaked into Public draft: %q", got)
	}
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyDown}) // Name
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyDown}) // Description
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	task5UpdateText(m, " public-draft")
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	for range 3 { // Packages, When, then Repository scope
		task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyTab})
	}
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if got := m.applicationForm.DescriptionInput.Value(); got != "private description revised" {
		t.Fatalf("Private description draft was lost after scope round trip: %q", got)
	}
	if got := m.applicationForm.WhenInput.Value(); got != `{{ eq .OS "linux" }}` {
		t.Fatalf("Private condition draft was lost after scope round trip: %q", got)
	}
	if got := m.applicationForm.PackageManagers["apt"]; got != "private-edited-pkg" {
		t.Fatalf("Private package draft was lost after scope round trip: %q", got)
	}
	if got := m.applicationForm.NameInput.Value(); got != "shared-renamed" {
		t.Fatalf("shared name changed on scope round trip: %q", got)
	}
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if got := m.applicationForm.DescriptionInput.Value(); got != "public description public-draft" {
		t.Fatalf("Public scope draft was lost after switching back: %q", got)
	}
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyRight})
	task5UpdateApplication(m, tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.applicationForm != nil {
		t.Fatalf("keyboard save failed: %s", m.applicationForm.Err)
	}
	publicReload, err := config.Load(filepath.Join(publicDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	privateReload, err := config.Load(filepath.Join(privateDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := publicReload.Applications[0]; got.Name != "shared-renamed" || got.Description != "public description" || got.When != `{{ eq .OS "linux" }}` || got.Package != nil {
		t.Fatalf("keyboard-saved Public contribution = %+v", got)
	}
	if got := privateReload.Applications[0]; got.Name != "shared-renamed" || got.Description != "private description revised" || got.When != `{{ eq .OS "linux" }}` || got.Package.Managers["apt"].PackageName != "private-edited-pkg" {
		t.Fatalf("scope save changed the Private contribution: %+v", got)
	}
}

func TestTask5NewPrivateApplicationAndPackageCanBeCreatedWithKeys(t *testing.T) {
	m, publicDir, privateDir := task5NewReviewModel(t, workspaceConfig("public"), workspaceConfig("private"))
	m.initApplicationForm(-1)
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyUp})
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyRight})
	if got := m.applicationForm.RepositoryRole; got != workspace.Private {
		t.Fatalf("new application scope after Right = %q; want Private", got)
	}
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyDown}) // Name
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	task5UpdateText(m, "private-new")
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyDown}) // Description
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyDown}) // Packages
	aptIndex := -1
	for i, managerName := range displayPackageManagers {
		if managerName == "apt" {
			aptIndex = i
			break
		}
	}
	for i := 0; i < aptIndex; i++ {
		task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	task5UpdateText(m, "private-package")
	task5UpdateApplication(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	task5UpdateApplication(m, tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.applicationForm != nil {
		t.Fatalf("Private application save failed: %s", m.applicationForm.Err)
	}
	publicReload, err := config.Load(filepath.Join(publicDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	privateReload, err := config.Load(filepath.Join(privateDir, "tidydots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(publicReload.Applications) != 1 || publicReload.Applications[0].Name != "shared" {
		t.Fatalf("new Private app leaked into Public: %+v", publicReload.Applications)
	}
	if len(privateReload.Applications) != 2 || privateReload.Applications[1].Name != "private-new" || privateReload.Applications[1].Package == nil || privateReload.Applications[1].Package.Managers["apt"].PackageName != "private-package" {
		t.Fatalf("Private application/package not persisted: %+v", privateReload.Applications)
	}
}

func TestTask5RenamedMoveUsesPlanRefsForSelectionAndFocus(t *testing.T) {
	for _, source := range []workspace.Role{workspace.Public, workspace.Private} {
		for _, outcome := range []string{"applied", "committed cleanup pending"} {
			t.Run(string(source)+"/"+outcome, func(t *testing.T) {
				destination := workspace.Private
				if source == workspace.Private {
					destination = workspace.Public
				}
				entry := config.SubEntry{
					Name: "old-name", Backup: "./backup", Files: []string{"f"},
					When: `{{ false }}`, Targets: map[string]string{"linux": filepath.Join(t.TempDir(), "target")},
				}
				activePublic := config.SubEntry{Name: "active-public", Check: map[string]string{"linux": "true"}, Run: map[string]string{"linux": "true"}}
				activePrivate := config.SubEntry{Name: "active-private", Check: map[string]string{"linux": "true"}, Run: map[string]string{"linux": "true"}}
				public, private := workspaceConfig("shared"), workspaceConfig("shared")
				public.Applications[0].Entries = []config.SubEntry{activePublic}
				private.Applications[0].Entries = []config.SubEntry{activePrivate}
				if source == workspace.Public {
					public.Applications[0].Entries = append(public.Applications[0].Entries, entry)
				} else {
					private.Applications[0].Entries = append(private.Applications[0].Entries, entry)
				}
				m, publicDir, privateDir := task5NewReviewModel(t, public, private)
				m.filterEnabled = false // keep the inactive owner row visible during the move
				sourceRepo, err := m.Workspace.Repository(source)
				if err != nil {
					t.Fatal(err)
				}
				backupDir := filepath.Join(sourceRepo.Root, "backup")
				if err := os.MkdirAll(backupDir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(backupDir, "f"), []byte("source"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := m.initWorkspaceApplicationItems(); err != nil {
					t.Fatal(err)
				}
				m.Applications[0].Expanded = true
				m.rebuildTable()
				oldItem := task5FindEntry(t, m, source, "old-name")
				if !oldItem.Filtered {
					t.Fatal("fixture should be a visible but inactive contribution")
				}
				m.toggleSubEntrySelection(0, oldItem.Index)
				if !m.isSubEntrySelected("shared", "old-name", string(source)) {
					t.Fatalf("test did not select source entry: %+v", m.selectedSubEntries)
				}
				m.initSubEntryForm(0, oldItem.Index)
				m.subEntryForm.NameInput.SetValue("renamed")
				m.subEntryForm.RepositoryRole = destination
				updated, _ := m.updateSubEntryForm(tea.KeyPressMsg{Code: 's', Text: "s"})
				model := updated.(Model)
				m = &model
				if m.pendingMove == nil || m.pendingMove.SourceRef.Name != "old-name" || m.pendingMove.DestinationRef.Name != "renamed" {
					formErr := "<nil form>"
					if m.subEntryForm != nil {
						formErr = m.subEntryForm.Err
					}
					t.Fatalf("move plan refs = %+v; screen=%v form error=%q", m.pendingMove, m.Screen, formErr)
				}
				if !m.isSubEntrySelected("shared", "old-name", string(source)) {
					t.Fatalf("selection missing before move completion: refs=%+v -> %+v map=%+v", m.pendingMove.SourceRef, m.pendingMove.DestinationRef, m.selectedSubEntries)
				}
				if outcome == "applied" {
					updated, cmd := m.updateSummary(tea.KeyPressMsg{Code: tea.KeyEnter})
					model := updated.(Model)
					if cmd == nil {
						t.Fatal("move confirmation did not queue execution")
					}
					updated, _ = model.Update(cmd())
					model = updated.(Model)
					m = &model
				} else {
					publicAfter, privateAfter := workspaceConfig("shared"), workspaceConfig("shared")
					publicAfter.Applications[0].Entries = []config.SubEntry{activePublic}
					privateAfter.Applications[0].Entries = []config.SubEntry{activePrivate}
					movedEntry := entry
					movedEntry.Name = "renamed"
					if destination == workspace.Public {
						publicAfter.Applications[0].Entries = append(publicAfter.Applications[0].Entries, movedEntry)
					} else {
						privateAfter.Applications[0].Entries = append(privateAfter.Applications[0].Entries, movedEntry)
					}
					writeWorkspaceConfig(t, publicDir, publicAfter)
					writeWorkspaceConfig(t, privateDir, privateAfter)
					updated, _ := m.Update(workspaceMoveCompleteMsg{err: &workspace.TransactionError{ID: "task5-rereview-committed", Outcome: workspace.OutcomeCommittedCleanupPending, Cause: os.ErrPermission}})
					model := updated.(Model)
					m = &model
				}
				if !m.isSubEntrySelected("shared", "renamed", string(destination)) {
					t.Fatalf("committed %s move lost resulting selection: %+v; err=%v; results=%+v; pending=%+v; items=%+v", outcome, m.selectedSubEntries, m.err, m.results, m.pendingMove, m.Applications)
				}
				if m.tableCursor < 0 || m.tableCursor >= len(m.tableRows) {
					t.Fatalf("focus cursor out of range after move: %d/%d", m.tableCursor, len(m.tableRows))
				}
				focused := m.tableRows[m.tableCursor]
				if focused.SubName != "renamed" || focused.Repository != string(destination) {
					t.Fatalf("focus after %s move = %+v; want resulting %s/renamed row", outcome, focused, destination)
				}
			})
		}
	}
}
