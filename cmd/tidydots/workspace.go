package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/AntoineGS/tidydots/internal/packages"
	"github.com/AntoineGS/tidydots/internal/platform"
	tmpl "github.com/AntoineGS/tidydots/internal/template"
	"github.com/AntoineGS/tidydots/internal/workspace"
	"github.com/spf13/cobra"
)

// resolveCLIWorkspace returns a workspace only when workspace selection was
// requested or a registered private repository is enabled. A bare --dir
// remains an isolated single-repository override.
func resolveCLIWorkspace(cmd *cobra.Command) (*workspace.Workspace, *platform.Platform, bool, error) {
	metadata, metadataErr := config.LoadAppConfigMetadata()
	publicRoot, err := cliPublicRoot(metadata, metadataErr)
	if err != nil {
		return nil, nil, false, err
	}
	pending, err := pendingTransactionsForCLICommand(cmd, publicRoot, metadata)
	if err != nil {
		return nil, nil, false, err
	}
	if configDir != "" && privateDir == "" && !publicOnly {
		return nil, nil, false, nil
	}
	selectedPrivate := selectCLIPrivateRoot(metadata)
	if !workspaceRoutingRequested(selectedPrivate) {
		return nil, nil, false, nil
	}
	loaded, err := loadSelectedCLIWorkspace(publicRoot, selectedPrivate, pending)
	if err != nil {
		if selectedPrivate != "" {
			return nil, nil, false, fmt.Errorf("loading enabled private repository: %w; use --public-only to continue with only the primary repository", err)
		}
		return nil, nil, false, err
	}
	plat, err := detectCLIPlatform()
	if err != nil {
		return nil, nil, false, err
	}
	return loaded, plat, true, nil
}

func pendingTransactionsForCLICommand(cmd *cobra.Command, publicRoot string, metadata *config.AppConfig) ([]workspace.TransactionInfo, error) {
	roots := []string{publicRoot}
	if metadata != nil {
		roots = append(roots, metadata.ConfigDir, metadata.PrivateConfigDir)
	}
	if privateDir != "" {
		roots = append(roots, config.ExpandPath(privateDir, nil))
	}
	pending, err := discoverCLIPendingTransactions(roots...)
	if err != nil {
		return nil, err
	}
	if len(pending) > 0 && !allowsPendingInspection(cmd) {
		return nil, pendingTransactionError(pending[0])
	}
	return pending, nil
}

func loadSelectedCLIWorkspace(publicRoot, privateRoot string, pending []workspace.TransactionInfo) (*workspace.Workspace, error) {
	selectedPending, err := pendingTransactionsForRoots(pending, publicRoot, privateRoot)
	if err != nil {
		return nil, err
	}
	if len(selectedPending) > 0 {
		return workspace.LoadForInspection(publicRoot, privateRoot)
	}
	return workspace.Load(publicRoot, privateRoot)
}

func cliPublicRoot(metadata *config.AppConfig, metadataErr error) (string, error) {
	if configDir != "" {
		return config.ExpandPath(configDir, nil), nil
	}
	if metadataErr != nil {
		return "", metadataErr
	}
	return metadata.ConfigDir, nil
}

func selectCLIPrivateRoot(metadata *config.AppConfig) string {
	if publicOnly {
		return ""
	}
	if privateDir != "" {
		return config.ExpandPath(privateDir, nil)
	}
	if configDir == "" && metadata != nil {
		return metadata.PrivateConfigDir
	}
	return ""
}

func workspaceRoutingRequested(selectedPrivate string) bool {
	return privateDir != "" || publicOnly || selectedPrivate != ""
}

func checkPendingCLITransactions(roots ...string) error {
	pending, err := discoverCLIPendingTransactions(roots...)
	if err != nil {
		return fmt.Errorf("checking pending workspace transactions: %w", err)
	}
	if len(pending) > 0 {
		return pendingTransactionError(pending[0])
	}
	return nil
}

func discoverCLIPendingTransactions(roots ...string) ([]workspace.TransactionInfo, error) {
	requestedRoots := make(map[string]struct{}, len(roots)*2)
	for _, root := range roots {
		if root == "" {
			continue
		}
		absolute, err := filepath.Abs(config.ExpandPath(root, nil))
		if err != nil {
			return nil, fmt.Errorf("resolving repository path for pending transaction check: %w", err)
		}
		requestedRoots[filepath.Clean(absolute)] = struct{}{}
		if canonical, err := filepath.EvalSymlinks(absolute); err == nil {
			requestedRoots[filepath.Clean(canonical)] = struct{}{}
		}
	}
	allPending, err := workspace.PendingTransactions(context.Background(), "")
	if err != nil {
		return nil, fmt.Errorf("checking pending workspace transactions: %w", err)
	}
	if len(requestedRoots) == 0 {
		return allPending, nil
	}
	var matches []workspace.TransactionInfo
	for _, transaction := range allPending {
		if transactionTouchesAnyRoot(transaction, requestedRoots) {
			matches = append(matches, transaction)
		}
	}
	return matches, nil
}

func transactionTouchesAnyRoot(transaction workspace.TransactionInfo, roots map[string]struct{}) bool {
	for _, root := range transaction.Roots {
		absolute, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if _, exists := roots[filepath.Clean(absolute)]; exists {
			return true
		}
	}
	return false
}

func pendingTransactionsForRoots(pending []workspace.TransactionInfo, roots ...string) ([]workspace.TransactionInfo, error) {
	identities := make(map[string]struct{}, len(roots)*2)
	for _, root := range roots {
		if root == "" {
			continue
		}
		absolute, err := filepath.Abs(config.ExpandPath(root, nil))
		if err != nil {
			return nil, err
		}
		identities[filepath.Clean(absolute)] = struct{}{}
		if canonical, err := filepath.EvalSymlinks(absolute); err == nil {
			identities[filepath.Clean(canonical)] = struct{}{}
		}
	}
	var selected []workspace.TransactionInfo
	for _, transaction := range pending {
		if transactionTouchesAnyRoot(transaction, identities) {
			selected = append(selected, transaction)
		}
	}
	return selected, nil
}

func pendingTransactionError(transaction workspace.TransactionInfo) error {
	return fmt.Errorf("workspace transaction %s is pending (phase %s); recover it with `tidydots recover %s` before continuing", transaction.ID, transaction.Phase, transaction.ID)
}

func allowsPendingInspection(cmd *cobra.Command) bool {
	if cmd == nil || interactive {
		return false
	}
	switch cmd.Name() {
	case "list", "list-packages", "status":
		return true
	case "restore", "backup", "install", "preview":
		return dryRun
	default:
		return false
	}
}

func acquireSingleRepositoryMutationGuard() (func(), error) {
	root, err := getConfigDir()
	if err != nil {
		return nil, err
	}
	lockedRoot, err := canonicalDirectory(root)
	if err != nil {
		return nil, fmt.Errorf("resolving repository root for mutation guard: %w", err)
	}
	release, err := workspace.MutationGuard(context.Background(), "", lockedRoot)
	if err != nil {
		return nil, fmt.Errorf("guarding repository mutation: %w", err)
	}
	currentRoot, err := getConfigDir()
	if err != nil {
		release()
		return nil, err
	}
	currentCanonical, err := canonicalDirectory(currentRoot)
	if err != nil {
		release()
		return nil, fmt.Errorf("resolving repository root after mutation guard: %w", err)
	}
	if currentCanonical != lockedRoot {
		release()
		return nil, fmt.Errorf("configured repository changed while acquiring its mutation lock; retry the command")
	}
	return release, nil
}

type workspaceManagerOwner struct {
	repo *workspace.Repository
	mgr  *manager.Manager
}

// reservedGitIgnoreProbes mirror the patterns installed by
// workspace.RegisterGitExcludes and validate them read-only during dry-run.
var reservedGitIgnoreProbes = []string{
	".tidydots-txn/probe",
	".tidydots-txn-config-probe",
	".tidydots.db",
	"probe.tmpl.rendered",
	"probe.tmpl.conflict",
}

func validateGitExcludes(root string) error {
	if _, err := os.Stat(filepath.Join(root, ".git")); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect Git metadata in %s: %w", root, err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("validate reserved Git excludes in %s: %w", root, err)
	}
	for _, path := range reservedGitIgnoreProbes {
		cmd := exec.CommandContext(context.Background(), git, "check-ignore", "--no-index", "-q", path)
		cmd.Dir = root
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git does not effectively ignore %q in %s; rerun init without --dry-run to install the reserved exclusions", path, root)
		}
	}
	return nil
}

func configureWorkspaceGitExcludes(w *workspace.Workspace, dryRun bool) error {
	for _, repo := range w.Repositories {
		if dryRun {
			if err := validateGitExcludes(repo.Root); err != nil {
				return fmt.Errorf("validating %s repository exclusions: %w", repo.Role, err)
			}
			continue
		}
		if err := workspace.RegisterGitExcludes(repo.Root); err != nil {
			return fmt.Errorf("registering %s repository exclusions: %w", repo.Role, err)
		}
	}
	return nil
}

func detectCLIPlatform() (*platform.Platform, error) {
	plat := platform.Detect()
	if osOverride == "" {
		return plat, nil
	}
	if osOverride != platform.OSLinux && osOverride != platform.OSWindows {
		return nil, fmt.Errorf("invalid OS override: %s (must be 'linux' or 'windows')", osOverride)
	}
	return plat.WithOS(osOverride), nil
}

func selectWorkspaceConfigs(w *workspace.Workspace, args []string, allowSetup bool, plat *platform.Platform) (map[workspace.Role]*config.Config, error) {
	appName := ""
	var selectedRef *workspace.EntryRef
	if len(args) > 0 {
		appName = args[0]
		app, err := findWorkspaceApplication(w, appName)
		if err != nil {
			return nil, err
		}
		if !workspaceApplicationActive(w, appName, plat) {
			return nil, fmt.Errorf("application %q does not match current conditions", appName)
		}
		if len(args) == 2 {
			ref, err := findWorkspaceEntry(app, args[1])
			if err != nil {
				return nil, err
			}
			if err := validateWorkspaceEntry(w, ref, args[1], appName, allowSetup, plat); err != nil {
				return nil, err
			}
			selectedRef = &ref
		}
	}
	return buildWorkspaceConfigs(w, appName, selectedRef, plat), nil
}

func findWorkspaceApplication(w *workspace.Workspace, name string) (workspace.Application, error) {
	apps, err := w.Applications()
	if err != nil {
		return workspace.Application{}, err
	}
	for _, app := range apps {
		if app.Name == name {
			return app, nil
		}
	}
	return workspace.Application{}, fmt.Errorf("application %q not found", name)
}

func workspaceApplicationActive(w *workspace.Workspace, name string, plat *platform.Platform) bool {
	engine := tmpl.NewEngine(tmpl.NewContextFromPlatform(plat))
	for _, repo := range w.Repositories {
		for _, app := range repo.Config.Applications {
			if app.Name == name && config.EvaluateWhen(app.When, engine) {
				return true
			}
		}
	}
	return false
}

func findWorkspaceEntry(app workspace.Application, name string) (workspace.EntryRef, error) {
	for _, entry := range app.Entries {
		if entry.Ref.Name == name {
			return entry.Ref, nil
		}
	}
	return workspace.EntryRef{}, fmt.Errorf("entry %q not found in application %q", name, app.Name)
}

func validateWorkspaceEntry(w *workspace.Workspace, ref workspace.EntryRef, entryName, appName string, allowSetup bool, plat *platform.Platform) error {
	_, app, entry, err := w.Lookup(ref)
	if err != nil {
		return err
	}
	engine := tmpl.NewEngine(tmpl.NewContextFromPlatform(plat))
	if !config.EvaluateWhen(app.When, engine) {
		return fmt.Errorf("application %q does not match current conditions", appName)
	}
	if !config.EvaluateWhen(entry.When, engine) {
		return fmt.Errorf("entry %q in application %q does not match current conditions", entryName, appName)
	}
	if entry.IsSetup() && !allowSetup {
		return fmt.Errorf("entry %q in application %q is a setup entry and is not supported by this command", entryName, appName)
	}
	return nil
}

func buildWorkspaceConfigs(w *workspace.Workspace, appName string, selectedRef *workspace.EntryRef, plat *platform.Platform) map[workspace.Role]*config.Config {
	configs := make(map[workspace.Role]*config.Config, len(w.Repositories))
	engine := tmpl.NewEngine(tmpl.NewContextFromPlatform(plat))
	for _, repo := range w.Repositories {
		selectedConfig := *repo.Config
		selectedConfig.BackupRoot = repo.Root
		selectedConfig.Applications = make([]config.Application, 0, len(repo.Config.Applications))
		for _, sourceApp := range repo.Config.Applications {
			if appName != "" && sourceApp.Name != appName {
				continue
			}
			if selectedRef != nil && repo.Role != selectedRef.Repository {
				continue
			}
			if appName != "" && !config.EvaluateWhen(sourceApp.When, engine) {
				continue
			}
			appCopy := sourceApp
			if selectedRef != nil {
				appCopy.Entries = nil
				for _, entry := range sourceApp.Entries {
					if entry.Name == selectedRef.Name {
						appCopy.Entries = append(appCopy.Entries, entry)
						break
					}
				}
			}
			selectedConfig.Applications = append(selectedConfig.Applications, appCopy)
		}
		configs[repo.Role] = &selectedConfig
	}
	return configs
}

func newWorkspaceManager(repo *workspace.Repository, cfg *config.Config, plat *platform.Platform, readOnly bool) (*manager.Manager, error) {
	cfg.BackupRoot = repo.Root
	mgr := manager.New(cfg, plat)
	mgr.DryRun = dryRun
	mgr.Verbose = verbose
	mgr.NoMerge = noMerge
	mgr.ForceDelete = forceDelete
	mgr.ForceRender = forceRender
	var err error
	if readOnly {
		err = mgr.InitReadOnlyStateStore()
	} else {
		err = mgr.InitStateStore()
	}
	if err != nil {
		return nil, fmt.Errorf("initializing %s repository state: %w", repo.Role, err)
	}
	return mgr, nil
}

func runWorkspaceRestore(w *workspace.Workspace, plat *platform.Platform, args []string) error {
	return runWorkspaceFileOperation(w, plat, args, true, false)
}

func runWorkspaceBackup(w *workspace.Workspace, plat *platform.Platform, args []string) error {
	return runWorkspaceFileOperation(w, plat, args, false, true)
}

func runWorkspaceFileOperation(w *workspace.Workspace, plat *platform.Platform, args []string, allowSetup, backup bool) error {
	configs, err := selectWorkspaceConfigs(w, args, allowSetup, plat)
	if err != nil {
		return err
	}
	if err := validateSelectedWorkspaceTargets(w, configs, plat); err != nil {
		return err
	}
	if !dryRun {
		roots := make([]string, 0, len(w.Repositories))
		for _, repo := range w.Repositories {
			roots = append(roots, repo.Root)
		}
		release, err := workspace.MutationGuard(context.Background(), w.JournalDir, roots...)
		if err != nil {
			return fmt.Errorf("guarding workspace mutation: %w", err)
		}
		defer release()
		if err := w.ValidateFresh(); err != nil {
			return fmt.Errorf("workspace configuration is stale; reload and retry: %w", err)
		}
	}
	managers := make([]workspaceManagerOwner, 0, len(w.Repositories))
	for _, repo := range w.Repositories {
		if len(configs[repo.Role].Applications) == 0 {
			continue
		}
		mgr, err := newWorkspaceManager(repo, configs[repo.Role], plat, dryRun)
		if err != nil {
			closeWorkspaceOwnerManagers(managers)
			return err
		}
		managers = append(managers, workspaceManagerOwner{repo: repo, mgr: mgr})
	}
	defer closeWorkspaceOwnerManagers(managers)
	if dryRun {
		fmt.Println("=== DRY RUN MODE ===")
	}
	return runWithCancellation(func(ctx context.Context) error {
		var errs []error
		for _, owner := range managers {
			if backup {
				err = owner.mgr.BackupWithContext(ctx)
			} else {
				err = owner.mgr.RestoreWithContext(ctx)
			}
			if err != nil {
				verb := "restoring"
				if backup {
					verb = "backing up"
				}
				errs = append(errs, fmt.Errorf("%s %s repository: %w", verb, owner.repo.Role, err))
			}
		}
		return errors.Join(errs...)
	})
}

// validateSelectedWorkspaceTargets checks writes selected for this operation
// against every ownership claim in the other repository. This permits unrelated
// conflicting definitions while retaining protection for inactive private
// paths that overlap selected public folder operations.
func validateSelectedWorkspaceTargets(w *workspace.Workspace, selected map[workspace.Role]*config.Config, plat *platform.Platform) error {
	for _, selectedRepo := range w.Repositories {
		selectedConfig := selected[selectedRepo.Role]
		if selectedConfig == nil || len(selectedConfig.Applications) == 0 {
			continue
		}
		candidate := &workspace.Workspace{JournalDir: w.JournalDir}
		for _, repo := range w.Repositories {
			clonedRepo := *repo
			if repo.Role == selectedRepo.Role {
				clonedRepo.Config = selectedConfig
			}
			candidate.Repositories = append(candidate.Repositories, &clonedRepo)
		}
		if err := candidate.ValidateTargets(plat); err != nil {
			return err
		}
	}
	return nil
}

func runWorkspaceList(w *workspace.Workspace, plat *platform.Platform, args []string) error {
	configs, err := selectWorkspaceConfigs(w, args, false, plat)
	if err != nil {
		return err
	}
	for _, repo := range w.Repositories {
		if len(configs[repo.Role].Applications) == 0 {
			continue
		}
		mgr, err := newWorkspaceManager(repo, configs[repo.Role], plat, true)
		if err != nil {
			return err
		}
		fmt.Printf("[%s repository]\n", repo.Role)
		err = mgr.List()
		_ = mgr.Close()
		if err != nil {
			return fmt.Errorf("listing %s repository: %w", repo.Role, err)
		}
	}
	return nil
}

func closeWorkspaceOwnerManagers(owners []workspaceManagerOwner) {
	for _, owner := range owners {
		_ = owner.mgr.Close()
	}
}

func packageConfigForRepository(repo *workspace.Repository, plat *platform.Platform) (*packages.Config, []packages.Package) {
	engine := tmpl.NewEngine(tmpl.NewContextFromPlatform(plat))
	applications := repo.Config.GetFilteredPackages(engine)
	packageList := packages.FromApplications(applications)
	return &packages.Config{
		Packages:        packageList,
		DefaultManager:  packages.PackageManager(repo.Config.DefaultManager),
		ManagerPriority: convertToPackageManagers(repo.Config.ManagerPriority),
	}, packageList
}

func runWorkspaceInstall(w *workspace.Workspace, plat *platform.Platform, args []string) error {
	type packageOwner struct {
		repo    *workspace.Repository
		manager *packages.Manager
		install []packages.Package
	}
	owners := make([]packageOwner, 0, len(w.Repositories))
	configured := 0
	for _, repo := range w.Repositories {
		cfg, list := packageConfigForRepository(repo, plat)
		if len(list) == 0 {
			continue
		}
		configured += len(list)
		pkgMgr := packages.NewManager(cfg, plat.OS, dryRun, verbose)
		install := pkgMgr.GetInstallablePackages()
		if len(args) > 0 {
			install = filterPackages(install, args)
		}
		owners = append(owners, packageOwner{repo: repo, manager: pkgMgr, install: install})
	}
	if configured == 0 {
		return fmt.Errorf("no matching packages configured in tidydots.yaml")
	}
	if dryRun {
		fmt.Println("=== DRY RUN MODE ===")
	}
	if !dryRun {
		roots := make([]string, 0, len(w.Repositories))
		for _, repo := range w.Repositories {
			roots = append(roots, repo.Root)
		}
		release, err := workspace.MutationGuard(context.Background(), w.JournalDir, roots...)
		if err != nil {
			return fmt.Errorf("guarding workspace package installation: %w", err)
		}
		defer release()
		if err := w.ValidateFresh(); err != nil {
			return fmt.Errorf("workspace configuration is stale; reload and retry: %w", err)
		}
	}
	var errs []error
	successCount, failCount := 0, 0
	for _, owner := range owners {
		fmt.Printf("Available package managers (%s): %v\n", owner.repo.Role, owner.manager.Available)
		if owner.manager.Preferred != "" {
			fmt.Printf("Preferred package manager (%s): %s\n", owner.repo.Role, owner.manager.Preferred)
		}
		for _, result := range owner.manager.InstallAll(owner.install) {
			status := "[ok]"
			if !result.Success {
				status = "[error]"
				failCount++
				errs = append(errs, fmt.Errorf("%s repository package %s: %s", owner.repo.Role, result.Package, result.Message))
			} else {
				successCount++
			}
			fmt.Printf("%s %s (%s): %s\n", status, result.Package, owner.repo.Role, result.Message)
		}
	}
	fmt.Printf("\nInstallation complete: %d successful, %d failed\n", successCount, failCount)
	return errors.Join(errs...)
}

func filterPackages(packagesToFilter []packages.Package, names []string) []packages.Package {
	filtered := make([]packages.Package, 0, len(packagesToFilter))
	for _, pkg := range packagesToFilter {
		for _, name := range names {
			if pkg.Name == name {
				filtered = append(filtered, pkg)
				break
			}
		}
	}
	return filtered
}

func runWorkspaceListPackages(w *workspace.Workspace, plat *platform.Platform) error {
	hasPackages := false
	for _, repo := range w.Repositories {
		cfg, list := packageConfigForRepository(repo, plat)
		if len(list) == 0 {
			continue
		}
		hasPackages = true
		pkgMgr := packages.NewManager(cfg, plat.OS, false, verbose)
		fmt.Printf("Available package managers (%s): %v\n\n", repo.Role, pkgMgr.Available)
		for _, pkg := range pkgMgr.Config.Packages {
			method := pkgMgr.GetInstallMethod(pkg)
			canInstall := pkgMgr.CanInstall(pkg)
			status := "✓"
			if !canInstall {
				status = "✗"
				method = "unavailable"
			}
			fmt.Printf("%s %s (%s; %s)\n", status, pkg.Name, method, repo.Role)
			if pkg.Description != "" {
				fmt.Printf("    %s\n", pkg.Description)
			}
		}
	}
	if !hasPackages {
		fmt.Println("No matching packages configured in tidydots.yaml")
	}
	return nil
}

func isWorkspacePathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolveWorkspacePath(path string) (string, error) {
	absolute, err := filepath.Abs(config.ExpandPath(path, nil))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func workspacePreviewRoot(w *workspace.Workspace, requestedPath string) (*workspace.Repository, string, error) {
	resolved, err := resolveWorkspacePath(requestedPath)
	if err != nil {
		return nil, "", fmt.Errorf("resolving preview path: %w", err)
	}
	var owner *workspace.Repository
	for _, repo := range w.Repositories {
		if isWorkspacePathWithin(repo.Root, resolved) {
			if owner != nil {
				return nil, "", fmt.Errorf("preview path %q is ambiguous between repositories", requestedPath)
			}
			owner = repo
		}
	}
	if owner == nil {
		return nil, "", fmt.Errorf("preview path %q is outside the enabled repositories", requestedPath)
	}
	return owner, resolved, nil
}
