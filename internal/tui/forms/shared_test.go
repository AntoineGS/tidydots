package forms_test

import (
	"testing"

	"charm.land/bubbles/v2/textinput"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/tui/forms"
	"github.com/AntoineGS/tidydots/internal/tui/tuishared"
)

// makeInput is a test helper that sets a text input value and returns it.
func makeInput(value string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(value)
	return ti
}

func TestNewFormInput(t *testing.T) {
	tests := []struct {
		name        string
		placeholder string
		charLimit   int
		width       int
	}{
		{
			name:        "basic_input",
			placeholder: "e.g., neovim",
			charLimit:   64,
			width:       40,
		},
		{
			name:        "wide_input",
			placeholder: "enter expression",
			charLimit:   512,
			width:       60,
		},
		{
			name:        "empty_placeholder",
			placeholder: "",
			charLimit:   128,
			width:       40,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ti := forms.NewFormInput(tt.placeholder, tt.charLimit, tt.width)

			if ti.Placeholder != tt.placeholder {
				t.Errorf("Placeholder = %q, want %q", ti.Placeholder, tt.placeholder)
			}

			if ti.CharLimit != tt.charLimit {
				t.Errorf("CharLimit = %d, want %d", ti.CharLimit, tt.charLimit)
			}
		})
	}
}

func TestBuildPackageSpec(t *testing.T) {
	tests := []struct {
		name        string
		managers    map[string]string
		wantNil     bool
		wantLen     int
		wantManager string
		wantPackage string
	}{
		{
			name:     "nil_map_returns_nil",
			managers: nil,
			wantNil:  true,
		},
		{
			name:     "empty_map_returns_nil",
			managers: map[string]string{},
			wantNil:  true,
		},
		{
			name:        "single_manager",
			managers:    map[string]string{"pacman": "neovim"},
			wantNil:     false,
			wantLen:     1,
			wantManager: "pacman",
			wantPackage: "neovim",
		},
		{
			name: "multiple_managers",
			managers: map[string]string{
				"pacman": "neovim",
				"apt":    "neovim",
				"brew":   "neovim",
			},
			wantNil: false,
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := forms.BuildPackageSpec(tt.managers)

			if tt.wantNil {
				if result != nil {
					t.Errorf("BuildPackageSpec() = %v, want nil", result)
				}
				return
			}

			if result == nil {
				t.Fatal("BuildPackageSpec() = nil, want non-nil")
			}

			if len(result.Managers) != tt.wantLen {
				t.Errorf("len(Managers) = %d, want %d", len(result.Managers), tt.wantLen)
			}

			if tt.wantManager != "" {
				mv, ok := result.Managers[tt.wantManager]
				if !ok {
					t.Errorf("Managers[%s] not found", tt.wantManager)
				} else if mv.PackageName != tt.wantPackage {
					t.Errorf("Managers[%s].PackageName = %q, want %q", tt.wantManager, mv.PackageName, tt.wantPackage)
				}
			}

			// Verify all managers from input are present
			for mgr, pkg := range tt.managers {
				mv, ok := result.Managers[mgr]
				if !ok {
					t.Errorf("Managers[%s] not found", mgr)
				} else if mv.PackageName != pkg {
					t.Errorf("Managers[%s].PackageName = %q, want %q", mgr, mv.PackageName, pkg)
				}
			}
		})
	}
}

func TestMergeGitPackage(t *testing.T) {
	t.Run("hasGit_false_returns_pkg_unchanged", func(t *testing.T) {
		pkg := &config.EntryPackage{
			Managers: map[string]config.ManagerValue{
				"pacman": {PackageName: "neovim"},
			},
		}
		result := forms.MergeGitPackage(pkg, false, makeInput(""), makeInput(""), makeInput(""), makeInput(""), false)
		if result != pkg {
			t.Error("expected same pkg pointer returned when hasGit=false")
		}
	})

	t.Run("nil_pkg_with_empty_url_returns_nil", func(t *testing.T) {
		result := forms.MergeGitPackage(nil, true, makeInput(""), makeInput(""), makeInput(""), makeInput(""), false)
		if result != nil {
			t.Errorf("expected nil when URL is empty, got %v", result)
		}
	})

	t.Run("nil_pkg_created_when_url_set", func(t *testing.T) {
		result := forms.MergeGitPackage(
			nil, true,
			makeInput("https://github.com/user/repo.git"),
			makeInput("main"),
			makeInput("~/.local/share/app"),
			makeInput(""),
			false,
		)
		if result == nil {
			t.Fatal("expected non-nil pkg when URL is set")
		}
		gitMV, ok := result.Managers[tuishared.TypeGit]
		if !ok {
			t.Fatal("expected git manager in result")
		}
		if !gitMV.IsGit() {
			t.Fatal("expected git manager value to be a git package")
		}
		if gitMV.Git.URL != "https://github.com/user/repo.git" {
			t.Errorf("URL = %q, want %q", gitMV.Git.URL, "https://github.com/user/repo.git")
		}
		if gitMV.Git.Branch != "main" {
			t.Errorf("Branch = %q, want %q", gitMV.Git.Branch, "main")
		}
		if gitMV.Git.Targets[tuishared.OSLinux] != "~/.local/share/app" {
			t.Errorf("Targets[linux] = %q, want %q", gitMV.Git.Targets[tuishared.OSLinux], "~/.local/share/app")
		}
		if _, ok := gitMV.Git.Targets[tuishared.OSWindows]; ok {
			t.Error("Targets[windows] should not be set when empty")
		}
	})

	t.Run("targets_set_for_both_os", func(t *testing.T) {
		result := forms.MergeGitPackage(
			nil, true,
			makeInput("https://github.com/user/repo.git"),
			makeInput(""),
			makeInput("~/.local/share/app"),
			makeInput("~/AppData/Local/app"),
			false,
		)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		gitMV := result.Managers[tuishared.TypeGit]
		if gitMV.Git.Targets[tuishared.OSLinux] != "~/.local/share/app" {
			t.Errorf("Targets[linux] = %q, want %q", gitMV.Git.Targets[tuishared.OSLinux], "~/.local/share/app")
		}
		if gitMV.Git.Targets[tuishared.OSWindows] != "~/AppData/Local/app" {
			t.Errorf("Targets[windows] = %q, want %q", gitMV.Git.Targets[tuishared.OSWindows], "~/AppData/Local/app")
		}
	})

	t.Run("sudo_preserved_when_true", func(t *testing.T) {
		result := forms.MergeGitPackage(
			nil, true,
			makeInput("https://github.com/user/repo.git"),
			makeInput(""),
			makeInput("~/.local"),
			makeInput(""),
			true,
		)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		gitMV := result.Managers[tuishared.TypeGit]
		if !gitMV.Git.Sudo {
			t.Error("Sudo = false, want true")
		}
	})

	t.Run("existing_pkg_preserved", func(t *testing.T) {
		existing := &config.EntryPackage{
			Managers: map[string]config.ManagerValue{
				"pacman": {PackageName: "neovim"},
			},
		}
		result := forms.MergeGitPackage(
			existing, true,
			makeInput("https://github.com/user/repo.git"),
			makeInput(""),
			makeInput("~/.local"),
			makeInput(""),
			false,
		)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		if len(result.Managers) != 2 {
			t.Errorf("len(Managers) = %d, want 2", len(result.Managers))
		}
		if result.Managers["pacman"].PackageName != "neovim" {
			t.Error("existing pacman manager should be preserved")
		}
	})
}

func TestMergeInstallerPackage(t *testing.T) {
	t.Run("hasInstaller_false_returns_pkg_unchanged", func(t *testing.T) {
		pkg := &config.EntryPackage{
			Managers: map[string]config.ManagerValue{
				"pacman": {PackageName: "neovim"},
			},
		}
		result := forms.MergeInstallerPackage(pkg, false, makeInput(""), makeInput(""), makeInput(""))
		if result != pkg {
			t.Error("expected same pkg pointer returned when hasInstaller=false")
		}
	})

	t.Run("nil_pkg_with_empty_commands_returns_nil", func(t *testing.T) {
		result := forms.MergeInstallerPackage(nil, true, makeInput(""), makeInput(""), makeInput("binary"))
		if result != nil {
			t.Errorf("expected nil when both commands are empty, got %v", result)
		}
	})

	t.Run("nil_pkg_created_when_linux_set", func(t *testing.T) {
		result := forms.MergeInstallerPackage(
			nil, true,
			makeInput("curl -fsSL https://example.com/install.sh | sh"),
			makeInput(""),
			makeInput("mytool"),
		)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		installerMV, ok := result.Managers[tuishared.TypeInstaller]
		if !ok {
			t.Fatal("expected installer manager in result")
		}
		if !installerMV.IsInstaller() {
			t.Fatal("expected installer manager value to be an installer package")
		}
		if installerMV.Installer.Command[tuishared.OSLinux] != "curl -fsSL https://example.com/install.sh | sh" {
			t.Errorf("Command[linux] = %q, want %q", installerMV.Installer.Command[tuishared.OSLinux], "curl -fsSL https://example.com/install.sh | sh")
		}
		if installerMV.Installer.Binary != "mytool" {
			t.Errorf("Binary = %q, want %q", installerMV.Installer.Binary, "mytool")
		}
	})

	t.Run("binary_preserved_when_set", func(t *testing.T) {
		result := forms.MergeInstallerPackage(
			nil, true,
			makeInput("make install"),
			makeInput(""),
			makeInput("cargo"),
		)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		installerMV := result.Managers[tuishared.TypeInstaller]
		if installerMV.Installer.Binary != "cargo" {
			t.Errorf("Binary = %q, want %q", installerMV.Installer.Binary, "cargo")
		}
	})

	t.Run("both_os_commands_set", func(t *testing.T) {
		result := forms.MergeInstallerPackage(
			nil, true,
			makeInput("make install"),
			makeInput("setup.exe /S"),
			makeInput(""),
		)
		if result == nil {
			t.Fatal("expected non-nil result")
		}
		installerMV := result.Managers[tuishared.TypeInstaller]
		if installerMV.Installer.Command[tuishared.OSLinux] != "make install" {
			t.Errorf("Command[linux] = %q, want %q", installerMV.Installer.Command[tuishared.OSLinux], "make install")
		}
		if installerMV.Installer.Command[tuishared.OSWindows] != "setup.exe /S" {
			t.Errorf("Command[windows] = %q, want %q", installerMV.Installer.Command[tuishared.OSWindows], "setup.exe /S")
		}
	})
}

func TestBuildTargetsFromInputs(t *testing.T) {
	tests := []struct {
		name           string
		linuxValue     string
		windowsValue   string
		wantLinux      string
		wantWindows    string
		wantLinuxKey   bool
		wantWindowsKey bool
	}{
		{
			name:           "both_set",
			linuxValue:     "~/.config/nvim",
			windowsValue:   "~/AppData/Local/nvim",
			wantLinux:      "~/.config/nvim",
			wantWindows:    "~/AppData/Local/nvim",
			wantLinuxKey:   true,
			wantWindowsKey: true,
		},
		{
			name:           "linux_only",
			linuxValue:     "~/.config/nvim",
			windowsValue:   "",
			wantLinux:      "~/.config/nvim",
			wantLinuxKey:   true,
			wantWindowsKey: false,
		},
		{
			name:           "windows_only",
			linuxValue:     "",
			windowsValue:   "~/AppData/Local/nvim",
			wantWindows:    "~/AppData/Local/nvim",
			wantLinuxKey:   false,
			wantWindowsKey: true,
		},
		{
			name:           "both_empty_returns_empty_map",
			linuxValue:     "",
			windowsValue:   "",
			wantLinuxKey:   false,
			wantWindowsKey: false,
		},
		{
			name:           "whitespace_trimmed",
			linuxValue:     "  ~/.config/nvim  ",
			windowsValue:   "",
			wantLinux:      "~/.config/nvim",
			wantLinuxKey:   true,
			wantWindowsKey: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := forms.BuildTargetsFromInputs(makeInput(tt.linuxValue), makeInput(tt.windowsValue))

			linuxVal, hasLinux := result["linux"]
			windowsVal, hasWindows := result["windows"]

			if hasLinux != tt.wantLinuxKey {
				t.Errorf("linux key present = %v, want %v", hasLinux, tt.wantLinuxKey)
			}
			if hasWindows != tt.wantWindowsKey {
				t.Errorf("windows key present = %v, want %v", hasWindows, tt.wantWindowsKey)
			}
			if tt.wantLinuxKey && linuxVal != tt.wantLinux {
				t.Errorf("linux = %q, want %q", linuxVal, tt.wantLinux)
			}
			if tt.wantWindowsKey && windowsVal != tt.wantWindows {
				t.Errorf("windows = %q, want %q", windowsVal, tt.wantWindows)
			}
		})
	}
}

func TestNewGitTextInputs(t *testing.T) {
	gitURL, gitBranch, gitLinux, gitWindows := forms.NewGitTextInputs()

	inputs := []struct {
		name        string
		input       textinput.Model
		placeholder string
		charLimit   int
	}{
		{"gitURL", gitURL, tuishared.PlaceholderGitURL, tuishared.CharLimitPath},
		{"gitBranch", gitBranch, tuishared.PlaceholderGitBranch, tuishared.CharLimitBranch},
		{"gitLinux", gitLinux, tuishared.PlaceholderGitLinux, tuishared.CharLimitPath},
		{"gitWindows", gitWindows, tuishared.PlaceholderGitWindows, tuishared.CharLimitPath},
	}

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input.Placeholder != tt.placeholder {
				t.Errorf("Placeholder = %q, want %q", tt.input.Placeholder, tt.placeholder)
			}
			if tt.input.CharLimit != tt.charLimit {
				t.Errorf("CharLimit = %d, want %d", tt.input.CharLimit, tt.charLimit)
			}
		})
	}
}

func TestNewInstallerTextInputs(t *testing.T) {
	installerLinux, installerWindows, installerBinary := forms.NewInstallerTextInputs()

	inputs := []struct {
		name        string
		input       textinput.Model
		placeholder string
		charLimit   int
	}{
		{"installerLinux", installerLinux, tuishared.PlaceholderInstallerLinux, tuishared.CharLimitURL},
		{"installerWindows", installerWindows, tuishared.PlaceholderInstallerWindows, tuishared.CharLimitURL},
		{"installerBinary", installerBinary, tuishared.PlaceholderInstallerBinary, tuishared.CharLimitBinary},
	}

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			if tt.input.Placeholder != tt.placeholder {
				t.Errorf("Placeholder = %q, want %q", tt.input.Placeholder, tt.placeholder)
			}
			if tt.input.CharLimit != tt.charLimit {
				t.Errorf("CharLimit = %d, want %d", tt.input.CharLimit, tt.charLimit)
			}
		})
	}
}

func TestDisplayPackageManagers_ExcludesGit(t *testing.T) {
	for _, mgr := range forms.DisplayPackageManagers {
		if mgr == "git" {
			t.Error("DisplayPackageManagers should not contain 'git'")
		}
	}

	if len(forms.DisplayPackageManagers) == 0 {
		t.Error("DisplayPackageManagers should not be empty")
	}
}
