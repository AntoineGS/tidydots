// Package main provides the CLI entry point for tidydots.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"syscall"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/AntoineGS/tidydots/internal/packages"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/preview"
	tmpl "github.com/AntoineGS/tidydots/internal/template"
	"github.com/AntoineGS/tidydots/internal/tui"
	"github.com/AntoineGS/tidydots/internal/workspace"
	"github.com/spf13/cobra"
)

var version = "dev"

var (
	configDir      string // Override from --dir flag
	osOverride     string
	dryRun         bool
	verbose        bool
	interactive    bool
	actions        bool
	statusActions  bool
	statusJSON     bool
	noMerge        bool
	forceDelete    bool
	forceRender    bool
	privateDir     string
	publicOnly     bool
	initPrivateDir string
	removePrivate  bool
	cpuProfile     string
	logFile        *os.File
)

var interactiveIsTerminal = tui.IsTerminal

var runInteractiveTUI = func(
	cfg *config.Config,
	plat *platform.Platform,
	dryRun bool,
	configPath string,
	actionFilterEnabled bool,
) error {
	if actionFilterEnabled {
		return tui.RunWithActionFilter(cfg, plat, dryRun, configPath)
	}
	return tui.Run(cfg, plat, dryRun, configPath)
}

func main() {
	rootCmd := newRootCommand()

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:     "tidydots",
		Version: version,
		Short:   "Manage dotfiles and configurations across platforms",
		Args:    cobra.NoArgs,
		Long: `tidydots is a cross-platform tool for managing dotfiles and configurations.
It supports backup and restore operations using symlinks, with support for
both Windows and Linux systems.

Configuration is stored in two places:
  ~/.config/tidydots/config.yaml  - Points to your configurations repo
  <repo>/tidydots.yaml            - Defines paths to manage

Run 'tidydots init <path>' to set up the app configuration.
Run without arguments to start the interactive TUI.`,
		RunE: runInteractive,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateWorkspaceFlags(cmd); err != nil {
				return err
			}
			if actions && cmd.Name() != "tidydots" {
				return fmt.Errorf("--actions cannot be used with %s; use it with tidydots or status --actions", cmd.Name())
			}
			if verbose {
				logWriter := os.Stderr
				// When running interactively (TUI), write logs to a file to avoid corrupting the display
				if tui.IsTerminal() {
					logPath := filepath.Join(os.TempDir(), "tidydots.log")
					f, err := os.Create(logPath)
					if err == nil {
						logFile = f
						logWriter = f
						fmt.Fprintf(os.Stderr, "Verbose logs: %s\n", logPath)
					}
				}
				slog.SetDefault(slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{
					Level: slog.LevelDebug,
				})))
			}
			if cpuProfile != "" {
				f, err := os.Create(filepath.Clean(cpuProfile))
				if err != nil {
					return fmt.Errorf("creating CPU profile: %w", err)
				}
				if err := pprof.StartCPUProfile(f); err != nil {
					_ = f.Close()
					return fmt.Errorf("starting CPU profile: %w", err)
				}
			}
			return nil
		},
		PersistentPostRun: func(_ *cobra.Command, _ []string) {
			if logFile != nil {
				_ = logFile.Close()
			}
			if cpuProfile != "" {
				pprof.StopCPUProfile()
			}
		},
	}

	rootCmd.PersistentFlags().StringVarP(&configDir, "dir", "d", "", "Override repository directory, including TUI hostname choices")
	rootCmd.PersistentFlags().StringVar(&privateDir, "private-dir", "", "Pair a private repository for this invocation")
	rootCmd.PersistentFlags().BoolVar(&publicOnly, "public-only", false, "Use only the primary repository for this invocation")
	rootCmd.PersistentFlags().StringVarP(&osOverride, "os", "o", "", "Override OS detection (linux or windows)")
	rootCmd.PersistentFlags().BoolVarP(&dryRun, "dry-run", "n", false, "Show what would be done without making changes")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.Flags().BoolVar(&actions, "actions", false, "Start the interactive TUI with action filtering enabled")
	rootCmd.PersistentFlags().StringVar(&cpuProfile, "cpuprofile", "", "Write CPU profile to file (e.g. cpu.prof)")
	_ = rootCmd.PersistentFlags().MarkHidden("cpuprofile")

	initCmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize app configuration",
		Long: `Initialize the app configuration by setting the path to your configurations repository.

This creates ~/.config/tidydots/config.yaml with the path to your repo.
The repo should contain a tidydots.yaml file with your path definitions.

Use --private <path> to register an existing private repository, or
--remove-private to unregister it without deleting files. Both support --dry-run.`,
		Args: validateInitArgs,
		RunE: runInit,
	}
	initCmd.Flags().StringVar(&initPrivateDir, "private", "", "Register an existing private repository")
	initCmd.Flags().BoolVar(&removePrivate, "remove-private", false, "Unregister the private repository")

	restoreCmd := &cobra.Command{
		Use:   "restore [app [entry]]",
		Short: "Restore configurations by creating symlinks",
		Long:  `Restore configurations by creating symlinks from target locations to backup sources. Optionally target one application or one entry.`,
		Args:  validateTargetArgs,
		RunE:  runRestore,
	}
	restoreCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Run in interactive mode")
	restoreCmd.Flags().BoolVar(&noMerge, "no-merge", false, "Disable merge mode, return error if target exists")
	restoreCmd.Flags().BoolVar(&forceDelete, "force", false, "When combined with --no-merge, delete existing files without prompting")
	restoreCmd.Flags().BoolVar(&forceRender, "force-render", false, "Force re-render of templates, skipping 3-way merge")

	backupCmd := &cobra.Command{
		Use:   "backup [app [entry]]",
		Short: "Backup configurations from target locations",
		Long:  `Copy configuration files from target locations to backup directory. Optionally target one application or one config entry.`,
		Args:  validateTargetArgs,
		RunE:  runBackup,
	}
	backupCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Run in interactive mode")

	listCmd := &cobra.Command{
		Use:   "list [app [entry]]",
		Short: "List configured paths",
		Long:  `Display configured paths and their targets for the current OS. Optionally target one application or one config entry.`,
		Args:  validateTargetArgs,
		RunE:  runList,
	}

	installCmd := &cobra.Command{
		Use:   "install [package-names...]",
		Short: "Install packages using configured package managers",
		Long: `Install packages from your configuration using the appropriate package manager.
If no package names are provided, all matching packages will be installed.
Packages are filtered based on their filters (os, hostname, user).`,
		RunE: runInstall,
	}
	installCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Run in interactive mode")

	listPkgsCmd := &cobra.Command{
		Use:   "list-packages",
		Short: "List all configured packages",
		Long:  `Display all configured packages and their installation methods for the current OS.`,
		RunE:  runListPackages,
	}

	recoverCmd := &cobra.Command{
		Use:   "recover <transaction-id>",
		Short: "Recover an interrupted workspace transaction",
		Long:  "Roll back an interrupted workspace transaction or finish cleanup after its commit point.",
		Args:  cobra.ExactArgs(1),
		RunE:  runRecover,
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show resolved configuration and package status",
		Long:  "Resolve all package and entry checks and report their current status.",
		Args:  cobra.NoArgs,
		RunE:  runStatus,
	}
	statusCmd.Flags().BoolVar(&statusActions, "actions", false, "Show only applications and entries needing action")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output status as JSON")

	previewCmd := &cobra.Command{
		Use:   "preview <path>",
		Short: "Live preview template rendering",
		Long: `Watch .tmpl files for changes and render them to .tmpl.rendered in real time.

Open the .tmpl file and its .tmpl.rendered file side by side in your editor
for a live preview experience. The rendered file updates on each save.

The path can be a single .tmpl file or a directory containing .tmpl files.`,
		Args: cobra.ExactArgs(1),
		RunE: runPreview,
	}

	rootCmd.AddCommand(initCmd, restoreCmd, backupCmd, listCmd, installCmd, listPkgsCmd, recoverCmd, statusCmd, previewCmd)

	return rootCmd
}

func runRecover(cmd *cobra.Command, args []string) error {
	if err := workspace.Recover(cmd.Context(), "", args[0], dryRun); err != nil {
		return fmt.Errorf("recovering workspace transaction: %w", err)
	}
	if dryRun {
		fmt.Printf("Dry run: workspace transaction %s would be recovered\n", args[0])
		return nil
	}
	fmt.Printf("Workspace transaction %s recovered\n", args[0])
	return nil
}

func validateWorkspaceFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("private-dir") && privateDir == "" {
		return fmt.Errorf("--private-dir requires a path")
	}
	if privateDir != "" && publicOnly {
		return fmt.Errorf("--private-dir and --public-only are mutually exclusive")
	}
	return nil
}

func validateInitArgs(cmd *cobra.Command, args []string) error {
	privateChanged := cmd.Flags().Changed("private")
	removeChanged := cmd.Flags().Changed("remove-private")
	if privateChanged && removeChanged {
		return fmt.Errorf("--private and --remove-private are mutually exclusive")
	}
	if privateChanged {
		if initPrivateDir == "" {
			return fmt.Errorf("--private requires a path")
		}
		if len(args) != 0 {
			return fmt.Errorf("init --private does not accept a positional path")
		}
		return nil
	}
	if removeChanged {
		if !removePrivate {
			return fmt.Errorf("--remove-private cannot be set to false")
		}
		if len(args) != 0 {
			return fmt.Errorf("init --remove-private does not accept a positional path")
		}
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("init requires a path, --private <path>, or --remove-private")
	}
	return nil
}

func runInit(_ *cobra.Command, args []string) error {
	if initPrivateDir != "" || removePrivate {
		return runPrivateRegistration()
	}
	absPath, err := validateInitPath(args[0])
	if err != nil {
		return err
	}

	// Save app config
	appCfg, err := config.LoadAppConfigMetadata()
	if err != nil {
		appCfg = &config.AppConfig{}
	}
	oldConfiguredRoot := appCfg.ConfigDir
	oldPrivateRoot := appCfg.PrivateConfigDir
	oldPrimary, oldPrimaryErr := canonicalConfiguredDirectory(oldConfiguredRoot)
	newPrimary, err := canonicalDirectory(absPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}
	return savePrimaryRegistration(appCfg, oldConfiguredRoot, oldPrivateRoot, oldPrimary, oldPrimaryErr, newPrimary)
}

func validateInitPath(path string) (string, error) {
	path = config.ExpandPath(path, nil)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	info, err := os.Stat(absPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("directory does not exist %s: %w", absPath, err)
	}
	if err != nil {
		return "", fmt.Errorf("checking directory %s: %w", absPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", absPath)
	}
	if _, err := os.Stat(filepath.Join(absPath, "tidydots.yaml")); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Warning: %s not found in %s\n", "tidydots.yaml", absPath)
		fmt.Println("You'll need to create it before using tidydots.")
	}
	return absPath, nil
}

func savePrimaryRegistration(appCfg *config.AppConfig, oldConfiguredRoot, oldPrivateRoot, oldPrimary string, oldPrimaryErr error, newPrimary string) error {
	appCfg.ConfigDir = newPrimary
	pendingRoots := []string{newPrimary, oldConfiguredRoot, oldPrivateRoot}
	if err := checkPendingCLITransactions(pendingRoots...); err != nil {
		return err
	}
	if oldPrimaryErr != nil || oldPrimary != newPrimary {
		appCfg.PrivateConfigDir = ""
	}
	if dryRun {
		fmt.Printf("Dry run: app configuration would be saved to %s\n", config.AppConfigPath())
		fmt.Printf("Configurations directory: %s\n", newPrimary)
		return nil
	}
	release, err := acquireRegistrationMutationGuard(pendingRoots...)
	if err != nil {
		return err
	}
	defer release()
	if err := checkPendingCLITransactions(pendingRoots...); err != nil {
		return err
	}

	if err := config.SaveAppConfig(appCfg); err != nil {
		return fmt.Errorf("saving app config: %w", err)
	}

	fmt.Printf("App configuration saved to %s\n", config.AppConfigPath())
	fmt.Printf("Configurations directory: %s\n", newPrimary)

	return nil
}

func canonicalConfiguredDirectory(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	return canonicalDirectory(path)
}

func runPrivateRegistration() error {
	appCfg, err := config.LoadAppConfigMetadata()
	if err != nil {
		return fmt.Errorf("private repository registration requires an initialized app config: %w", err)
	}
	requestedRoots := []string{appCfg.ConfigDir, appCfg.PrivateConfigDir}
	if initPrivateDir != "" {
		requestedRoots = append(requestedRoots, config.ExpandPath(initPrivateDir, nil))
	}
	if err := checkPendingCLITransactions(requestedRoots...); err != nil {
		return err
	}
	if removePrivate {
		return removePrivateRegistration(appCfg, requestedRoots)
	}
	return registerPrivateRepository(appCfg)
}

func removePrivateRegistration(appCfg *config.AppConfig, roots []string) error {
	appCfg.PrivateConfigDir = ""
	if dryRun {
		fmt.Println("Dry run: private repository registration would be removed")
		return nil
	}
	release, err := acquireRegistrationMutationGuard(roots...)
	if err != nil {
		return err
	}
	defer release()
	if err := checkPendingCLITransactions(roots...); err != nil {
		return err
	}
	if err := config.SaveAppConfig(appCfg); err != nil {
		return fmt.Errorf("saving app config: %w", err)
	}
	fmt.Println("Private repository registration removed")
	return nil
}

func registerPrivateRepository(appCfg *config.AppConfig) error {
	var err error
	privatePath := config.ExpandPath(initPrivateDir, nil)
	privatePath, err = filepath.Abs(privatePath)
	if err != nil {
		return fmt.Errorf("resolving private repository path: %w", err)
	}
	loaded, err := workspace.Load(appCfg.ConfigDir, privatePath)
	if err != nil {
		return fmt.Errorf("validating private repository: %w", err)
	}
	var release func()
	if !dryRun {
		roots := make([]string, 0, len(loaded.Repositories)+1)
		roots = append(roots, appCfg.PrivateConfigDir)
		for _, repo := range loaded.Repositories {
			roots = append(roots, repo.Root)
		}
		release, err = acquireRegistrationMutationGuard(roots...)
		if err != nil {
			return err
		}
		defer release()
		if err := checkPendingCLITransactions(roots...); err != nil {
			return err
		}
	}
	if err := configureWorkspaceGitExcludes(loaded, dryRun); err != nil {
		return err
	}
	private, err := loaded.Repository(workspace.Private)
	if err != nil {
		return err
	}
	appCfg.PrivateConfigDir = private.Root
	if dryRun {
		fmt.Printf("Dry run: private repository %s would be registered\n", private.Root)
		return nil
	}
	if err := config.SaveAppConfig(appCfg); err != nil {
		return fmt.Errorf("saving app config: %w", err)
	}
	fmt.Printf("Private repository registered: %s\n", private.Root)
	return nil
}

func acquireRegistrationMutationGuard(roots ...string) (func(), error) {
	canonicalRoots := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		if root == "" {
			continue
		}
		canonical, err := canonicalDirectory(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("resolving repository root for registration guard: %w", err)
		}
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		canonicalRoots = append(canonicalRoots, canonical)
	}
	release, err := workspace.MutationGuard(context.Background(), "", canonicalRoots...)
	if err != nil {
		return nil, fmt.Errorf("guarding repository registration: %w", err)
	}
	if err := checkPendingCLITransactions(roots...); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func canonicalDirectory(path string) (string, error) {
	path = config.ExpandPath(path, nil)
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", canonical)
	}
	return filepath.Clean(canonical), nil
}

func getConfigDir() (string, error) {
	// 1. Use --dir flag if provided
	if configDir != "" {
		absPath, err := filepath.Abs(configDir)
		if err != nil {
			return "", fmt.Errorf("invalid config directory: %w", err)
		}
		return absPath, nil
	}

	// 2. Load from app config
	appCfg, err := config.LoadAppConfig()
	if err != nil {
		return "", err
	}

	return appCfg.ConfigDir, nil
}

func loadConfig() (*config.Config, *platform.Platform, string, error) {
	cfgDir, err := getConfigDir()
	if err != nil {
		return nil, nil, "", err
	}

	configFile := filepath.Join(cfgDir, "tidydots.yaml")
	cfg, err := config.Load(configFile)
	if err != nil {
		return nil, nil, "", fmt.Errorf("loading config from %s: %w", configFile, err)
	}

	cfg.BackupRoot = cfgDir

	plat := platform.Detect()

	if osOverride != "" {
		if osOverride != platform.OSLinux && osOverride != platform.OSWindows {
			return nil, nil, "", fmt.Errorf("invalid OS override: %s (must be 'linux' or 'windows')", osOverride)
		}
		plat = plat.WithOS(osOverride)
	}

	// Paths are kept with ~ in the config for portability
	// They will be expanded when needed for file operations

	return cfg, plat, configFile, nil
}

func createManager(args []string, allowSetup bool) (*manager.Manager, error) {
	cfg, plat, _, err := loadConfig()
	if err != nil {
		return nil, err
	}

	tmplCtx := tmpl.NewContextFromPlatform(plat)
	engine := tmpl.NewEngine(tmplCtx)
	cfg, err = selectConfigTarget(cfg, engine, args, allowSetup)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Detected OS: %s\n", plat.OS)
	fmt.Printf("Config directory: %s\n", cfg.BackupRoot)

	mgr := manager.New(cfg, plat)
	mgr.DryRun = dryRun
	mgr.Verbose = verbose
	mgr.NoMerge = noMerge
	mgr.ForceDelete = forceDelete
	mgr.ForceRender = forceRender

	if err := mgr.InitStateStore(); err != nil {
		fmt.Printf("Warning: could not initialize template state store: %v\n", err)
	}

	return mgr, nil
}

var createCommandManager = createManager

func runInteractive(cmd *cobra.Command, _ []string) error {
	w, workspacePlatform, routed, err := resolveCLIWorkspace(cmd)
	if err != nil {
		return err
	}
	if routed {
		if err := w.ValidateTargets(workspacePlatform); err != nil {
			return err
		}
		if !interactiveIsTerminal() {
			return fmt.Errorf("interactive mode requires a terminal; use subcommands (restore, backup, list) for non-interactive use")
		}
		return tui.RunWorkspace(w, workspacePlatform, dryRun, actions)
	}
	cfg, plat, configPath, err := loadConfig()
	if err != nil {
		return err
	}

	// Check if we're in a terminal
	if !interactiveIsTerminal() {
		return fmt.Errorf("interactive mode requires a terminal; use subcommands (restore, backup, list) for non-interactive use")
	}

	return runInteractiveTUI(cfg, plat, dryRun, configPath, actions)
}

func runStatus(cmd *cobra.Command, _ []string) error {
	w, plat, routed, err := resolveCLIWorkspace(cmd)
	if err != nil {
		return err
	}
	if routed {
		report, err := tui.ComputeWorkspaceStatus(w, plat, statusActions)
		if err != nil {
			return err
		}
		if statusJSON {
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(report)
		}
		return writeHumanStatus(cmd.OutOrStdout(), report)
	}
	cfg, plat, _, err := loadConfig()
	if err != nil {
		return err
	}

	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}
	mgr := manager.New(cfg, plat).WithLogger(slog.New(slog.NewTextHandler(
		cmd.ErrOrStderr(),
		&slog.HandlerOptions{Level: logLevel},
	)))
	mgr.Verbose = verbose
	if err := mgr.InitStateStore(); err != nil {
		return fmt.Errorf("initializing status checks: %w", err)
	}
	defer mgr.Close() //nolint:errcheck // best-effort cleanup

	report, err := tui.ComputeStatus(cfg, plat, mgr, statusActions)
	if err != nil {
		return err
	}

	if statusJSON {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return fmt.Errorf("writing status JSON: %w", err)
		}
		return nil
	}

	return writeHumanStatus(cmd.OutOrStdout(), report)
}

func writeHumanStatus(out io.Writer, report tui.StatusReport) error {
	showRepositoryOwners := statusHasPrivateOwner(report)
	if _, err := fmt.Fprintf(out, "Actionable: %t\n", report.Actionable); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Applications: %d (%d actionable)\n", report.Counts.Applications, report.Counts.ActionableApplications); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Entries: %d (%d actionable)\n", report.Counts.Entries, report.Counts.ActionableEntries); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Packages: %d (%d actionable)\n", report.Counts.Packages, report.Counts.ActionablePackages); err != nil {
		return err
	}

	for _, app := range report.Applications {
		if _, err := fmt.Fprintf(out, "\n%s: %s", app.Name, app.Status); err != nil {
			return err
		}
		if app.Actionable {
			if _, err := fmt.Fprint(out, " [actionable]"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if err := writeStatusApplication(out, app, showRepositoryOwners); err != nil {
			return err
		}
	}

	return nil
}

func statusHasPrivateOwner(report tui.StatusReport) bool {
	for _, app := range report.Applications {
		if app.Package != nil && app.Package.Repository == string(workspace.Private) {
			return true
		}
		for _, entry := range app.Entries {
			if entry.Repository == string(workspace.Private) {
				return true
			}
		}
	}
	return false
}

func writeStatusApplication(out io.Writer, app tui.StatusApplication, showOwners bool) error {
	if app.Package != nil {
		owner := ""
		if showOwners && app.Package.Repository != "" {
			owner = " [" + app.Package.Repository + "]"
		}
		if _, err := fmt.Fprintf(out, "  package: %s (%s)%s\n", app.Package.Name, app.Package.Method, owner); err != nil {
			return err
		}
	}
	for _, entry := range app.Entries {
		detail := entry.State
		if entry.Error != "" {
			detail += ": " + entry.Error
		}
		owner := ""
		if showOwners && entry.Repository != "" {
			owner = " [" + entry.Repository + "]"
		}
		if _, err := fmt.Fprintf(out, "  %s%s: %s\n", entry.Name, owner, detail); err != nil {
			return err
		}
	}
	return nil
}

func runRestore(cmd *cobra.Command, args []string) error {
	if err := validateInteractiveTarget(interactive, args); err != nil {
		return err
	}
	if interactive {
		return runInteractive(cmd, args)
	}
	w, plat, routed, err := resolveCLIWorkspace(cmd)
	if err != nil {
		return err
	}
	if routed {
		return runWorkspaceRestore(w, plat, args)
	}
	return runSingleFileOperation(args, true, func(mgr *manager.Manager) error {
		return runRestoreWithManager(mgr)
	})
}

func runRestoreWithManager(m manager.Restorer) error {
	return runWithCancellation(m.RestoreWithContext)
}

func runBackup(cmd *cobra.Command, args []string) error {
	if err := validateInteractiveTarget(interactive, args); err != nil {
		return err
	}
	if interactive {
		return runInteractive(cmd, args)
	}
	w, plat, routed, err := resolveCLIWorkspace(cmd)
	if err != nil {
		return err
	}
	if routed {
		return runWorkspaceBackup(w, plat, args)
	}
	return runSingleFileOperation(args, false, func(mgr *manager.Manager) error {
		return runBackupWithManager(mgr)
	})
}

func runSingleFileOperation(args []string, allowSetup bool, operation func(*manager.Manager) error) error {
	if !dryRun {
		release, err := acquireSingleRepositoryMutationGuard()
		if err != nil {
			return err
		}
		defer release()
	}

	mgr, err := createCommandManager(args, allowSetup)
	if err != nil {
		return err
	}
	defer mgr.Close() //nolint:errcheck // best-effort cleanup

	if dryRun {
		fmt.Println("=== DRY RUN MODE ===")
	}

	return operation(mgr)
}

func runBackupWithManager(m manager.Backuper) error {
	return runWithCancellation(m.BackupWithContext)
}

// runWithCancellation runs a context-aware function with signal-based cancellation.
// It sets up SIGINT/SIGTERM handling and cancels the context when a signal is received.
func runWithCancellation(fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	go func() {
		<-sigChan
		fmt.Println("\nOperation canceled by user")
		cancel()
	}()

	return fn(ctx)
}

func runList(cmd *cobra.Command, args []string) error {
	w, plat, routed, err := resolveCLIWorkspace(cmd)
	if err != nil {
		return err
	}
	if routed {
		return runWorkspaceList(w, plat, args)
	}
	mgr, err := createCommandManager(args, false)
	if err != nil {
		return err
	}
	defer mgr.Close() //nolint:errcheck // best-effort cleanup

	return runListWithManager(mgr)
}

func runListWithManager(m manager.Lister) error {
	return m.List()
}

func runInstall(cmd *cobra.Command, args []string) error {
	if interactive {
		return runInteractive(cmd, args)
	}
	w, workspacePlatform, routed, err := resolveCLIWorkspace(cmd)
	if err != nil {
		return err
	}
	if routed {
		return runWorkspaceInstall(w, workspacePlatform, args)
	}
	if !dryRun {
		release, err := acquireSingleRepositoryMutationGuard()
		if err != nil {
			return err
		}
		defer release()
	}

	cfg, plat, _, err := loadConfig()
	if err != nil {
		return err
	}

	fmt.Printf("Detected OS: %s\n", plat.OS)
	fmt.Printf("Config directory: %s\n", cfg.BackupRoot)

	// Create template engine for when expression evaluation
	tmplCtx := tmpl.NewContextFromPlatform(plat)
	engine := tmpl.NewEngine(tmplCtx)

	// Get filtered package entries
	packageEntries := cfg.GetFilteredPackages(engine)
	if len(packageEntries) == 0 {
		return fmt.Errorf("no matching packages configured in tidydots.yaml")
	}

	// Create package manager
	pkgMgr := packages.NewManager(&packages.Config{
		Packages:        packages.FromApplications(packageEntries),
		DefaultManager:  packages.PackageManager(cfg.DefaultManager),
		ManagerPriority: convertToPackageManagers(cfg.ManagerPriority),
	}, plat.OS, dryRun, verbose)

	fmt.Printf("Available package managers: %v\n", pkgMgr.Available)
	if pkgMgr.Preferred != "" {
		fmt.Printf("Preferred package manager: %s\n", pkgMgr.Preferred)
	}

	if dryRun {
		fmt.Println("=== DRY RUN MODE ===")
	}

	// Get installable packages
	packagesToInstall := pkgMgr.GetInstallablePackages()

	// Filter by name if args provided
	if len(args) > 0 {
		var filtered []packages.Package
		for _, pkg := range packagesToInstall {
			for _, name := range args {
				if pkg.Name == name {
					filtered = append(filtered, pkg)
					break
				}
			}
		}
		packagesToInstall = filtered
	}

	return installPackages(pkgMgr, packagesToInstall)
}

func installPackages(pkgMgr *packages.Manager, packagesToInstall []packages.Package) error {
	results := pkgMgr.InstallAll(packagesToInstall)

	// Print results
	successCount := 0
	failCount := 0
	for _, r := range results {
		if r.Success {
			fmt.Printf("[ok] %s: %s\n", r.Package, r.Message)
			successCount++
		} else {
			fmt.Printf("[error] %s: %s\n", r.Package, r.Message)
			failCount++
		}
	}

	fmt.Printf("\nInstallation complete: %d successful, %d failed\n", successCount, failCount)

	if failCount > 0 {
		return fmt.Errorf("%d packages failed to install", failCount)
	}
	return nil
}

func runListPackages(cmd *cobra.Command, _ []string) error {
	w, workspacePlatform, routed, err := resolveCLIWorkspace(cmd)
	if err != nil {
		return err
	}
	if routed {
		return runWorkspaceListPackages(w, workspacePlatform)
	}
	cfg, plat, _, err := loadConfig()
	if err != nil {
		return err
	}

	// Create template engine for when expression evaluation
	tmplCtx := tmpl.NewContextFromPlatform(plat)
	engine := tmpl.NewEngine(tmplCtx)

	// Get filtered package entries
	packageEntries := cfg.GetFilteredPackages(engine)
	if len(packageEntries) == 0 {
		fmt.Println("No matching packages configured in tidydots.yaml")
		return nil
	}

	// Create package manager to determine install methods
	pkgMgr := packages.NewManager(&packages.Config{
		Packages:        packages.FromApplications(packageEntries),
		DefaultManager:  packages.PackageManager(cfg.DefaultManager),
		ManagerPriority: convertToPackageManagers(cfg.ManagerPriority),
	}, plat.OS, false, verbose)

	fmt.Printf("Available package managers: %v\n\n", pkgMgr.Available)

	for _, pkg := range pkgMgr.Config.Packages {
		method := pkgMgr.GetInstallMethod(pkg)
		canInstall := pkgMgr.CanInstall(pkg)

		status := "✓"
		if !canInstall {
			status = "✗"
			method = "unavailable"
		}

		fmt.Printf("%s %s (%s)\n", status, pkg.Name, method)
		if pkg.Description != "" {
			fmt.Printf("    %s\n", pkg.Description)
		}
	}

	return nil
}

func convertToPackageManagers(strs []string) []packages.PackageManager {
	result := make([]packages.PackageManager, 0, len(strs))
	for _, s := range strs {
		result = append(result, packages.PackageManager(s))
	}
	return result
}

func runPreview(cmd *cobra.Command, args []string) error {
	w, workspacePlatform, routed, err := resolveCLIWorkspace(cmd)
	if err != nil {
		return err
	}
	if routed {
		_, ownedPath, err := workspacePreviewRoot(w, args[0])
		if err != nil {
			return err
		}
		if dryRun {
			return planPreview(workspacePlatform, ownedPath, cmd.OutOrStdout())
		}
		roots := make([]string, 0, len(w.Repositories))
		for _, repo := range w.Repositories {
			roots = append(roots, repo.Root)
		}
		release, err := workspace.MutationGuard(cmd.Context(), w.JournalDir, roots...)
		if err != nil {
			return fmt.Errorf("guarding preview workspace: %w", err)
		}
		defer release()
		if err := w.ValidateFresh(); err != nil {
			return fmt.Errorf("workspace configuration is stale; reload and retry: %w", err)
		}
		return runPreviewWithPlatform(workspacePlatform, ownedPath)
	}
	if dryRun {
		_, plat, _, err := loadConfig()
		if err != nil {
			return err
		}
		return planPreview(plat, args[0], cmd.OutOrStdout())
	}
	release, err := acquireSingleRepositoryMutationGuard()
	if err != nil {
		return err
	}
	defer release()
	_, plat, _, err := loadConfig()
	if err != nil {
		return err
	}
	return runPreviewWithPlatform(plat, args[0])
}

func planPreview(plat *platform.Platform, path string, out io.Writer) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat preview path %s: %w", path, err)
	}
	var templates []string
	if !info.IsDir() {
		if !tmpl.IsTemplateFile(path) {
			return fmt.Errorf("%s is not a template file (must end in .tmpl)", path)
		}
		templates = []string{path}
	} else {
		err := filepath.WalkDir(path, func(file string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() && tmpl.IsTemplateFile(file) {
				templates = append(templates, file)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("walking preview path %s: %w", path, err)
		}
	}
	if len(templates) == 0 {
		return fmt.Errorf("no template files found at %s", path)
	}
	engine := tmpl.NewEngine(tmpl.NewContextFromPlatform(plat))
	for _, sourcePath := range templates {
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", sourcePath, err)
		}
		rendered, _, err := engine.RenderStringWithSourceMap(filepath.Base(sourcePath), string(content))
		if err != nil {
			return fmt.Errorf("rendering template %s: %w", sourcePath, err)
		}
		if _, err := fmt.Fprintf(out, "Dry run: would render %s to %s (watcher not started)\n%s", sourcePath, tmpl.RenderedPath(sourcePath), rendered); err != nil {
			return err
		}
		if !strings.HasSuffix(rendered, "\n") {
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
	}
	return nil
}

func runPreviewWithPlatform(plat *platform.Platform, path string) error {
	tmplCtx := tmpl.NewContextFromPlatform(plat)
	engine := tmpl.NewEngine(tmplCtx)

	logger := slog.Default()
	if verbose {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	w := preview.New(engine, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return w.WatchWithStdin(ctx, path, os.Stdin)
}
