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
