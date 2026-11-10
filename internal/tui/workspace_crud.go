package tui

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

const (
	dryRunDeleteMessage = "Would delete configuration (dry-run)"
	workspaceMoveResult = "Ownership move"
)

type workspaceMoveCompleteMsg struct{ err error }

func (m Model) lockWorkspaceMutation() (func(), error) {
	if m.Workspace == nil || m.DryRun {
		return func() {}, nil
	}
	roots := make([]string, 0, len(m.Workspace.Repositories))
	for _, repo := range m.Workspace.Repositories {
		roots = append(roots, repo.Root)
	}
	return workspace.MutationGuard(context.Background(), m.Workspace.JournalDir, roots...)
}

func (m Model) validateFreshWorkspace() error {
	if m.Workspace == nil {
		return nil
	}
	if err := m.Workspace.ValidateFresh(); err != nil {
		return fmt.Errorf("workspace configuration is stale; refresh before writing: %w", err)
	}
	return nil
}

func (m Model) validateSubEntrySnapshot(item SubEntryItem) error {
	if m.Workspace == nil {
		return nil
	}
	if item.Ref.App == "" || item.Ref.Name == "" || item.AppName != item.Ref.App {
		return fmt.Errorf("queued entry is stale; refresh and select it again")
	}
	repo, err := m.Workspace.Repository(item.Ref.Repository)
	if err != nil {
		return fmt.Errorf("queued entry is stale; refresh and select it again: %w", err)
	}
	for _, app := range repo.Config.Applications {
		if app.Name != item.Ref.App || app.When != item.AppWhen {
			continue
		}
		for _, entry := range app.Entries {
			if entry.Name == item.Ref.Name && reflect.DeepEqual(entry, item.SubEntry) {
				return nil
			}
		}
	}
	return fmt.Errorf("queued entry definition or owner is stale; refresh and select it again")
}

func (m Model) validatePackageSnapshot(item PackageItem, appWhen string) error {
	if m.Workspace == nil {
		return nil
	}
	if item.Repository == "" {
		return fmt.Errorf("queued package owner is stale; refresh and select it again")
	}
	repo, err := m.Workspace.Repository(item.Repository)
	if err != nil {
		return fmt.Errorf("queued package owner is stale; refresh and select it again: %w", err)
	}
	for _, app := range repo.Config.Applications {
		if app.Name == item.Name && app.When == appWhen && reflect.DeepEqual(app.Package, item.Package) {
			return nil
		}
	}
	return fmt.Errorf("queued package definition or owner is stale; refresh and select it again")
}

func (m Model) applyPendingMove() tea.Cmd {
	plan := m.pendingMove
	w := m.Workspace
	dryRun := m.DryRun
	return func() tea.Msg {
		return workspaceMoveCompleteMsg{err: w.ApplyMove(context.Background(), plan, dryRun)}
	}
}

func (m *Model) validateWorkspaceTargets() error {
	if m.workspaceInvalidated {
		return fmt.Errorf("workspace view is invalid after a committed change could not be reloaded; restart the TUI before editing")
	}
	if m.Workspace == nil {
		return nil
	}
	return m.Workspace.ValidateCrossRepositoryTargets(m.Platform)
}

func (m *Model) refreshWorkspace(focus *workspace.EntryRef, appName string) error {
	return m.refreshWorkspaceWithLoader(focus, appName, workspace.Load)
}

func (m *Model) refreshWorkspaceForInspection(focus *workspace.EntryRef, appName string) error {
	return m.refreshWorkspaceWithLoader(focus, appName, workspace.LoadForInspection)
}

func (m *Model) refreshWorkspaceWithLoader(focus *workspace.EntryRef, appName string, load func(string, string) (*workspace.Workspace, error)) error {
	expanded := make(map[string]bool, len(m.Applications))
	focusApp, focusSub, focusRepo := "", "", ""
	if m.tableCursor >= 0 && m.tableCursor < len(m.tableRows) {
		row := m.tableRows[m.tableCursor]
		focusApp, focusSub, focusRepo = row.AppName, row.SubName, row.Repository
	}
	for _, app := range m.Applications {
		expanded[app.Application.Name] = app.Expanded
	}
	type packageSnapshot struct {
		owner     workspace.Role
		when      string
		pkg       *config.EntryPackage
		installed *bool
		method    string
	}
	packageStates := make(map[string]packageSnapshot, len(m.Applications))
	for _, app := range m.Applications {
		packageStates[app.Application.Name] = packageSnapshot{owner: app.PackageOwner, when: app.PackageWhen, pkg: app.Application.Package, installed: app.PkgInstalled, method: app.PkgMethod}
	}
	public, err := m.Workspace.Repository(workspace.Public)
	if err != nil {
		return err
	}
	privateRoot := ""
	if private, privateErr := m.Workspace.Repository(workspace.Private); privateErr == nil {
		privateRoot = private.Root
	}
	loaded, err := load(public.Root, privateRoot)
	if err != nil {
		return err
	}
	newReaders := openWorkspaceReaders(loaded, m.Platform, m.DryRun)
	for _, mgr := range m.RepositoryManagers {
		m.readerTracker.retire(mgr)
	}
	m.RepositoryManagers = newReaders
	if mgr := newReaders[workspace.Public]; mgr != nil {
		m.Manager = mgr
	}
	*m.Workspace = *loaded
	if err := m.initWorkspaceApplicationItems(); err != nil {
		m.workspaceInvalidated = true
		return err
	}
	// Old-generation results are stale; their command-held inspector leases
	// release independently. Only newly dispatched checks count for this view.
	m.pendingStateChecks = 0
	m.workspaceInvalidated = false
	for i := range m.Applications {
		m.Applications[i].Expanded = expanded[m.Applications[i].Application.Name]
		if old, ok := packageStates[m.Applications[i].Application.Name]; ok && old.owner == m.Applications[i].PackageOwner && old.when == m.Applications[i].PackageWhen && reflect.DeepEqual(old.pkg, m.Applications[i].Application.Package) {
			m.Applications[i].PkgInstalled = old.installed
			m.Applications[i].PkgMethod = old.method
		}
		for j := range m.Applications[i].SubItems {
			m.Applications[i].SubItems[j].State = m.detectSubEntryState(&m.Applications[i].SubItems[j])
		}
	}
	m.rebuildTable()
	m.pruneStaleSelections()
	if focus != nil {
		for i := range m.Applications {
			for j := range m.Applications[i].SubItems {
				if m.Applications[i].SubItems[j].Ref == *focus {
					for row := range m.tableRows {
						if m.tableRows[row].AppName == m.Applications[i].Application.Name && m.tableRows[row].SubIndex == m.Applications[i].SubItems[j].Index {
							m.tableCursor = row
							break
						}
					}
					m.updateScrollOffset()
					return nil
				}
			}
		}
	}
	if appName != "" {
		focusApp = appName
	}
	for row := range m.tableRows {
		candidate := m.tableRows[row]
		if candidate.AppName == focusApp && candidate.SubName == focusSub && (focusRepo == "" || candidate.Repository == focusRepo) {
			m.tableCursor = row
			m.updateScrollOffset()
			return nil
		}
	}
	m.rebuildTable()
	return nil
}

func (m *Model) saveWorkspaceEntry(ref workspace.EntryRef, updated config.SubEntry, destination workspace.Role) error {
	if err := m.validateWorkspaceTargets(); err != nil {
		return fmt.Errorf("validating workspace targets: %w", err)
	}
	if destination != ref.Repository {
		// Keep the ownership move side-effect-free until confirmation. The plan
		// also carries the resulting ref when the form renames the entry.
		plan, err := m.Workspace.PlanMove(context.Background(), workspace.MoveRequest{Ref: ref, Destination: destination, Updated: updated}, m.Platform)
		if err != nil {
			return err
		}
		m.pendingMove = plan
		m.pendingMoveFocus = plan.SourceRef
		m.pendingMoveApp = ref.App
		m.Screen = ScreenSummary
		return nil
	}
	updatedRef := ref
	updatedRef.Name = updated.Name
	if err := m.Workspace.SaveEntry(context.Background(), ref, updated, m.DryRun); err != nil {
		if isCommittedTransactionError(err) {
			m.remapSelectedEntry(ref, updatedRef)
			if m.subEntryForm != nil {
				m.subEntryForm.EntryRef = &updatedRef
			}
		}
		return m.reconcileCommittedWorkspaceError(err, &updatedRef, ref.App)
	}
	if m.DryRun {
		m.results = []ResultItem{{Name: ref.App + "/" + ref.Name, Success: true, Message: "Would save entry (dry-run)"}}
		return nil
	}
	m.remapSelectedEntry(ref, updatedRef)
	return m.refreshWorkspace(&updatedRef, ref.App)
}

func (m *Model) reconcileCommittedWorkspaceError(err error, focus *workspace.EntryRef, appName string) error {
	formatted := transactionOutcomeError(err)
	var txErr *workspace.TransactionError
	if !errors.As(err, &txErr) || txErr.Outcome != workspace.OutcomeCommittedCleanupPending {
		return formatted
	}
	if reloadErr := m.refreshWorkspaceForInspection(focus, appName); reloadErr != nil {
		m.workspaceInvalidated = true
		m.WorkspaceGeneration++
		m.Applications = nil
		m.tableRows = nil
		m.selectedApps = make(map[string]bool)
		m.selectedSubEntries = make(map[subEntryKey]bool)
		m.multiSelectActive = false
		m.pendingMove = nil
		if m.applicationForm != nil {
			m.applicationForm.OriginalAppName = ""
			m.applicationForm.EditAppIdx = -1
		}
		if m.subEntryForm != nil {
			m.subEntryForm.EntryRef = nil
			m.subEntryForm.TargetAppIdx = -1
			m.subEntryForm.EditAppIdx = -1
			m.subEntryForm.EditSubIdx = -1
		}
		return fmt.Errorf("%w; committed workspace reload failed and cached identities were invalidated: %w", formatted, reloadErr)
	}
	return formatted
}

func isCommittedTransactionError(err error) bool {
	var txErr *workspace.TransactionError
	return errors.As(err, &txErr) && txErr.Outcome == workspace.OutcomeCommittedCleanupPending
}

func transactionOutcomeError(err error) error {
	var txErr *workspace.TransactionError
	if !errors.As(err, &txErr) {
		return err
	}
	switch txErr.Outcome {
	case workspace.OutcomeRolledBack:
		return fmt.Errorf("change was rolled back; no recovery is needed (transaction %s): %w", txErr.ID, err)
	case workspace.OutcomePending:
		return fmt.Errorf("change needs recovery; transaction %s: %w", txErr.ID, err)
	case workspace.OutcomeCommittedCleanupPending:
		return fmt.Errorf("change committed, but cleanup needs recovery; transaction %s: %w", txErr.ID, err)
	default:
		return err
	}
}

func (m *Model) saveWorkspaceApplication(role workspace.Role, originalName, name, description, when string, pkg *config.EntryPackage) error {
	if err := m.validateWorkspaceTargets(); err != nil {
		return fmt.Errorf("validating workspace targets: %w", err)
	}
	updated := config.Application{Name: name, Description: description, When: when, Package: pkg}
	if err := m.Workspace.SaveApplication(context.Background(), role, originalName, updated, m.DryRun); err != nil {
		if isCommittedTransactionError(err) && originalName != name {
			m.remapSelectedApplication(originalName, name)
			if m.applicationForm != nil {
				m.applicationForm.OriginalAppName = name
			}
		}
		return m.reconcileCommittedWorkspaceError(err, nil, name)
	}
	if !m.DryRun {
		if originalName != name {
			m.remapSelectedApplication(originalName, name)
		}
		return m.refreshWorkspace(nil, name)
	}
	m.results = []ResultItem{{Name: name, Success: true, Message: "Would edit application (dry-run)"}}
	return nil
}

func (m *Model) remapSelectedApplication(oldName, newName string) {
	if m.selectedApps[oldName] {
		delete(m.selectedApps, oldName)
		m.selectedApps[newName] = true
	}
	updated := make(map[subEntryKey]bool, len(m.selectedSubEntries))
	for key, selected := range m.selectedSubEntries {
		if key.app == oldName {
			key.app = newName
		}
		updated[key] = selected
	}
	m.selectedSubEntries = updated
}
