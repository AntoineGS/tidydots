package forms_test

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/tui/forms"
)

func TestNewApplicationForm_NewMode(t *testing.T) {
	app := config.Application{}
	form := forms.NewApplicationForm(app, false)

	if form == nil {
		t.Fatal("NewApplicationForm returned nil")
	}

	t.Run("edit_idx_is_minus_one_for_new", func(t *testing.T) {
		if form.EditAppIdx != -1 {
			t.Errorf("EditAppIdx = %d, want -1", form.EditAppIdx)
		}
	})

	t.Run("inputs_are_empty", func(t *testing.T) {
		if form.NameInput.Value() != "" {
			t.Errorf("NameInput = %q, want empty", form.NameInput.Value())
		}
		if form.DescriptionInput.Value() != "" {
			t.Errorf("DescriptionInput = %q, want empty", form.DescriptionInput.Value())
		}
		if form.WhenInput.Value() != "" {
			t.Errorf("WhenInput = %q, want empty", form.WhenInput.Value())
		}
	})

	t.Run("no_git_package", func(t *testing.T) {
		if form.HasGitPackage {
			t.Error("HasGitPackage = true, want false for new form")
		}
		if form.GitFieldCursor != -1 {
			t.Errorf("GitFieldCursor = %d, want -1", form.GitFieldCursor)
		}
	})

	t.Run("no_installer_package", func(t *testing.T) {
		if form.HasInstallerPackage {
			t.Error("HasInstallerPackage = true, want false for new form")
		}
		if form.InstallerFieldCursor != -1 {
			t.Errorf("InstallerFieldCursor = %d, want -1", form.InstallerFieldCursor)
		}
	})

	t.Run("package_managers_empty", func(t *testing.T) {
		if len(form.PackageManagers) != 0 {
			t.Errorf("len(PackageManagers) = %d, want 0", len(form.PackageManagers))
		}
	})

	t.Run("package_deps_empty", func(t *testing.T) {
		if form.PackageDeps == nil {
			t.Error("PackageDeps is nil, want empty map")
		}
		if len(form.PackageDeps) != 0 {
			t.Errorf("len(PackageDeps) = %d, want 0", len(form.PackageDeps))
		}
	})
}

func TestNewApplicationForm_EditMode(t *testing.T) {
	app := config.Application{
		Name:        "nvim",
		Description: "Neovim text editor",
		When:        `{{ eq .OS "linux" }}`,
	}
	form := forms.NewApplicationForm(app, true)

	if form == nil {
		t.Fatal("NewApplicationForm returned nil")
	}

	t.Run("edit_idx_is_zero_for_edit", func(t *testing.T) {
		if form.EditAppIdx != 0 {
			t.Errorf("EditAppIdx = %d, want 0", form.EditAppIdx)
		}
	})

	t.Run("inputs_loaded_from_app", func(t *testing.T) {
		if form.NameInput.Value() != "nvim" {
			t.Errorf("NameInput = %q, want %q", form.NameInput.Value(), "nvim")
		}
		if form.DescriptionInput.Value() != "Neovim text editor" {
			t.Errorf("DescriptionInput = %q, want %q", form.DescriptionInput.Value(), "Neovim text editor")
		}
		if form.WhenInput.Value() != `{{ eq .OS "linux" }}` {
			t.Errorf("WhenInput = %q, want %q", form.WhenInput.Value(), `{{ eq .OS "linux" }}`)
		}
	})
}

func TestNewApplicationForm_LoadsPackageManagers(t *testing.T) {
	app := config.Application{
		Name: "test",
		Package: &config.EntryPackage{
			Managers: map[string]config.ManagerValue{
				"pacman": {PackageName: "neovim"},
				"apt":    {PackageName: "neovim"},
			},
		},
	}
	form := forms.NewApplicationForm(app, true)

	if len(form.PackageManagers) != 2 {
		t.Errorf("len(PackageManagers) = %d, want 2", len(form.PackageManagers))
	}
	if form.PackageManagers["pacman"] != "neovim" {
		t.Errorf("PackageManagers[pacman] = %q, want %q", form.PackageManagers["pacman"], "neovim")
	}
	if form.PackageManagers["apt"] != "neovim" {
		t.Errorf("PackageManagers[apt] = %q, want %q", form.PackageManagers["apt"], "neovim")
	}
}

func TestNewApplicationForm_LoadsGitPackage(t *testing.T) {
	app := config.Application{
		Name: "test",
		Package: &config.EntryPackage{
			Managers: map[string]config.ManagerValue{
				"git": {Git: &config.GitPackage{
					URL:    "https://github.com/user/repo.git",
					Branch: "main",
					Targets: map[string]string{
						"linux":   "~/.local/share/app",
						"windows": "~/AppData/Local/app",
					},
					Sudo: true,
				}},
			},
		},
	}
	form := forms.NewApplicationForm(app, true)

	if !form.HasGitPackage {
		t.Error("HasGitPackage = false, want true")
	}
	if form.GitURLInput.Value() != "https://github.com/user/repo.git" {
		t.Errorf("GitURLInput = %q, want %q", form.GitURLInput.Value(), "https://github.com/user/repo.git")
	}
	if form.GitBranchInput.Value() != "main" {
		t.Errorf("GitBranchInput = %q, want %q", form.GitBranchInput.Value(), "main")
	}
	if form.GitLinuxInput.Value() != "~/.local/share/app" {
		t.Errorf("GitLinuxInput = %q, want %q", form.GitLinuxInput.Value(), "~/.local/share/app")
	}
	if form.GitWindowsInput.Value() != "~/AppData/Local/app" {
		t.Errorf("GitWindowsInput = %q, want %q", form.GitWindowsInput.Value(), "~/AppData/Local/app")
	}
	if !form.GitSudo {
		t.Error("GitSudo = false, want true")
	}
}

func TestNewApplicationForm_LoadsInstallerPackage(t *testing.T) {
	app := config.Application{
		Name: "test",
		Package: &config.EntryPackage{
			Managers: map[string]config.ManagerValue{
				"installer": {Installer: &config.InstallerPackage{
					Command: map[string]string{
						"linux":   "curl -fsSL example.com | sh",
						"windows": "iwr example.com | iex",
					},
					Binary: "mytool",
				}},
			},
		},
	}
	form := forms.NewApplicationForm(app, true)

	if !form.HasInstallerPackage {
		t.Error("HasInstallerPackage = false, want true")
	}
	if form.InstallerLinuxInput.Value() != "curl -fsSL example.com | sh" {
		t.Errorf("InstallerLinuxInput = %q, want %q", form.InstallerLinuxInput.Value(), "curl -fsSL example.com | sh")
	}
	if form.InstallerWindowsInput.Value() != "iwr example.com | iex" {
		t.Errorf("InstallerWindowsInput = %q, want %q", form.InstallerWindowsInput.Value(), "iwr example.com | iex")
	}
	if form.InstallerBinaryInput.Value() != "mytool" {
		t.Errorf("InstallerBinaryInput = %q, want %q", form.InstallerBinaryInput.Value(), "mytool")
	}
}

func TestNewApplicationForm_LoadsPackageDeps(t *testing.T) {
	app := config.Application{
		Name: "test",
		Package: &config.EntryPackage{
			Managers: map[string]config.ManagerValue{
				"pacman": {PackageName: "neovim", Deps: []string{"ripgrep", "fd"}},
			},
		},
	}
	form := forms.NewApplicationForm(app, true)

	if len(form.PackageDeps) == 0 {
		t.Fatal("PackageDeps should not be empty")
	}
	deps, ok := form.PackageDeps["pacman"]
	if !ok {
		t.Fatal("PackageDeps[pacman] not found")
	}
	if len(deps) != 2 {
		t.Errorf("len(deps) = %d, want 2", len(deps))
	}
}

func TestApplicationForm_Validate(t *testing.T) {
	tests := []struct {
		name    string
		appName string
		wantErr bool
	}{
		{
			name:    "valid_name",
			appName: "neovim",
			wantErr: false,
		},
		{
			name:    "empty_name_returns_error",
			appName: "",
			wantErr: true,
		},
		{
			name:    "whitespace_only_name_returns_error",
			appName: "   ",
			wantErr: true,
		},
		{
			name:    "name_with_special_chars",
			appName: "my-app_v2",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := forms.NewApplicationForm(config.Application{Name: tt.appName}, false)
			err := form.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplicationForm_BuildApplication(t *testing.T) {
	t.Run("nil_form_returns_error", func(t *testing.T) {
		var f *forms.ApplicationForm
		_, _, _, _, err := f.BuildApplication()
		if err == nil {
			t.Error("BuildApplication() on nil form should return error")
		}
	})

	t.Run("empty_name_returns_error", func(t *testing.T) {
		form := forms.NewApplicationForm(config.Application{Name: ""}, false)
		_, _, _, _, err := form.BuildApplication()
		if err == nil {
			t.Error("BuildApplication() with empty name should return error")
		}
	})

	t.Run("builds_basic_application", func(t *testing.T) {
		app := config.Application{
			Name:        "nvim",
			Description: "Neovim text editor",
			When:        `{{ eq .OS "linux" }}`,
		}
		form := forms.NewApplicationForm(app, false)
		name, description, when, pkg, err := form.BuildApplication()

		if err != nil {
			t.Fatalf("BuildApplication() unexpected error: %v", err)
		}
		if name != "nvim" {
			t.Errorf("name = %q, want %q", name, "nvim")
		}
		if description != "Neovim text editor" {
			t.Errorf("description = %q, want %q", description, "Neovim text editor")
		}
		if when != `{{ eq .OS "linux" }}` {
			t.Errorf("when = %q, want %q", when, `{{ eq .OS "linux" }}`)
		}
		if pkg != nil {
			t.Errorf("pkg = %v, want nil (no package managers set)", pkg)
		}
	})

	t.Run("builds_with_package_managers", func(t *testing.T) {
		app := config.Application{
			Name: "nvim",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"pacman": {PackageName: "neovim"},
				},
			},
		}
		form := forms.NewApplicationForm(app, false)
		_, _, _, pkg, err := form.BuildApplication()

		if err != nil {
			t.Fatalf("BuildApplication() unexpected error: %v", err)
		}
		if pkg == nil {
			t.Fatal("pkg = nil, want non-nil")
		}
		if pkg.Managers["pacman"].PackageName != "neovim" {
			t.Errorf("pkg.Managers[pacman].PackageName = %q, want %q", pkg.Managers["pacman"].PackageName, "neovim")
		}
	})

	t.Run("git_package_requires_url", func(t *testing.T) {
		form := forms.NewApplicationForm(config.Application{Name: "test"}, false)
		form.HasGitPackage = true
		// GitURLInput is empty by default

		_, _, _, _, err := form.BuildApplication()
		if err == nil {
			t.Error("BuildApplication() with git package but no URL should return error")
		}
	})

	t.Run("git_package_requires_target", func(t *testing.T) {
		form := forms.NewApplicationForm(config.Application{Name: "test"}, false)
		form.HasGitPackage = true
		form.GitURLInput.SetValue("https://github.com/user/repo.git")
		// Both linux and windows targets are empty

		_, _, _, _, err := form.BuildApplication()
		if err == nil {
			t.Error("BuildApplication() with git package but no targets should return error")
		}
	})

	t.Run("installer_package_requires_command", func(t *testing.T) {
		form := forms.NewApplicationForm(config.Application{Name: "test"}, false)
		form.HasInstallerPackage = true
		// Both command inputs are empty

		_, _, _, _, err := form.BuildApplication()
		if err == nil {
			t.Error("BuildApplication() with installer package but no commands should return error")
		}
	})
}

func TestApplicationForm_GetFieldType(t *testing.T) {
	tests := []struct {
		name       string
		focusIndex int
		wantType   forms.ApplicationFieldType
	}{
		{
			name:       "index_0_is_name",
			focusIndex: 0,
			wantType:   forms.AppFieldName,
		},
		{
			name:       "index_1_is_description",
			focusIndex: 1,
			wantType:   forms.AppFieldDescription,
		},
		{
			name:       "index_2_is_packages",
			focusIndex: 2,
			wantType:   forms.AppFieldPackages,
		},
		{
			name:       "index_3_is_when",
			focusIndex: 3,
			wantType:   forms.AppFieldWhen,
		},
		{
			name:       "out_of_range_defaults_to_name",
			focusIndex: 99,
			wantType:   forms.AppFieldName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := forms.NewApplicationForm(config.Application{Name: "test"}, false)
			form.FocusIndex = tt.focusIndex
			got := form.GetFieldType()
			if got != tt.wantType {
				t.Errorf("GetFieldType() = %v, want %v", got, tt.wantType)
			}
		})
	}

	t.Run("nil_form_returns_name", func(t *testing.T) {
		var f *forms.ApplicationForm
		got := f.GetFieldType()
		if got != forms.AppFieldName {
			t.Errorf("GetFieldType() on nil = %v, want AppFieldName", got)
		}
	})
}

func TestApplicationForm_ResetCursors(t *testing.T) {
	form := forms.NewApplicationForm(config.Application{Name: "test"}, false)
	form.PackagesCursor = 5
	form.GitFieldCursor = 3
	form.InstallerFieldCursor = 2
	form.EditingGitField = true
	form.EditingInstallerField = true
	form.EditingPackage = true
	form.EditingDeps = true
	form.EditingDepItem = true

	form.ResetCursors()

	if form.PackagesCursor != 0 {
		t.Errorf("PackagesCursor = %d, want 0", form.PackagesCursor)
	}
	if form.GitFieldCursor != -1 {
		t.Errorf("GitFieldCursor = %d, want -1", form.GitFieldCursor)
	}
	if form.InstallerFieldCursor != -1 {
		t.Errorf("InstallerFieldCursor = %d, want -1", form.InstallerFieldCursor)
	}
	if form.EditingGitField {
		t.Error("EditingGitField = true, want false")
	}
	if form.EditingInstallerField {
		t.Error("EditingInstallerField = true, want false")
	}
	if form.EditingPackage {
		t.Error("EditingPackage = true, want false")
	}
	if form.EditingDeps {
		t.Error("EditingDeps = true, want false")
	}
	if form.EditingDepItem {
		t.Error("EditingDepItem = true, want false")
	}
}

func TestApplicationForm_GitGitPackage_ExcludedFromPackageManagers(t *testing.T) {
	// Git and installer managers should not appear in PackageManagers map
	app := config.Application{
		Name: "test",
		Package: &config.EntryPackage{
			Managers: map[string]config.ManagerValue{
				"pacman": {PackageName: "neovim"},
				"git": {Git: &config.GitPackage{
					URL:     "https://github.com/user/repo.git",
					Targets: map[string]string{"linux": "~/.local"},
				}},
				"installer": {Installer: &config.InstallerPackage{
					Command: map[string]string{"linux": "make install"},
				}},
			},
		},
	}
	form := forms.NewApplicationForm(app, true)

	// PackageManagers should only have "pacman", not "git" or "installer"
	if len(form.PackageManagers) != 1 {
		t.Errorf("len(PackageManagers) = %d, want 1 (only pacman)", len(form.PackageManagers))
	}
	if _, hasGit := form.PackageManagers["git"]; hasGit {
		t.Error("git should not appear in PackageManagers")
	}
	if _, hasInstaller := form.PackageManagers["installer"]; hasInstaller {
		t.Error("installer should not appear in PackageManagers")
	}
}
