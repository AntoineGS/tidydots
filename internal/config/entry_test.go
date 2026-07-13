package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSubEntry_EffectiveMethod_DefaultsToSymlink(t *testing.T) {
	t.Parallel()
	var e SubEntry // Method == ""
	if got := e.EffectiveMethod(); got != MethodSymlink {
		t.Errorf("EffectiveMethod() = %q, want %q", got, MethodSymlink)
	}
	if e.IsCopy() {
		t.Error("IsCopy() = true for default entry, want false")
	}
}

func TestSubEntry_IsCopy_WhenMethodCopy(t *testing.T) {
	t.Parallel()
	e := SubEntry{Method: MethodCopy}
	if !e.IsCopy() {
		t.Error("IsCopy() = false for method: copy, want true")
	}
}

func TestSubEntry_Method_UnmarshalsFromYAML(t *testing.T) {
	t.Parallel()
	var e SubEntry
	if err := yaml.Unmarshal([]byte("name: x\nmethod: copy\n"), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Method != MethodCopy {
		t.Errorf("Method = %q, want %q", e.Method, MethodCopy)
	}
}

func TestSubEntry_SetupFields(t *testing.T) {
	const data = `
name: vicinae
entries:
  - targets:
      linux: ~/.config/vicinae
    name: config
    backup: ./Linux/vicinae
  - name: enable-service
    check:
      linux: systemctl --user is-enabled --quiet vicinae.service
    run:
      linux: systemctl --user enable --now vicinae.service
`

	var app Application
	if err := yaml.Unmarshal([]byte(data), &app); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(app.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(app.Entries))
	}

	cfgEntry, setupEntry := app.Entries[0], app.Entries[1]

	if cfgEntry.IsSetup() {
		t.Error("config entry reported IsSetup() = true")
	}

	if !setupEntry.IsSetup() {
		t.Error("setup entry reported IsSetup() = false")
	}

	if setupEntry.IsConfig() {
		t.Error("setup entry reported IsConfig() = true; it has no backup")
	}

	if got, want := setupEntry.GetRun("linux"), "systemctl --user enable --now vicinae.service"; got != want {
		t.Errorf("GetRun(linux) = %q, want %q", got, want)
	}

	if got, want := setupEntry.GetCheck("linux"), "systemctl --user is-enabled --quiet vicinae.service"; got != want {
		t.Errorf("GetCheck(linux) = %q, want %q", got, want)
	}

	// The OS map is the platform gate: an absent key means "does not apply here".
	if got := setupEntry.GetRun("windows"); got != "" {
		t.Errorf("GetRun(windows) = %q, want \"\"", got)
	}
}
