package config

import (
	"errors"
	"strings"
	"testing"
)

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		setup      func() *ValidationErrors
		name       string
		wantMsg    string
		wantErrors bool
	}{
		{
			name: "empty_validation_errors",
			setup: func() *ValidationErrors {
				return &ValidationErrors{}
			},
			wantErrors: false,
			wantMsg:    "no validation errors",
		},
		{
			name: "multiple_errors",
			setup: func() *ValidationErrors {
				ve := &ValidationErrors{}
				ve.Add(errors.New("error 1"))
				ve.Add(errors.New("error 2"))
				return ve
			},
			wantErrors: true,
			wantMsg:    "error 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ve := tt.setup()

			if ve.HasErrors() != tt.wantErrors {
				t.Errorf("HasErrors() = %v, want %v", ve.HasErrors(), tt.wantErrors)
			}

			msg := ve.Error()
			if !strings.Contains(msg, tt.wantMsg) {
				t.Errorf("Error() = %q, want to contain %q", msg, tt.wantMsg)
			}
		})
	}
}

func TestFieldError(t *testing.T) {
	baseErr := errors.New("invalid format")
	fieldErr := NewFieldError("myapp", "repo", "not-a-url", baseErr)

	var fe *FieldError
	if !errors.As(fieldErr, &fe) {
		t.Fatal("Should be FieldError type")
	}

	if fe.Entry != "myapp" {
		t.Errorf("Entry = %q, want %q", fe.Entry, "myapp")
	}

	if fe.Field != "repo" {
		t.Errorf("Field = %q, want %q", fe.Field, "repo")
	}

	if fe.Value != "not-a-url" {
		t.Errorf("Value = %q, want %q", fe.Value, "not-a-url")
	}

	if !errors.Is(fieldErr, baseErr) {
		t.Error("FieldError should wrap underlying error")
	}
}

func TestConfigSentinelErrors(t *testing.T) {
	tests := []struct {
		err  error
		want error
		name string
	}{
		{
			name: "unsupported_version",
			err:  ErrUnsupportedVersion,
			want: ErrUnsupportedVersion,
		},
		{
			name: "invalid_config",
			err:  ErrInvalidConfig,
			want: ErrInvalidConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, tt.want) {
				t.Errorf("errors.Is() = false, want true")
			}
		})
	}
}
