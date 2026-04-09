package tui

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
)

func TestApplicationForm_Validation(t *testing.T) {
	tests := []struct {
		name    string
		app     config.Application
		wantErr bool
	}{
		{
			name: "valid_application",
			app: config.Application{
				Name:        "test",
				Description: "Test app",
			},
			wantErr: false,
		},
		{
			name: "empty_name",
			app: config.Application{
				Name:        "",
				Description: "Test",
			},
			wantErr: true,
		},
		{
			name: "whitespace_name",
			app: config.Application{
				Name:        "   ",
				Description: "Test",
			},
			wantErr: true,
		},
		{
			name: "valid_with_empty_description",
			app: config.Application{
				Name:        "test",
				Description: "",
			},
			wantErr: false,
		},
		{
			name: "valid_with_special_chars",
			app: config.Application{
				Name:        "test-app_v1",
				Description: "Test app with special chars!",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := NewApplicationForm(tt.app, false)
			err := form.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApplicationForm_EditMode(t *testing.T) {
	app := config.Application{
		Name:        "test",
		Description: "Test app",
	}

	// Test new form (not editing)
	newForm := NewApplicationForm(app, false)
	if newForm.EditAppIdx != -1 {
		t.Errorf("NewApplicationForm(false) EditAppIdx = %d, want -1", newForm.EditAppIdx)
	}

	// Test edit form
	editForm := NewApplicationForm(app, true)
	if editForm.EditAppIdx != 0 {
		t.Errorf("NewApplicationForm(true) EditAppIdx = %d, want 0", editForm.EditAppIdx)
	}
}

func TestSubEntryForm_TypeValidation(t *testing.T) {
	tests := []struct {
		name    string
		entry   config.SubEntry
		wantErr bool
	}{
		{
			name: "valid_config_entry",
			entry: config.SubEntry{
				Name:   "test",
				Backup: "./test",
				Targets: map[string]string{
					"linux": "~/.config/test",
				},
			},
			wantErr: false,
		},
		{
			name: "config_missing_backup",
			entry: config.SubEntry{
				Name: "test",
				Targets: map[string]string{
					"linux": "~/.config/test",
				},
			},
			wantErr: true,
		},
		{
			name: "missing_name",
			entry: config.SubEntry{
				Name:   "",
				Backup: "./test",
				Targets: map[string]string{
					"linux": "~/.config/test",
				},
			},
			wantErr: true,
		},
		{
			name: "missing_targets",
			entry: config.SubEntry{
				Name:    "test",
				Backup:  "./test",
				Targets: map[string]string{},
			},
			wantErr: true,
		},
		{
			name: "whitespace_only_name",
			entry: config.SubEntry{
				Name:   "   ",
				Backup: "./test",
				Targets: map[string]string{
					"linux": "~/.config/test",
				},
			},
			wantErr: true,
		},
		{
			name: "valid_with_both_targets",
			entry: config.SubEntry{
				Name:   "test",
				Backup: "./test",
				Targets: map[string]string{
					"linux":   "~/.config/test",
					"windows": "~/AppData/Local/test",
				},
			},
			wantErr: false,
		},
		{
			name: "config_whitespace_backup",
			entry: config.SubEntry{
				Name:   "test",
				Backup: "   ",
				Targets: map[string]string{
					"linux": "~/.config/test",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := NewSubEntryForm(tt.entry)
			err := form.Validate()

			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSubEntryForm_Construction(t *testing.T) {
	t.Run("config_entry", func(t *testing.T) {
		entry := config.SubEntry{
			Name:   "test",
			Backup: "./test",
			Sudo:   true,
			Files:  []string{".bashrc", ".profile"},
			Targets: map[string]string{
				"linux":   "~/.config/test",
				"windows": "~/AppData/Local/test",
			},
		}

		form := NewSubEntryForm(entry)

		if form.NameInput.Value() != "test" {
			t.Errorf("NameInput = %q, want %q", form.NameInput.Value(), "test")
		}

		if form.BackupInput.Value() != "./test" {
			t.Errorf("BackupInput = %q, want %q", form.BackupInput.Value(), "./test")
		}

		if !form.IsSudo {
			t.Error("isSudo = false, want true")
		}

		if form.LinuxTargetInput.Value() != "~/.config/test" {
			t.Errorf("LinuxTargetInput = %q, want %q", form.LinuxTargetInput.Value(), "~/.config/test")
		}

		if form.WindowsTargetInput.Value() != "~/AppData/Local/test" {
			t.Errorf("WindowsTargetInput = %q, want %q", form.WindowsTargetInput.Value(), "~/AppData/Local/test")
		}

		if len(form.Files) != 2 {
			t.Errorf("files length = %d, want 2", len(form.Files))
		}
	})
}

func TestApplicationForm_GitPackageLoad(t *testing.T) {
	t.Run("new_form_has_no_git_package", func(t *testing.T) {
		form := NewApplicationForm(config.Application{Name: "test"}, false)
		if form.HasGitPackage {
			t.Error("new form should not have git package")
		}
		if form.GitFieldCursor != -1 {
			t.Errorf("gitFieldCursor = %d, want -1", form.GitFieldCursor)
		}
	})

	t.Run("edit_form_loads_git_package", func(t *testing.T) {
		app := config.Application{
			Name: "test",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"pacman": {PackageName: "neovim"},
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
		form := NewApplicationForm(app, true)

		if !form.HasGitPackage {
			t.Error("form should have git package")
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
			t.Error("GitSudo should be true")
		}
	})

	t.Run("edit_form_without_git_package", func(t *testing.T) {
		app := config.Application{
			Name: "test",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"pacman": {PackageName: "neovim"},
				},
			},
		}
		form := NewApplicationForm(app, true)

		if form.HasGitPackage {
			t.Error("form should not have git package")
		}
		if form.GitURLInput.Value() != "" {
			t.Errorf("GitURLInput = %q, want empty", form.GitURLInput.Value())
		}
	})
}

func TestSaveApplicationForm_GitPackageMerge(t *testing.T) {
	t.Run("save_with_git_package", func(t *testing.T) {
		app := config.Application{
			Name: "test",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"pacman": {PackageName: "neovim"},
					"git": {Git: &config.GitPackage{
						URL:     "https://github.com/user/repo.git",
						Branch:  "main",
						Targets: map[string]string{"linux": "~/.local/share/app"},
						Sudo:    false,
					}},
				},
			},
		}
		form := NewApplicationForm(app, true)
		if !form.HasGitPackage {
			t.Fatal("form should have git package loaded")
		}
		pkg := buildPackageSpec(form.PackageManagers)
		pkg = mergeGitPackage(pkg, form.HasGitPackage, form.GitURLInput, form.GitBranchInput, form.GitLinuxInput, form.GitWindowsInput, form.GitSudo)
		if pkg == nil {
			t.Fatal("package spec should not be nil")
		}
		gitVal, ok := pkg.Managers["git"]
		if !ok {
			t.Fatal("package should have git manager")
		}
		if !gitVal.IsGit() {
			t.Fatal("git manager value should be a git package")
		}
		if gitVal.Git.URL != "https://github.com/user/repo.git" {
			t.Errorf("git URL = %q, want %q", gitVal.Git.URL, "https://github.com/user/repo.git")
		}
		if gitVal.Git.Branch != "main" {
			t.Errorf("git Branch = %q, want %q", gitVal.Git.Branch, "main")
		}
		if gitVal.Git.Targets["linux"] != "~/.local/share/app" {
			t.Errorf("git Linux target = %q, want %q", gitVal.Git.Targets["linux"], "~/.local/share/app")
		}
	})

	t.Run("save_without_git_package", func(t *testing.T) {
		app := config.Application{Name: "test"}
		form := NewApplicationForm(app, false)
		pkg := buildPackageSpec(form.PackageManagers)
		pkg = mergeGitPackage(pkg, form.HasGitPackage, form.GitURLInput, form.GitBranchInput, form.GitLinuxInput, form.GitWindowsInput, form.GitSudo)
		if pkg != nil {
			t.Errorf("package spec should be nil, got %v", pkg)
		}
	})

	t.Run("save_git_only_no_regular_managers", func(t *testing.T) {
		app := config.Application{
			Name: "test",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"git": {Git: &config.GitPackage{
						URL:     "https://github.com/user/repo.git",
						Targets: map[string]string{"linux": "~/.local"},
					}},
				},
			},
		}
		form := NewApplicationForm(app, true)
		pkg := buildPackageSpec(form.PackageManagers)
		pkg = mergeGitPackage(pkg, form.HasGitPackage, form.GitURLInput, form.GitBranchInput, form.GitLinuxInput, form.GitWindowsInput, form.GitSudo)
		if pkg == nil {
			t.Fatal("package spec should not be nil")
		}
		if len(pkg.Managers) != 1 {
			t.Errorf("len(Managers) = %d, want 1", len(pkg.Managers))
		}
		if _, ok := pkg.Managers["git"]; !ok {
			t.Error("should have git manager")
		}
	})
}

func TestApplicationForm_InstallerPackageLoad(t *testing.T) {
	t.Run("new_form_has_no_installer_package", func(t *testing.T) {
		form := NewApplicationForm(config.Application{Name: "test"}, false)
		if form.HasInstallerPackage {
			t.Error("new form should not have installer package")
		}
		if form.InstallerFieldCursor != -1 {
			t.Errorf("installerFieldCursor = %d, want -1", form.InstallerFieldCursor)
		}
	})

	t.Run("edit_form_loads_installer_package", func(t *testing.T) {
		app := config.Application{
			Name: "test",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"pacman": {PackageName: "neovim"},
					"installer": {Installer: &config.InstallerPackage{
						Command: map[string]string{
							"linux":   "curl -fsSL https://example.com/install.sh | sh",
							"windows": "iwr https://example.com/install.ps1 | iex",
						},
						Binary: "mytool",
					}},
				},
			},
		}
		form := NewApplicationForm(app, true)

		if !form.HasInstallerPackage {
			t.Error("form should have installer package")
		}
		if form.InstallerLinuxInput.Value() != "curl -fsSL https://example.com/install.sh | sh" {
			t.Errorf("InstallerLinuxInput = %q, want %q", form.InstallerLinuxInput.Value(), "curl -fsSL https://example.com/install.sh | sh")
		}
		if form.InstallerWindowsInput.Value() != "iwr https://example.com/install.ps1 | iex" {
			t.Errorf("InstallerWindowsInput = %q, want %q", form.InstallerWindowsInput.Value(), "iwr https://example.com/install.ps1 | iex")
		}
		if form.InstallerBinaryInput.Value() != "mytool" {
			t.Errorf("InstallerBinaryInput = %q, want %q", form.InstallerBinaryInput.Value(), "mytool")
		}
	})

	t.Run("edit_form_without_installer_package", func(t *testing.T) {
		app := config.Application{
			Name: "test",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"pacman": {PackageName: "neovim"},
				},
			},
		}
		form := NewApplicationForm(app, true)

		if form.HasInstallerPackage {
			t.Error("form should not have installer package")
		}
		if form.InstallerLinuxInput.Value() != "" {
			t.Errorf("InstallerLinuxInput = %q, want empty", form.InstallerLinuxInput.Value())
		}
	})

	t.Run("edit_form_loads_installer_without_binary", func(t *testing.T) {
		app := config.Application{
			Name: "test",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"installer": {Installer: &config.InstallerPackage{
						Command: map[string]string{
							"linux": "make install",
						},
					}},
				},
			},
		}
		form := NewApplicationForm(app, true)

		if !form.HasInstallerPackage {
			t.Error("form should have installer package")
		}
		if form.InstallerLinuxInput.Value() != "make install" {
			t.Errorf("InstallerLinuxInput = %q, want %q", form.InstallerLinuxInput.Value(), "make install")
		}
		if form.InstallerBinaryInput.Value() != "" {
			t.Errorf("InstallerBinaryInput = %q, want empty", form.InstallerBinaryInput.Value())
		}
	})
}

func TestSaveApplicationForm_InstallerPackageMerge(t *testing.T) {
	t.Run("save_with_installer_package", func(t *testing.T) {
		app := config.Application{
			Name: "test",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"pacman": {PackageName: "neovim"},
					"installer": {Installer: &config.InstallerPackage{
						Command: map[string]string{"linux": "curl -fsSL example.com | sh"},
						Binary:  "mytool",
					}},
				},
			},
		}
		form := NewApplicationForm(app, true)
		if !form.HasInstallerPackage {
			t.Fatal("form should have installer package loaded")
		}
		pkg := buildPackageSpec(form.PackageManagers)
		pkg = mergeGitPackage(pkg, form.HasGitPackage, form.GitURLInput, form.GitBranchInput, form.GitLinuxInput, form.GitWindowsInput, form.GitSudo)
		pkg = mergeInstallerPackage(pkg, form.HasInstallerPackage, form.InstallerLinuxInput, form.InstallerWindowsInput, form.InstallerBinaryInput)
		if pkg == nil {
			t.Fatal("package spec should not be nil")
		}
		installerVal, ok := pkg.Managers["installer"]
		if !ok {
			t.Fatal("package should have installer manager")
		}
		if !installerVal.IsInstaller() {
			t.Fatal("installer manager value should be an installer package")
		}
		if installerVal.Installer.Command["linux"] != "curl -fsSL example.com | sh" {
			t.Errorf("installer Command[linux] = %q, want %q", installerVal.Installer.Command["linux"], "curl -fsSL example.com | sh")
		}
		if installerVal.Installer.Binary != "mytool" {
			t.Errorf("installer Binary = %q, want %q", installerVal.Installer.Binary, "mytool")
		}
	})

	t.Run("save_without_installer_package", func(t *testing.T) {
		app := config.Application{Name: "test"}
		form := NewApplicationForm(app, false)
		pkg := buildPackageSpec(form.PackageManagers)
		pkg = mergeInstallerPackage(pkg, form.HasInstallerPackage, form.InstallerLinuxInput, form.InstallerWindowsInput, form.InstallerBinaryInput)
		if pkg != nil {
			t.Errorf("package spec should be nil, got %v", pkg)
		}
	})

	t.Run("save_installer_only_no_regular_managers", func(t *testing.T) {
		app := config.Application{
			Name: "test",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"installer": {Installer: &config.InstallerPackage{
						Command: map[string]string{"linux": "make install"},
					}},
				},
			},
		}
		form := NewApplicationForm(app, true)
		pkg := buildPackageSpec(form.PackageManagers)
		pkg = mergeInstallerPackage(pkg, form.HasInstallerPackage, form.InstallerLinuxInput, form.InstallerWindowsInput, form.InstallerBinaryInput)
		if pkg == nil {
			t.Fatal("package spec should not be nil")
		}
		if len(pkg.Managers) != 1 {
			t.Errorf("len(Managers) = %d, want 1", len(pkg.Managers))
		}
		if _, ok := pkg.Managers["installer"]; !ok {
			t.Error("should have installer manager")
		}
	})

	t.Run("save_with_both_git_and_installer", func(t *testing.T) {
		app := config.Application{
			Name: "test",
			Package: &config.EntryPackage{
				Managers: map[string]config.ManagerValue{
					"git": {Git: &config.GitPackage{
						URL:     "https://github.com/user/repo.git",
						Targets: map[string]string{"linux": "~/.local"},
					}},
					"installer": {Installer: &config.InstallerPackage{
						Command: map[string]string{"linux": "make install"},
						Binary:  "mytool",
					}},
				},
			},
		}
		form := NewApplicationForm(app, true)
		pkg := buildPackageSpec(form.PackageManagers)
		pkg = mergeGitPackage(pkg, form.HasGitPackage, form.GitURLInput, form.GitBranchInput, form.GitLinuxInput, form.GitWindowsInput, form.GitSudo)
		pkg = mergeInstallerPackage(pkg, form.HasInstallerPackage, form.InstallerLinuxInput, form.InstallerWindowsInput, form.InstallerBinaryInput)
		if pkg == nil {
			t.Fatal("package spec should not be nil")
		}
		if len(pkg.Managers) != 2 {
			t.Errorf("len(Managers) = %d, want 2", len(pkg.Managers))
		}
		if _, ok := pkg.Managers["git"]; !ok {
			t.Error("should have git manager")
		}
		if _, ok := pkg.Managers["installer"]; !ok {
			t.Error("should have installer manager")
		}
	})
}

func TestBuildPackageSpec(t *testing.T) {
	t.Run("empty_managers", func(t *testing.T) {
		result := buildPackageSpec(map[string]string{})
		if result != nil {
			t.Errorf("buildPackageSpec(empty) = %v, want nil", result)
		}
	})

	t.Run("nil_managers", func(t *testing.T) {
		result := buildPackageSpec(nil)
		if result != nil {
			t.Errorf("buildPackageSpec(nil) = %v, want nil", result)
		}
	})

	t.Run("single_manager", func(t *testing.T) {
		managers := map[string]string{
			"pacman": "neovim",
		}
		result := buildPackageSpec(managers)

		if result == nil {
			t.Fatal("buildPackageSpec returned nil, want non-nil")
		}

		if len(result.Managers) != 1 {
			t.Errorf("len(Managers) = %d, want 1", len(result.Managers))
		}

		if result.Managers["pacman"].PackageName != "neovim" {
			t.Errorf("Managers[pacman] = %q, want %q", result.Managers["pacman"].PackageName, "neovim")
		}
	})

	t.Run("multiple_managers", func(t *testing.T) {
		managers := map[string]string{
			"pacman": "neovim",
			"apt":    "neovim",
			"brew":   "neovim",
		}
		result := buildPackageSpec(managers)

		if result == nil {
			t.Fatal("buildPackageSpec returned nil, want non-nil")
		}

		if len(result.Managers) != 3 {
			t.Errorf("len(Managers) = %d, want 3", len(result.Managers))
		}

		for mgr, pkg := range managers {
			if result.Managers[mgr].PackageName != pkg {
				t.Errorf("Managers[%s] = %q, want %q", mgr, result.Managers[mgr].PackageName, pkg)
			}
		}
	})
}
