package tui

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	tmpl "github.com/AntoineGS/tidydots/internal/template"
	"github.com/AntoineGS/tidydots/internal/tui/forms"
)

func TestApplicationCommandEditUsesDecodedEnd(t *testing.T) {
	const original = "printf '\\t'\n\twith-tab"
	m := NewModel(&config.Config{Version: 3, Applications: []config.Application{{Name: "tool", Package: &config.EntryPackage{Custom: map[string]string{"linux": original}}}}}, &platform.Platform{OS: platform.OSLinux}, false)
	m.Applications = []ApplicationItem{{Application: m.Config.Applications[0]}}
	m.ConfigPath = t.TempDir() + "/tidydots.yaml"
	m.initApplicationForm(0)
	m.applicationForm.FocusIndex = 2
	m.applicationForm.PackagesCursor = len(displayPackageManagers) + 2
	m.applicationForm.CustomFieldCursor = 0
	updated, _ := m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Text: "!", Code: '!'})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if got := m.applicationForm.CustomLinuxInput.Value(); got != original+"!" {
		t.Fatalf("edited command = %q, want %q", got, original+"!")
	}
	if err := m.saveApplicationForm(); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	reloaded, err := config.Load(m.ConfigPath)
	if err != nil || reloaded.Applications[0].Package.Custom["linux"] != original+"!" {
		t.Fatalf("reloaded command = %q, err=%v", reloaded.Applications[0].Package.Custom["linux"], err)
	}
}

func TestInstallerBinaryEditConfirmAndCancel(t *testing.T) {
	m := NewModel(&config.Config{Version: 3}, &platform.Platform{OS: platform.OSLinux}, false)
	m.initApplicationForm(-1)
	m.applicationForm.HasInstallerPackage = true
	m.applicationForm.InstallerBinaryInput.SetValue("old-bin")
	m.applicationForm.FocusIndex = 2
	m.applicationForm.PackagesCursor = len(displayPackageManagers) + 1
	m.applicationForm.InstallerFieldCursor = InstallerFieldBinary
	updated, _ := m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if m.applicationForm.InstallerBinaryInput.Value() != "old-bin" {
		t.Fatalf("cancel did not restore binary: %q", m.applicationForm.InstallerBinaryInput.Value())
	}
	m.applicationForm.InstallerFieldCursor = InstallerFieldBinary
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Text: "!", Code: '!'})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.applicationForm.InstallerBinaryInput.Value() != "old-bin!" {
		t.Fatalf("confirm did not retain binary: %q", m.applicationForm.InstallerBinaryInput.Value())
	}
}

func TestBuildHostnameWhen(t *testing.T) {
	tests := []struct {
		hosts []string
		want  string
	}{
		{nil, ""},
		{[]string{"desktop"}, `{{ eq .Hostname "desktop" }}`},
		{[]string{"desktop", "laptop"}, `{{ or (eq .Hostname "desktop") (eq .Hostname "laptop") }}`},
		{[]string{`desk"top`}, `{{ eq .Hostname "desk\"top" }}`},
	}
	for _, tt := range tests {
		if got := buildHostnameWhen(tt.hosts); got != tt.want {
			t.Errorf("buildHostnameWhen(%v) = %q, want %q", tt.hosts, got, tt.want)
		}
	}
}

func TestApplicationWhenHostnameChooser(t *testing.T) {
	m := NewModel(&config.Config{Version: 3}, &platform.Platform{OS: platform.OSLinux}, false)
	m.HostnameChoices = []string{"desktop", "laptop"}
	m.initApplicationForm(-1)
	m.applicationForm.FocusIndex = 3

	updated, _ := m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.applicationForm.WhenMode != forms.WhenModeChooser {
		t.Fatalf("WhenMode = %v, want chooser", m.applicationForm.WhenMode)
	}
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if got := m.applicationForm.WhenInput.Value(); got != `{{ or (eq .Hostname "desktop") (eq .Hostname "laptop") }}` {
		t.Errorf("WhenInput = %q, want hostname expression", got)
	}

	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if got := m.applicationForm.WhenInput.Value(); got != `{{ or (eq .Hostname "desktop") (eq .Hostname "laptop") }}` {
		t.Errorf("cancel changed WhenInput to %q", got)
	}
}

func TestApplicationWhenHostnameChooserSavePersistsSelection(t *testing.T) {
	cfg := &config.Config{
		Version:   3,
		Hostnames: []string{"desktop"},
		Applications: []config.Application{{
			Name: "shell",
			Package: &config.EntryPackage{Managers: map[string]config.ManagerValue{
				"apt": {PackageName: "shell"},
			}},
		}},
	}
	configPath := t.TempDir() + "/tidydots.yaml"
	if err := config.Save(cfg, configPath); err != nil {
		t.Fatalf("config.Save() error = %v", err)
	}

	m := NewModel(cfg, &platform.Platform{OS: platform.OSLinux}, false)
	m.ConfigPath = configPath
	m.HostnameChoices = append([]string(nil), cfg.Hostnames...)
	m.initApplicationForm(0)
	m.applicationForm.FocusIndex = 3

	updated, _ := m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m = updated.(Model)

	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if got, want := reloaded.Applications[0].When, `{{ eq .Hostname "desktop" }}`; got != want {
		t.Errorf("application when = %q, want %q", got, want)
	}
}

func TestApplicationWhenHostnameChooserEscapeRestoresDistinctExpression(t *testing.T) {
	const original = `{{ eq .OS "linux" }}`
	m := NewModel(&config.Config{Version: 3}, &platform.Platform{OS: platform.OSLinux}, false)
	m.HostnameChoices = []string{"desktop", "laptop"}
	m.initApplicationForm(-1)
	m.applicationForm.WhenInput.SetValue(original)
	m.applicationForm.FocusIndex = 3

	updated, _ := m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)

	if got := m.applicationForm.WhenInput.Value(); got != original {
		t.Errorf("WhenInput after chooser cancel = %q, want original %q", got, original)
	}
}

func TestApplicationWhenHostnameChooserClearsOnEmptySelection(t *testing.T) {
	m := NewModel(&config.Config{Version: 3}, &platform.Platform{OS: platform.OSLinux}, false)
	m.HostnameChoices = []string{"desktop"}
	m.initApplicationForm(-1)
	m.applicationForm.WhenInput.SetValue(`{{ eq .OS "linux" }}`)
	m.applicationForm.FocusIndex = 3
	updated, _ := m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.applicationForm.WhenMode != forms.WhenModeNone || m.applicationForm.WhenInput.Value() != "" {
		t.Fatalf("empty chooser selection did not clear/close: mode=%v value=%q", m.applicationForm.WhenMode, m.applicationForm.WhenInput.Value())
	}
}

func TestSubEntryWhenHostnameChooserAndCancel(t *testing.T) {
	m := NewModel(&config.Config{Version: 3, Applications: []config.Application{{Name: "tool"}}}, &platform.Platform{OS: platform.OSLinux}, false)
	m.Applications = []ApplicationItem{{Application: m.Config.Applications[0]}}
	m.HostnameChoices = []string{"desktop", "laptop"}
	m.initSubEntryForm(0, -1)
	m.subEntryForm.FocusIndex = 2

	updated, _ := m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if got := m.subEntryForm.WhenInput.Value(); got != `{{ eq .Hostname "desktop" }}` {
		t.Fatalf("WhenInput = %q, want hostname expression", got)
	}

	m.subEntryForm.OriginalValue = m.subEntryForm.WhenInput.Value()
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if got := m.subEntryForm.WhenInput.Value(); got != `{{ eq .Hostname "desktop" }}` {
		t.Fatalf("chooser cancel changed WhenInput to %q", got)
	}
}

func TestSubEntryWhenHostnameChooserSavePersistsSelection(t *testing.T) {
	cfg := &config.Config{
		Version:   3,
		Hostnames: []string{"desktop"},
		Applications: []config.Application{{
			Name: "shell",
			Entries: []config.SubEntry{{
				Name:    "config",
				Backup:  "./shell",
				Targets: map[string]string{"linux": "~/.config/shell"},
			}},
		}},
	}
	configPath := t.TempDir() + "/tidydots.yaml"
	if err := config.Save(cfg, configPath); err != nil {
		t.Fatalf("config.Save() error = %v", err)
	}

	m := NewModel(cfg, &platform.Platform{OS: platform.OSLinux}, false)
	m.ConfigPath = configPath
	m.HostnameChoices = append([]string(nil), cfg.Hostnames...)
	m.initSubEntryForm(0, 0)
	m.subEntryForm.FocusIndex = 2

	updated, _ := m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeySpace})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	m = updated.(Model)

	reloaded, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("config.Load() error = %v", err)
	}
	if got, want := reloaded.Applications[0].Entries[0].When, `{{ eq .Hostname "desktop" }}`; got != want {
		t.Errorf("entry when = %q, want %q", got, want)
	}
}

func TestSubEntryWhenHostnameChooserSelectsMultipleInConfiguredOrder(t *testing.T) {
	m := NewModel(&config.Config{Version: 3, Applications: []config.Application{{Name: "tool"}}}, &platform.Platform{OS: platform.OSLinux}, false)
	m.Applications = []ApplicationItem{{Application: m.Config.Applications[0]}}
	m.HostnameChoices = []string{"omarchbook", "desktop", "laptop"}
	m.initSubEntryForm(0, -1)
	m.subEntryForm.FocusIndex = 2
	updated, _ := m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	for _, keyMsg := range []tea.KeyPressMsg{
		{Code: tea.KeySpace},
		{Code: tea.KeyDown},
		{Code: tea.KeySpace},
		{Code: tea.KeyEnter},
	} {
		updated, _ = m.updateSubEntryForm(keyMsg)
		m = updated.(Model)
	}
	want := `{{ or (eq .Hostname "omarchbook") (eq .Hostname "desktop") }}`
	if got := m.subEntryForm.WhenInput.Value(); got != want {
		t.Fatalf("WhenInput = %q, want %q", got, want)
	}
}

func TestSubEntryWhenChooserTypeExpressionEntersManualEdit(t *testing.T) {
	m := NewModel(&config.Config{Version: 3, Applications: []config.Application{{Name: "tool"}}}, &platform.Platform{OS: platform.OSLinux}, false)
	m.Applications = []ApplicationItem{{Application: m.Config.Applications[0]}}
	m.HostnameChoices = []string{"desktop"}
	m.initSubEntryForm(0, -1)
	m.subEntryForm.FocusIndex = 2
	updated, _ := m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.subEntryForm.EditingWhen || m.subEntryForm.WhenMode != forms.WhenModeNone {
		t.Fatalf("type expression did not enter manual edit: editing=%v mode=%v", m.subEntryForm.EditingWhen, m.subEntryForm.WhenMode)
	}
}

func TestSubEntryWhenChooserHelpUsesChooserBindings(t *testing.T) {
	m := NewModel(&config.Config{Version: 3, Applications: []config.Application{{Name: "tool"}}}, &platform.Platform{OS: platform.OSLinux}, false)
	m.Applications = []ApplicationItem{{Application: m.Config.Applications[0]}}
	m.HostnameChoices = []string{"desktop"}
	m.initSubEntryForm(0, -1)
	m.subEntryForm.FocusIndex = 2
	m.startSubEntryWhenChooser()
	help := stripAnsiCodes(m.renderSubEntryFormHelp())
	for _, want := range []string{"↑/k", "↓/j", "space", "enter/e", "esc", "apply", "save"} {
		if !strings.Contains(help, want) {
			t.Errorf("chooser help %q does not contain %q", help, want)
		}
	}
	if strings.Contains(help, "edit") {
		t.Errorf("chooser help should not describe selection as editing: %q", help)
	}
}

func TestSaveSubEntryAcceptsSproutWhenWithRuntimeRenderer(t *testing.T) {
	m := NewModel(&config.Config{Version: 3, Applications: []config.Application{{Name: "tool"}}}, &platform.Platform{OS: OSLinux}, false)
	m.ConfigPath = t.TempDir() + "/tidydots.yaml"
	m.Renderer = tmpl.NewEngine(tmpl.NewContextFromPlatform(m.Platform))
	m.Applications = []ApplicationItem{{Application: m.Config.Applications[0]}}
	m.initSubEntryForm(0, -1)
	m.subEntryForm.NameInput.SetValue("sprout-entry")
	m.subEntryForm.BackupInput.SetValue("./entry")
	m.subEntryForm.LinuxTargetInput.SetValue("~/.entry")
	m.subEntryForm.WhenInput.SetValue(`{{ "hello" | toUpper }}`)
	if err := m.saveSubEntryForm(); err != nil {
		t.Fatalf("valid Sprout expression rejected: %v", err)
	}
	if got := m.Config.Applications[0].Entries[0].When; got != `{{ "hello" | toUpper }}` {
		t.Fatalf("stored When = %q, want Sprout expression", got)
	}
}

func TestSubEntryWhenTextEditCancelRestoresOriginal(t *testing.T) {
	const original = `{{ eq .OS "linux" }}`
	m := NewModel(&config.Config{Version: 3, Applications: []config.Application{{Name: "tool"}}}, &platform.Platform{OS: platform.OSLinux}, false)
	m.Applications = []ApplicationItem{{Application: m.Config.Applications[0]}}
	m.initSubEntryForm(0, -1)
	m.subEntryForm.WhenInput.SetValue(original)
	m.subEntryForm.FocusIndex = 2
	updated, _ := m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = updated.(Model)
	updated, _ = m.updateSubEntryForm(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if got := m.subEntryForm.WhenInput.Value(); got != original {
		t.Fatalf("text cancel changed WhenInput to %q", got)
	}
}

func TestSaveSubEntryRejectsMalformedWhenWithoutMutation(t *testing.T) {
	configPath := t.TempDir() + "/tidydots.yaml"
	original := &config.Config{Version: 3, Applications: []config.Application{{Name: "tool", Entries: []config.SubEntry{{Name: "entry", Backup: "./entry", Targets: map[string]string{"linux": "~/.entry"}}}}}}
	if err := config.Save(original, configPath); err != nil {
		t.Fatalf("initial config save failed: %v", err)
	}
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read initial config: %v", err)
	}
	m := NewModel(original, &platform.Platform{OS: platform.OSLinux}, false)
	m.ConfigPath = configPath
	m.Renderer = whenSaveRenderer{}
	m.Applications = []ApplicationItem{{Application: original.Applications[0], SubItems: []SubEntryItem{{SubEntry: original.Applications[0].Entries[0]}}}}
	m.initSubEntryForm(0, 0)
	m.subEntryForm.WhenInput.SetValue("{{ if }}")
	if err := m.saveSubEntryForm(); err == nil {
		t.Fatal("malformed expression was saved")
	}
	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read resulting config: %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("malformed save changed YAML")
	}
	if got := m.Config.Applications[0].Entries[0].When; got != "" {
		t.Fatalf("malformed save mutated in-memory config: When=%q", got)
	}
}

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

func TestApplicationForm_DeletePackageMethodSections(t *testing.T) {
	m := NewModel(&config.Config{Version: 3}, &platform.Platform{OS: platform.OSLinux}, false)
	m.initApplicationForm(-1)
	m.applicationForm.FocusIndex = 2
	m.applicationForm.HasCustomPackage = true
	m.applicationForm.CustomLinuxInput.SetValue("make install")
	m.applicationForm.HasURLPackage = true
	m.applicationForm.URLLinuxInput.SetValue("https://example.com/tool")
	m.applicationForm.URLLinuxCommandInput.SetValue("install {file}")

	customIdx := len(displayPackageManagers) + 2
	m.applicationForm.PackagesCursor = customIdx
	updated, _ := m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyDelete})
	m = updated.(Model)
	if m.applicationForm.HasCustomPackage || m.applicationForm.CustomLinuxInput.Value() != "" {
		t.Error("deleting custom package should clear its section")
	}

	urlIdx := len(displayPackageManagers) + 3
	m.applicationForm.PackagesCursor = urlIdx
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyDelete})
	m = updated.(Model)
	if m.applicationForm.HasURLPackage || m.applicationForm.URLLinuxInput.Value() != "" || m.applicationForm.URLLinuxCommandInput.Value() != "" {
		t.Error("deleting URL package should clear its section")
	}
}

func TestApplicationForm_CustomPackageInteraction(t *testing.T) {
	m := NewModel(&config.Config{Version: 3}, &platform.Platform{OS: platform.OSLinux}, false)
	m.initApplicationForm(-1)
	m.applicationForm.FocusIndex = 2
	m.applicationForm.PackagesCursor = len(displayPackageManagers) + 2
	m.applicationForm.HasCustomPackage = true
	m.applicationForm.CustomFieldCursor = 0
	m.applicationForm.CustomLinuxInput.SetValue("old command")

	updated, _ := m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if !m.applicationForm.EditingCustomField {
		t.Fatal("enter should start custom field editing")
	}
	m.applicationForm.CustomLinuxInput.SetValue("canceled command")
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if got := m.applicationForm.CustomLinuxInput.Value(); got != "old command" {
		t.Errorf("escape restored %q, want %q", got, "old command")
	}

	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	m.applicationForm.CustomLinuxInput.SetValue("confirmed command")
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.applicationForm.EditingCustomField || m.applicationForm.CustomLinuxInput.Value() != "confirmed command" {
		t.Error("enter should retain the confirmed custom command")
	}

	m.applicationForm.CustomFieldCursor = 1
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = updated.(Model)
	if m.applicationForm.FocusIndex != 1 || m.applicationForm.CustomFieldCursor != -1 {
		t.Errorf("shift-tab from custom section = focus %d, cursor %d; want focus 1, cursor -1", m.applicationForm.FocusIndex, m.applicationForm.CustomFieldCursor)
	}

	m.applicationForm.FocusIndex = 2
	m.applicationForm.PackagesCursor = len(displayPackageManagers) + 2
	m.applicationForm.CustomFieldCursor = 1
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	if m.applicationForm.FocusIndex != 3 || m.applicationForm.CustomFieldCursor != -1 || m.applicationForm.PackagesCursor != 0 {
		t.Errorf("tab from custom section = focus %d, cursor %d, package cursor %d; want focus 3, cursor -1, package cursor 0", m.applicationForm.FocusIndex, m.applicationForm.CustomFieldCursor, m.applicationForm.PackagesCursor)
	}
}

func TestApplicationForm_URLPackageInteraction(t *testing.T) {
	m := NewModel(&config.Config{Version: 3}, &platform.Platform{OS: platform.OSLinux}, false)
	m.initApplicationForm(-1)
	m.applicationForm.FocusIndex = 2
	m.applicationForm.PackagesCursor = len(displayPackageManagers) + 3
	m.applicationForm.HasURLPackage = true
	m.applicationForm.URLFieldCursor = 0
	m.applicationForm.URLLinuxInput.SetValue("https://old.example/tool")

	updated, _ := m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	m.applicationForm.URLLinuxInput.SetValue("https://cancelled.example/tool")
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = updated.(Model)
	if got := m.applicationForm.URLLinuxInput.Value(); got != "https://old.example/tool" {
		t.Errorf("escape restored %q, want original URL", got)
	}

	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	m.applicationForm.URLLinuxInput.SetValue("https://confirmed.example/tool")
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)
	if m.applicationForm.EditingURLField || m.applicationForm.URLLinuxInput.Value() != "https://confirmed.example/tool" {
		t.Error("enter should retain the confirmed URL")
	}

	m.applicationForm.URLFieldCursor = 3
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(Model)
	if m.applicationForm.FocusIndex != 3 || m.applicationForm.URLFieldCursor != -1 || m.applicationForm.PackagesCursor != 0 {
		t.Errorf("tab from URL section = focus %d, cursor %d, package cursor %d; want focus 3, cursor -1, package cursor 0", m.applicationForm.FocusIndex, m.applicationForm.URLFieldCursor, m.applicationForm.PackagesCursor)
	}

	m.applicationForm.FocusIndex = 2
	m.applicationForm.PackagesCursor = len(displayPackageManagers) + 3
	m.applicationForm.URLFieldCursor = 0
	updated, _ = m.updateApplicationForm(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = updated.(Model)
	if m.applicationForm.FocusIndex != 1 || m.applicationForm.URLFieldCursor != -1 {
		t.Errorf("shift-tab from URL section = focus %d, cursor %d; want focus 1, cursor -1", m.applicationForm.FocusIndex, m.applicationForm.URLFieldCursor)
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
