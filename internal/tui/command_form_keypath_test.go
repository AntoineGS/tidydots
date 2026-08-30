package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
)

func TestCommandFormKeypathsPreserveEditedCommands(t *testing.T) {
	const original = "printf '\\t'\n\twith-tab"
	tests := []struct {
		name   string
		pkg    func(string) *config.EntryPackage
		cursor func(*ApplicationForm)
		value  func(*config.EntryPackage) string
	}{
		{
			name: "installer command",
			pkg: func(command string) *config.EntryPackage {
				return &config.EntryPackage{Managers: map[string]config.ManagerValue{TypeInstaller: {Installer: &config.InstallerPackage{Command: map[string]string{OSLinux: command}, Binary: "tool"}}}}
			},
			cursor: func(f *ApplicationForm) { f.InstallerFieldCursor = InstallerFieldLinux },
			value:  func(pkg *config.EntryPackage) string { return pkg.Managers[TypeInstaller].Installer.Command[OSLinux] },
		},
		{
			name: "URL download command",
			pkg: func(command string) *config.EntryPackage {
				return &config.EntryPackage{URL: map[string]config.URLInstallSpec{OSLinux: {URL: "https://example.test/tool", Command: command}}}
			},
			cursor: func(f *ApplicationForm) { f.URLFieldCursor = 1 },
			value:  func(pkg *config.EntryPackage) string { return pkg.URL[OSLinux].Command },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := config.Application{Name: "tool", Package: tt.pkg(original)}
			m := NewModel(&config.Config{Version: 3, Applications: []config.Application{app}}, &platform.Platform{OS: platform.OSLinux}, false)
			m.Applications = []ApplicationItem{{Application: app}}
			m.ConfigPath = t.TempDir() + "/tidydots.yaml"
			m.initApplicationForm(0)
			m.applicationForm.FocusIndex = 2
			tt.cursor(m.applicationForm)
			if tt.name == "installer command" {
				m.applicationForm.PackagesCursor = len(displayPackageManagers) + 1
			} else {
				m.applicationForm.PackagesCursor = len(displayPackageManagers) + 3
			}
			updated, _ := m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = updated.(Model)
			updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Text: "!", Code: '!'})
			m = updated.(Model)
			updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = updated.(Model)
			if err := m.saveApplicationForm(); err != nil {
				t.Fatalf("saveApplicationForm() error = %v", err)
			}
			reloaded, err := config.Load(m.ConfigPath)
			if err != nil {
				t.Fatalf("config reload error = %v", err)
			}
			if got := tt.value(reloaded.Applications[0].Package); got != original+"!" {
				t.Fatalf("reloaded command = %q, want %q", got, original+"!")
			}
		})
	}
}

func TestSetupCommandKeypathPreservesEditedCommand(t *testing.T) {
	const original = "check \\t\n\twith-tab"
	entry := config.SubEntry{Name: "setup", Check: map[string]string{OSLinux: original}, Run: map[string]string{OSLinux: original}}
	app := config.Application{Name: "tool", Entries: []config.SubEntry{entry}}
	m := NewModel(&config.Config{Version: 3, Applications: []config.Application{app}}, &platform.Platform{OS: platform.OSLinux}, false)
	m.Applications = []ApplicationItem{{Application: app, SubItems: []SubEntryItem{{SubEntry: entry}}}}
	m.ConfigPath = t.TempDir() + "/tidydots.yaml"
	m.initSubEntryForm(0, 0)
	m.subEntryForm.FocusIndex = 2
	updated, _ := m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Text: "!", Code: '!'})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if err := m.saveSubEntryForm(); err != nil {
		t.Fatalf("saveSubEntryForm() error = %v", err)
	}
	reloaded, err := config.Load(m.ConfigPath)
	if err != nil || reloaded.Applications[0].Entries[0].Check[OSLinux] != original+"!" {
		t.Fatalf("reloaded setup command = %q, err=%v", reloaded.Applications[0].Entries[0].Check[OSLinux], err)
	}
}
