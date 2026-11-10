package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/workspace"
)

// StatusReport is the resolved status of the applications and entries that
// apply to the current platform.
type StatusReport struct {
	Actionable   bool                `json:"actionable"`
	ActionsOnly  bool                `json:"actions_only"`
	Counts       StatusCounts        `json:"counts"`
	Applications []StatusApplication `json:"applications"`
}

// StatusCounts contains total and actionable item counts.
type StatusCounts struct {
	Applications           int `json:"applications"`
	Entries                int `json:"entries"`
	Packages               int `json:"packages"`
	ActionableApplications int `json:"actionable_applications"`
	ActionableEntries      int `json:"actionable_entries"`
	ActionablePackages     int `json:"actionable_packages"`
}

// StatusApplication describes one application and its resolved package and
// entry states.
type StatusApplication struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Actionable  bool           `json:"actionable"`
	Package     *StatusPackage `json:"package"`
	Entries     []StatusEntry  `json:"entries"`
}

// StatusPackage describes the selected package installation method and state.
type StatusPackage struct {
	Repository string `json:"repository,omitempty"`
	Name       string `json:"name"`
	Method     string `json:"method"`
	Installed  *bool  `json:"installed"`
	Actionable bool   `json:"actionable"`
}

// StatusEntry describes a resolved configuration or setup entry.
type StatusEntry struct {
	Repository string `json:"repository,omitempty"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	State      string `json:"state"`
	Error      string `json:"error,omitempty"`
	Actionable bool   `json:"actionable"`
	Target     string `json:"target"`
	Backup     string `json:"backup"`
	Method     string `json:"method"`
}

// ComputeWorkspaceStatus resolves owner-specific checks from a merged
// workspace using read-only state snapshots.
func ComputeWorkspaceStatus(w *workspace.Workspace, plat *platform.Platform, actionsOnly bool) (StatusReport, error) {
	if w == nil {
		return StatusReport{}, fmt.Errorf("cannot compute status without a workspace")
	}
	if plat == nil {
		return StatusReport{}, fmt.Errorf("cannot compute status without a platform")
	}
	if err := w.ValidateCrossRepositoryTargets(plat); err != nil {
		return StatusReport{}, fmt.Errorf("validating workspace targets: %w", err)
	}
	managers := openWorkspaceReaders(w, plat, false)
	defer closeWorkspaceManagers(managers)
	m, err := newWorkspaceModel(w, plat, false, false, managers)
	if err != nil {
		return StatusReport{}, err
	}
	if err := m.resolveStatusChecks(); err != nil {
		return StatusReport{}, err
	}
	return buildStatusReport(m, actionsOnly), nil
}

// ComputeStatus resolves every package and entry check used by the TUI before
// building a report. When actionsOnly is true, application details are reduced
// with the same action filter used by the TUI.
func ComputeStatus(cfg *config.Config, plat *platform.Platform, mgr *manager.Manager, actionsOnly bool) (StatusReport, error) {
	if cfg == nil {
		return StatusReport{}, fmt.Errorf("cannot compute status without a configuration")
	}
	if plat == nil {
		return StatusReport{}, fmt.Errorf("cannot compute status without a platform")
	}
	if mgr == nil {
		return StatusReport{}, fmt.Errorf("cannot compute status without a manager")
	}

	m := NewModel(cfg, plat, false)
	m.Manager = mgr

	if err := m.resolveStatusChecks(); err != nil {
		return StatusReport{}, err
	}

	return buildStatusReport(m, actionsOnly), nil
}

// resolveStatusChecks runs the same per-item commands dispatched by Model.Init,
// applying each result immediately so the returned model is fully settled.
func (m *Model) resolveStatusChecks() error {
	for appIndex := range m.Applications {
		if m.Applications[appIndex].IsFiltered {
			continue
		}

		if appHasActivePackage(m.Applications[appIndex]) {
			var err error
			*m, err = applyStatusCheck(*m, m.packageStateCheckCmd(appIndex))
			if err != nil {
				return err
			}
		}

		for subIndex := range m.Applications[appIndex].SubItems {
			if m.Applications[appIndex].SubItems[subIndex].Filtered {
				continue
			}
			var err error
			*m, err = applyStatusCheck(*m, m.subEntryStateCheckCmd(appIndex, subIndex))
			if err != nil {
				return err
			}
		}
	}

	if m.pendingStateChecks != 0 {
		return fmt.Errorf("status checks did not settle: %d checks remain", m.pendingStateChecks)
	}

	return nil
}

func applyStatusCheck(m Model, cmd tea.Cmd) (Model, error) {
	if cmd == nil {
		return m, fmt.Errorf("status check returned no command")
	}

	msg := cmd()
	if msg == nil {
		return m, fmt.Errorf("status check returned no result")
	}

	updated, _ := m.Update(msg)
	result, ok := updated.(Model)
	if !ok {
		return m, fmt.Errorf("status check returned an unexpected model type %T", updated)
	}

	return result, nil
}

func buildStatusReport(m Model, actionsOnly bool) StatusReport {
	operational := make([]ApplicationItem, 0, len(m.Applications))
	for _, app := range m.Applications {
		if !app.IsFiltered {
			operational = append(operational, app)
		}
	}

	report := StatusReport{
		ActionsOnly:  actionsOnly,
		Applications: make([]StatusApplication, 0),
	}

	for _, app := range operational {
		report.Counts.Applications++
		appActionable := applicationActionable(app, false)
		if appActionable {
			report.Counts.ActionableApplications++
		}
		if appHasActivePackage(app) {
			report.Counts.Packages++
			if applicationPackageActionable(app) {
				report.Counts.ActionablePackages++
			}
		}
		for _, sub := range app.SubItems {
			if sub.Filtered {
				continue
			}
			report.Counts.Entries++
			if sub.State.Actionable() {
				report.Counts.ActionableEntries++
			}
		}
	}

	report.Actionable = report.Counts.ActionableApplications > 0
	apps := operational
	if actionsOnly {
		apps = filterActionableApplications(operational, false)
	}
	for _, app := range apps {
		report.Applications = append(report.Applications, buildStatusApplication(app, m.Platform.OS))
	}

	return report
}

func applicationActionable(app ApplicationItem, includeLoading bool) bool {
	if applicationPackageActionable(app) || includeLoading && appHasActivePackage(app) && app.PkgInstalled == nil {
		return true
	}
	for _, sub := range app.SubItems {
		if sub.Filtered {
			continue
		}
		if subEntryActionable(sub.State, includeLoading) {
			return true
		}
	}
	return false
}

func buildStatusApplication(app ApplicationItem, osType string) StatusApplication {
	result := StatusApplication{
		Name:        app.Application.Name,
		Description: app.Application.Description,
		Status:      getApplicationStatus(app),
		Actionable:  applicationActionable(app, false),
		Entries:     make([]StatusEntry, 0, len(app.SubItems)),
	}
	if appHasActivePackage(app) {
		result.Package = buildStatusPackage(app)
	}

	for _, sub := range app.SubItems {
		if sub.Filtered {
			continue
		}
		kind := "config"
		method := sub.SubEntry.EffectiveMethod()
		if sub.SubEntry.IsSetup() {
			kind = TypeSetup
			method = ""
		}
		result.Entries = append(result.Entries, StatusEntry{
			Repository: string(sub.Ref.Repository),
			Name:       sub.SubEntry.Name,
			Kind:       kind,
			State:      sub.State.String(),
			Error:      sub.CheckError,
			Actionable: sub.State.Actionable(),
			Target:     firstNonEmpty(sub.Target, sub.SubEntry.GetTarget(osType)),
			Backup:     firstNonEmpty(sub.BackupPath, sub.SubEntry.Backup),
			Method:     method,
		})
	}

	return result
}

func firstNonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func buildStatusPackage(app ApplicationItem) *StatusPackage {
	installed := app.PkgInstalled
	return &StatusPackage{
		Repository: string(app.PackageOwner),
		Name:       packageName(app.Application.Package, app.PkgMethod, app.Application.Name),
		Method:     app.PkgMethod,
		Installed:  installed,
		Actionable: applicationPackageActionable(app),
	}
}

func packageName(pkg *config.EntryPackage, method, appName string) string {
	if value, ok := pkg.Managers[method]; ok {
		if value.IsGit() {
			return value.Git.URL
		}
		if value.IsInstaller() {
			return value.Installer.Binary
		}
		return value.PackageName
	}

	if method == "custom" || method == "url" {
		return appName
	}

	if method == TypeNone {
		return ""
	}

	return ""
}
