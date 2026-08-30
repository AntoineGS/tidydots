package tui

import (
	"errors"
	"testing"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	tmpl "github.com/AntoineGS/tidydots/internal/template"
)

type whenSaveRenderer struct{}

func (whenSaveRenderer) RenderString(_, expression string) (string, error) {
	if expression == "{{ if }}" {
		return "", errors.New("parse error")
	}
	return "true", nil
}

func TestSaveApplicationValidatesWhenWithRuntimeRenderer(t *testing.T) {
	configPath := t.TempDir() + "/tidydots.yaml"
	m := NewModel(&config.Config{Version: 3}, &platform.Platform{OS: platform.OSLinux}, false)
	m.ConfigPath = configPath
	m.Renderer = tmpl.NewEngine(tmpl.NewContextFromPlatform(m.Platform))
	m.initApplicationForm(-1)
	m.applicationForm.NameInput.SetValue("sprout")
	m.applicationForm.WhenInput.SetValue(`{{ "hello" | toUpper }}`)
	if err := m.saveApplicationForm(); err != nil {
		t.Fatalf("valid Sprout expression rejected: %v", err)
	}
}

func TestSaveApplicationRejectsMalformedWhenBeforeWriting(t *testing.T) {
	configPath := t.TempDir() + "/tidydots.yaml"
	m := NewModel(&config.Config{Version: 3}, &platform.Platform{OS: platform.OSLinux}, false)
	m.ConfigPath = configPath
	m.Renderer = whenSaveRenderer{}
	m.initApplicationForm(-1)
	m.applicationForm.NameInput.SetValue("broken")
	m.applicationForm.WhenInput.SetValue("{{ if }}")
	if err := m.saveApplicationForm(); err == nil {
		t.Fatal("malformed expression was saved")
	}
}

var _ config.PathRenderer = whenSaveRenderer{}
