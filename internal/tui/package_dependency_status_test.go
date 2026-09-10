package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/manager"
	"github.com/AntoineGS/tidydots/internal/packages"
	"github.com/AntoineGS/tidydots/internal/platform"
)

func TestPackageStateCheckRequiresEveryInstallationPlanDependency(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX package-manager fixtures")
	}

	tests := []struct {
		name      string
		installed []string
		want      bool
	}{
		{
			name:      "all native dependencies installed",
			installed: []string{"main", "pacman-dep", "brew-dep"},
			want:      true,
		},
		{
			name:      "selected manager dependency missing",
			installed: []string{"main", "brew-dep"},
			want:      false,
		},
		{
			name:      "alternate manager dependency missing",
			installed: []string{"main", "pacman-dep"},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupPackageStatusFixtures(t, tt.installed)

			pkg := &config.EntryPackage{Managers: map[string]config.ManagerValue{
				string(packages.Pacman): {PackageName: "main", Deps: []string{"pacman-dep"}},
				string(packages.Brew):   {PackageName: "alternate-main", Deps: []string{"brew-dep"}},
			}}
			m := newStatusRefreshModel(t, pkg)
			m.Config.ManagerPriority = []string{string(packages.Pacman)}

			msg, ok := m.packageStateCheckCmd(0)().(pkgCheckResultMsg)
			if !ok {
				t.Fatal("package state check returned an unexpected message")
			}
			if msg.method != string(packages.Pacman) {
				t.Fatalf("selected method = %q, want %q", msg.method, packages.Pacman)
			}
			if msg.installed != tt.want {
				t.Fatalf("installed = %t, want %t", msg.installed, tt.want)
			}
		})
	}
}

func TestComputeStatusMakesMissingPlanDependencyActionable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX package-manager fixtures")
	}
	setupPackageStatusFixtures(t, []string{"main", "pacman-dep"})

	pkg := &config.EntryPackage{Managers: map[string]config.ManagerValue{
		string(packages.Pacman): {PackageName: "main", Deps: []string{"pacman-dep"}},
		string(packages.Brew):   {PackageName: "alternate-main", Deps: []string{"brew-dep"}},
	}}
	cfg := &config.Config{
		Version:         3,
		BackupRoot:      t.TempDir(),
		ManagerPriority: []string{string(packages.Pacman)},
		Applications:    []config.Application{{Name: "tool", Package: pkg}},
	}
	plat := &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}

	report, err := ComputeStatus(cfg, plat, manager.New(cfg, plat), true)
	if err != nil {
		t.Fatalf("ComputeStatus(actionsOnly): %v", err)
	}
	if !report.Actionable {
		t.Fatal("status report is not actionable for a missing dependency")
	}
	if report.Counts.ActionablePackages != 1 || report.Counts.ActionableApplications != 1 {
		t.Fatalf("actionable counts = %+v, want one package and application", report.Counts)
	}
	if len(report.Applications) != 1 || report.Applications[0].Package == nil {
		t.Fatalf("actionable applications = %+v, want the package application", report.Applications)
	}
	installed := report.Applications[0].Package.Installed
	if installed == nil || *installed {
		t.Fatalf("package installed status = %v, want false", installed)
	}
}

func TestPackageStateCheckRejectsInvalidDependencyWithoutLaunchingItsCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses POSIX package-manager fixtures")
	}

	marker := filepath.Join(t.TempDir(), "package-checks")
	setupPackageStatusFixturesWithMarker(t, []string{"main"}, marker)
	pkg := &config.EntryPackage{Managers: map[string]config.ManagerValue{
		string(packages.Pacman): {PackageName: "main", Deps: []string{"--dangerous-status-dependency"}},
	}}
	m := newStatusRefreshModel(t, pkg)
	m.Config.ManagerPriority = []string{string(packages.Pacman)}

	msg, ok := m.packageStateCheckCmd(0)().(pkgCheckResultMsg)
	if !ok {
		t.Fatal("package state check returned an unexpected message")
	}
	if msg.installed {
		t.Fatal("invalid dependency was reported as installed")
	}

	checks, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read package-check marker: %v", err)
	}
	if got := string(checks); strings.TrimSpace(got) != "main" {
		t.Fatalf("package checks = %q, want only the installed main package", got)
	}
}

func setupPackageStatusFixtures(t *testing.T, installed []string) {
	setupPackageStatusFixturesWithMarker(t, installed, "")
}

func setupPackageStatusFixturesWithMarker(t *testing.T, installed []string, marker string) {
	t.Helper()

	platform.ResetAvailableManagersCache()
	packages.ResetInstalledCache()
	t.Cleanup(platform.ResetAvailableManagersCache)
	t.Cleanup(packages.ResetInstalledCache)
	t.Setenv("PATH", t.TempDir())
	if marker != "" {
		t.Setenv("TIDYDOTS_STATUS_MARKER", marker)
	}
	platform.SetDetectionHints(platform.OSLinux, false)

	binDir := os.Getenv("PATH")
	for _, managerName := range []string{string(packages.Pacman), string(packages.Brew)} {
		path := filepath.Join(binDir, managerName)
		script := "#!/bin/sh\ncase \"$2\" in\n"
		if marker != "" {
			script = "#!/bin/sh\nprintf '%s\\n' \"$2\" >> \"$TIDYDOTS_STATUS_MARKER\"\ncase \"$2\" in\n"
		}
		for _, packageName := range installed {
			script += fmt.Sprintf("  %s) exit 0;;\n", packageName)
		}
		script += "esac\nexit 1\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", managerName, err)
		}
	}
}
