package config

import (
	"errors"
	"testing"
)

func TestValidatePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid absolute path",
			path:    "/home/user/.config",
			wantErr: false,
		},
		{
			name:    "valid tilde path",
			path:    "~/.config",
			wantErr: false,
		},
		{
			name:    "valid relative path",
			path:    "./relative",
			wantErr: false,
		},
		{
			name:    "empty string is valid",
			path:    "",
			wantErr: false,
		},
		{
			name:    "path with null byte is invalid",
			path:    "/home/user/\x00config",
			wantErr: true,
		},
		{
			name:    "path traversal with leading dotdot",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal embedded in middle",
			path:    "config/../../secret",
			wantErr: true,
		},
		{
			name:    "path traversal with just dotdot",
			path:    "..",
			wantErr: true,
		},
		{
			name:    "valid path with single dot",
			path:    "./config",
			wantErr: false,
		},
		{
			name:    "valid deep path",
			path:    "/home/user/.config/nvim/init.lua",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		config    *Config
		wantCount int // number of expected errors
		checkErrs func(t *testing.T, errs []error)
	}{
		{
			name: "valid v3 config with unique names",
			config: &Config{
				Version: 3,
				Applications: []Application{
					{
						Name: "nvim",
						Entries: []SubEntry{
							{Name: "nvim-config", Backup: "./nvim"},
						},
					},
					{
						Name: "zsh",
						Entries: []SubEntry{
							{Name: "zshrc", Backup: "./zsh"},
						},
					},
				},
			},
			wantCount: 0,
		},
		{
			name: "valid config with no applications",
			config: &Config{
				Version: 3,
			},
			wantCount: 0,
		},
		{
			name: "invalid version",
			config: &Config{
				Version: 2,
			},
			wantCount: 1,
			checkErrs: func(t *testing.T, errs []error) {
				t.Helper()

				if !errors.Is(errs[0], ErrUnsupportedVersion) {
					t.Errorf("expected ErrUnsupportedVersion, got %v", errs[0])
				}
			},
		},
		{
			name: "duplicate application names",
			config: &Config{
				Version: 3,
				Applications: []Application{
					{Name: "nvim"},
					{Name: "nvim"},
				},
			},
			wantCount: 1,
			checkErrs: func(t *testing.T, errs []error) {
				t.Helper()

				if !errors.Is(errs[0], ErrInvalidConfig) {
					t.Errorf("expected ErrInvalidConfig, got %v", errs[0])
				}
			},
		},
		{
			name: "empty application name",
			config: &Config{
				Version: 3,
				Applications: []Application{
					{Name: ""},
				},
			},
			wantCount: 1,
			checkErrs: func(t *testing.T, errs []error) {
				t.Helper()

				if !errors.Is(errs[0], ErrInvalidConfig) {
					t.Errorf("expected ErrInvalidConfig, got %v", errs[0])
				}
			},
		},
		{
			name: "duplicate entry names within application",
			config: &Config{
				Version: 3,
				Applications: []Application{
					{
						Name: "nvim",
						Entries: []SubEntry{
							{Name: "config", Backup: "./nvim"},
							{Name: "config", Backup: "./nvim2"},
						},
					},
				},
			},
			wantCount: 1,
			checkErrs: func(t *testing.T, errs []error) {
				t.Helper()

				if !errors.Is(errs[0], ErrInvalidConfig) {
					t.Errorf("expected ErrInvalidConfig, got %v", errs[0])
				}
			},
		},
		{
			name: "empty entry name",
			config: &Config{
				Version: 3,
				Applications: []Application{
					{
						Name: "nvim",
						Entries: []SubEntry{
							{Name: "", Backup: "./nvim"},
						},
					},
				},
			},
			wantCount: 1,
			checkErrs: func(t *testing.T, errs []error) {
				t.Helper()

				if !errors.Is(errs[0], ErrInvalidConfig) {
					t.Errorf("expected ErrInvalidConfig, got %v", errs[0])
				}
			},
		},
		{
			name: "multiple errors accumulated",
			config: &Config{
				Version: 2,
				Applications: []Application{
					{Name: ""},
					{Name: "nvim"},
					{Name: "nvim"},
				},
			},
			wantCount: 3, // bad version + empty name + duplicate name
		},
		{
			name: "same entry names across different apps is valid",
			config: &Config{
				Version: 3,
				Applications: []Application{
					{
						Name: "nvim",
						Entries: []SubEntry{
							{Name: "config", Backup: "./nvim"},
						},
					},
					{
						Name: "zsh",
						Entries: []SubEntry{
							{Name: "config", Backup: "./zsh"},
						},
					},
				},
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			errs := ValidateConfig(tt.config)
			if len(errs) != tt.wantCount {
				t.Errorf("ValidateConfig() returned %d errors, want %d: %v", len(errs), tt.wantCount, errs)
			}

			if tt.checkErrs != nil && len(errs) == tt.wantCount {
				tt.checkErrs(t, errs)
			}
		})
	}
}
