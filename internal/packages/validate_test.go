package packages

import (
	"testing"
)

func TestValidatePackageName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pkgName string
		wantErr bool
	}{
		// Valid package names
		{
			name:    "simple name",
			pkgName: "neovim",
			wantErr: false,
		},
		{
			name:    "name with hyphen",
			pkgName: "gcc-libs",
			wantErr: false,
		},
		{
			name:    "name with dot and digits",
			pkgName: "python3.11",
			wantErr: false,
		},
		{
			name:    "tap-style name with slashes",
			pkgName: "user/tap/pkg",
			wantErr: false,
		},
		{
			name:    "scoped npm-style name",
			pkgName: "@scope/pkg",
			wantErr: false,
		},
		{
			name:    "name with plus",
			pkgName: "some+thing",
			wantErr: false,
		},
		{
			name:    "name with underscore",
			pkgName: "base_devel",
			wantErr: false,
		},
		{
			name:    "name with hyphen in middle",
			pkgName: "base-devel",
			wantErr: false,
		},
		{
			name:    "name with colon",
			pkgName: "lib32:mesa",
			wantErr: false,
		},
		{
			name:    "single character",
			pkgName: "a",
			wantErr: false,
		},

		// Invalid package names
		{
			name:    "empty string",
			pkgName: "",
			wantErr: true,
		},
		{
			name:    "flag injection short",
			pkgName: "-S",
			wantErr: true,
		},
		{
			name:    "flag injection long",
			pkgName: "--noconfirm",
			wantErr: true,
		},
		{
			name:    "null byte",
			pkgName: "pkg\x00evil",
			wantErr: true,
		},
		{
			name:    "semicolon shell injection",
			pkgName: "pkg;rm -rf /",
			wantErr: true,
		},
		{
			name:    "dollar sign shell injection",
			pkgName: "pkg$HOME",
			wantErr: true,
		},
		{
			name:    "backtick command substitution",
			pkgName: "pkg`whoami`",
			wantErr: true,
		},
		{
			name:    "pipe shell injection",
			pkgName: "pkg|cat /etc/passwd",
			wantErr: true,
		},
		{
			name:    "double ampersand shell injection",
			pkgName: "pkg&&evil",
			wantErr: true,
		},
		{
			name:    "space in name",
			pkgName: "pkg name",
			wantErr: true,
		},
		{
			name:    "parentheses",
			pkgName: "pkg()",
			wantErr: true,
		},
		{
			name:    "newline injection",
			pkgName: "pkg\nevil",
			wantErr: true,
		},
		{
			name:    "single quote",
			pkgName: "it's",
			wantErr: true,
		},
		{
			name:    "double quote",
			pkgName: `pkg"evil`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidatePackageName(tt.pkgName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePackageName(%q) error = %v, wantErr %v", tt.pkgName, err, tt.wantErr)
			}
		})
	}
}
