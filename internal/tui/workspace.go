package tui

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

// RunWorkspace starts the merged workspace TUI. Each repository retains its
// own source configuration, manager, and state database; the merged application
// items are read-only projections and are never passed to config.Save.
func RunWorkspace(w *workspace.Workspace, plat *platform.Platform, dryRun, actionFilterEnabled bool) error {
	if w == nil {
		return fmt.Errorf("cannot run TUI without a workspace")
	}
	if plat == nil {
		return fmt.Errorf("cannot run TUI without a platform")
	}
	if err := w.ValidateCrossRepositoryTargets(plat); err != nil {
		return fmt.Errorf("validating workspace targets: %w", err)
	}
	readers := openWorkspaceReaders(w, plat, dryRun)

	model, err := newWorkspaceModel(w, plat, dryRun, actionFilterEnabled, readers)
	if err != nil {
		closeWorkspaceManagers(readers)
		return err
	}
	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		closeAllWorkspaceReaders(readers, model.readerTracker)
		return fmt.Errorf("TUI error: %w", err)
	}
	m, ok := finalModel.(Model)
	if !ok {
		closeAllWorkspaceReaders(readers, model.readerTracker)
		return fmt.Errorf("unexpected model type")
	}
	closeWorkspaceManagers(m.WriteManagers)
	closeAllWorkspaceReaders(m.RepositoryManagers, m.readerTracker)
	if m.Screen == ScreenResults && m.Operation != OpList && len(m.results) > 0 {
		printFinalSummary(m)
	}
	return nil
}

func newWorkspaceModel(
	w *workspace.Workspace,
	plat *platform.Platform,
	dryRun, actionFilterEnabled bool,
	readers map[workspace.Role]*manager.Manager,
) (Model, error) {
	if w == nil {
		return Model{}, fmt.Errorf("cannot build TUI model without a workspace")
	}
	public, err := w.Repository(workspace.Public)
	if err != nil {
		return Model{}, err
	}
	m := NewModelWithActionFilter(public.Config, plat, dryRun, actionFilterEnabled)
	m.Workspace = w
	m.RepositoryManagers = readers
	m.readerTracker = newWorkspaceReaderTracker()
	m.WriteManagers = make(map[workspace.Role]*manager.Manager)
	m.Manager = readers[workspace.Public]
	m.ConfigPath = public.ConfigPath
	m.HostnameChoices = workspaceHostnames(w)
	m.initApplicationItems()
	m.pendingStateChecks = m.countInitialStateChecks()
	if actionFilterEnabled {
		m.rebuildTable()
	}
	return m, nil
}

func workspaceHostnames(w *workspace.Workspace) []string {
	var hostnames []string
	seen := make(map[string]bool)
	for _, repo := range w.Repositories {
		for _, hostname := range repo.Config.Hostnames {
			if hostname != "" && !seen[hostname] {
				seen[hostname] = true
				hostnames = append(hostnames, hostname)
			}
		}
	}
	if len(hostnames) == 0 {
		if local, err := config.LoadAppConfigMetadata(); err == nil {
			return append([]string(nil), local.Hostnames...)
		}
	}
	return hostnames
}

func openWorkspaceReaders(w *workspace.Workspace, plat *platform.Platform, dryRun bool) map[workspace.Role]*manager.Manager {
	readers := make(map[workspace.Role]*manager.Manager, len(w.Repositories))
	for _, repo := range w.Repositories {
		cfg := repositoryManagerConfig(repo)
		mgr := manager.New(cfg, plat)
		mgr.DryRun = dryRun
		if err := mgr.InitReadOnlyStateStore(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not initialize %s state store for inspection: %v\n", repo.Role, err)
		}
		readers[repo.Role] = mgr
	}
	return readers
}

func repositoryManagerConfig(repo *workspace.Repository) *config.Config {
	if repo == nil || repo.Config == nil {
		return nil
	}
	clone := *repo.Config
	clone.BackupRoot = repo.Root
	return &clone
}

func closeWorkspaceManagers(managers map[workspace.Role]*manager.Manager) {
	for _, mgr := range managers {
		if mgr != nil {
			_ = mgr.Close()
		}
	}
}

type workspaceReaderTracker struct {
	mu      sync.Mutex
	leases  map[*manager.Manager]int
	retired map[*manager.Manager]bool
}

func newWorkspaceReaderTracker() *workspaceReaderTracker {
	return &workspaceReaderTracker{leases: make(map[*manager.Manager]int), retired: make(map[*manager.Manager]bool)}
}

func (t *workspaceReaderTracker) acquire(mgr *manager.Manager) func() {
	if t == nil || mgr == nil {
		return func() {}
	}
	t.mu.Lock()
	t.leases[mgr]++
	t.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.leases[mgr]--
			if t.leases[mgr] == 0 {
				delete(t.leases, mgr)
				if t.retired[mgr] {
					delete(t.retired, mgr)
					_ = mgr.Close()
				}
			}
		})
	}
}

func (t *workspaceReaderTracker) retire(mgr *manager.Manager) {
	if mgr == nil {
		return
	}
	if t == nil {
		_ = mgr.Close()
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.leases[mgr] == 0 {
		_ = mgr.Close()
		return
	}
	t.retired[mgr] = true
}

func closeAllWorkspaceReaders(current map[workspace.Role]*manager.Manager, tracker *workspaceReaderTracker) {
	seen := make(map[*manager.Manager]bool)
	for _, mgr := range current {
		if mgr != nil && !seen[mgr] {
			seen[mgr] = true
			tracker.retire(mgr)
		}
	}
}

func (m Model) managerForRole(role workspace.Role) *manager.Manager {
	if mgr := m.RepositoryManagers[role]; mgr != nil {
		return mgr
	}
	if m.Workspace == nil {
		return m.Manager
	}
	repo, err := m.Workspace.Repository(role)
	if err != nil {
		return nil
	}
	return manager.New(repositoryManagerConfig(repo), m.Platform)
}

func (m Model) configForRole(role workspace.Role) *config.Config {
	if m.Workspace == nil {
		return m.Config
	}
	repo, err := m.Workspace.Repository(role)
	if err != nil || repo == nil {
		return nil
	}
	return repo.Config
}

// writableManagerForRole opens a distinct writable manager for mutations. Any
// active inspector keeps its immutable read-only snapshot and is never promoted
// or closed underneath an asynchronous state reader.
func (m *Model) writableManagerForRole(role workspace.Role) (*manager.Manager, error) {
	if m.Workspace == nil {
		return m.pathManager(), nil
	}
	if m.DryRun {
		if mgr := m.managerForRole(role); mgr != nil {
			return mgr, nil
		}
		return nil, fmt.Errorf("%s repository is unavailable", role)
	}
	if mgr := m.WriteManagers[role]; mgr != nil {
		return mgr, nil
	}
	repo, err := m.Workspace.Repository(role)
	if err != nil {
		return nil, err
	}
	mgr := manager.New(repositoryManagerConfig(repo), m.Platform)
	mgr.DryRun = false
	if err := mgr.InitStateStore(); err != nil {
		return nil, fmt.Errorf("opening writable %s state store: %w", role, err)
	}
	if m.WriteManagers == nil {
		m.WriteManagers = make(map[workspace.Role]*manager.Manager)
	}
	m.WriteManagers[role] = mgr
	return mgr, nil
}

func managerForSubEntry(m Model, item SubEntryItem) *manager.Manager {
	if m.Workspace == nil || item.Ref.Repository == "" {
		return m.pathManager()
	}
	return m.managerForRole(item.Ref.Repository)
}

func (m *Model) writeManagerForSubEntry(item SubEntryItem) (*manager.Manager, error) {
	role := item.Ref.Repository
	if role == "" {
		role = workspace.Public
	}
	return m.writableManagerForRole(role)
}

func (m *Model) initWorkspaceApplicationItems() error {
	m.WorkspaceGeneration++
	apps, err := m.Workspace.Applications()
	if err != nil {
		return err
	}
	m.Applications = make([]ApplicationItem, 0, len(apps))
	for _, app := range apps {
		pkgWhen := ""
		if app.Package != nil {
			if repo, repoErr := m.Workspace.Repository(app.PackageOwner); repoErr == nil {
				for _, source := range repo.Config.Applications {
					if source.Name == app.Name {
						pkgWhen = source.When
						break
					}
				}
			}
		}
		packageFiltered := app.Package != nil && !config.EvaluateWhen(pkgWhen, m.Renderer)
		subItems := make([]SubEntryItem, 0, len(app.Entries))
		anyActive := !packageFiltered && app.Package != nil
		for _, entry := range app.Entries {
			active := config.EvaluateWhen(entry.AppWhen, m.Renderer) && config.EvaluateWhen(entry.Value.When, m.Renderer)
			if active {
				anyActive = true
			} else if m.filterEnabled {
				continue
			}
			if !subEntryAppliesToOS(entry.Value, m.Platform.OS) {
				continue
			}
			item := SubEntryItem{
				SubEntry:       entry.Value,
				AppName:        app.Name,
				Ref:            entry.Ref,
				AppWhen:        entry.AppWhen,
				Filtered:       !active,
				ShowRepository: true,
				Generation:     m.WorkspaceGeneration,
				Index:          len(subItems),
			}
			if target := entry.Value.GetTarget(m.Platform.OS); target != "" {
				if mgr := m.managerForRole(entry.Ref.Repository); mgr != nil {
					item.Target, item.BackupPath, err = resolveSubEntryPaths(item, mgr)
					if err != nil {
						item.CheckError = err.Error()
						item.State = StateUnavailable
					}
				}
			}
			subItems = append(subItems, item)
		}
		view := config.Application{Name: app.Name, Description: app.Description, Package: app.Package}
		for _, entry := range app.Entries {
			view.Entries = append(view.Entries, entry.Value)
		}
		m.Applications = append(m.Applications, ApplicationItem{
			Application:     view,
			PackageOwner:    app.PackageOwner,
			PackageWhen:     pkgWhen,
			PackageFiltered: packageFiltered,
			ShowRepository:  true,
			SubItems:        subItems,
			IsFiltered:      !anyActive,
		})
	}
	slices.SortFunc(m.Applications, func(a, b ApplicationItem) int {
		return strings.Compare(a.Application.Name, b.Application.Name)
	})
	m.initTableModel()
	return nil
}
