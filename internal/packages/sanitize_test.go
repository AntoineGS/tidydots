package packages

import (
	"testing"
)

func TestValidateURLScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		// Valid schemes
		{
			name:    "https URL",
			url:     "https://example.com",
			wantErr: false,
		},
		{
			name:    "http URL",
			url:     "http://example.com",
			wantErr: false,
		},
		{
			name:    "https GitHub URL",
			url:     "https://github.com/user/repo.git",
			wantErr: false,
		},
		{
			name:    "bare absolute path",
			url:     "/usr/local",
			wantErr: false,
		},
		{
			name:    "bare relative path",
			url:     "./relative",
			wantErr: false,
		},
		{
			name:    "SSH-style URL",
			url:     "user@host:repo.git",
			wantErr: false,
		},
		{
			name:    "SSH git URL",
			url:     "git@github.com:user/repo.git",
			wantErr: false,
		},

		// Invalid schemes
		{
			name:    "file scheme",
			url:     "file:///etc/shadow",
			wantErr: true,
		},
		{
			name:    "ftp scheme",
			url:     "ftp://evil.com",
			wantErr: true,
		},
		{
			name:    "gopher scheme",
			url:     "gopher://evil.com",
			wantErr: true,
		},
		{
			name:    "ext transport for arbitrary command execution",
			url:     "ext::sh -c evil",
			wantErr: true,
		},
		{
			name:    "dict scheme",
			url:     "dict://evil.com",
			wantErr: true,
		},
		{
			name:    "file scheme uppercase",
			url:     "FILE:///etc/shadow",
			wantErr: true,
		},
		{
			name:    "ext transport uppercase",
			url:     "EXT::sh -c evil",
			wantErr: true,
		},
		{
			name:    "unknown scheme with colon-slash-slash",
			url:     "custom://evil.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateURLScheme(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURLScheme(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestEscapeShellSingleQuote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no quotes",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "single quote",
			input: "it's",
			want:  "it'\\''s",
		},
		{
			name:  "multiple quotes",
			input: "it's a 'test'",
			want:  "it'\\''s a '\\''test'\\''",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only single quote",
			input: "'",
			want:  "'\\''",
		},
		{
			name:  "consecutive single quotes",
			input: "''",
			want:  "'\\'''\\''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := escapeShellSingleQuote(tt.input)
			if got != tt.want {
				t.Errorf("escapeShellSingleQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapePowerShellSingleQuote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no quotes",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "single quote",
			input: "it's",
			want:  "it''s",
		},
		{
			name:  "multiple quotes",
			input: "it's a 'test'",
			want:  "it''s a ''test''",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only single quote",
			input: "'",
			want:  "''",
		},
		{
			name:  "consecutive single quotes",
			input: "''",
			want:  "''''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := escapePowerShellSingleQuote(tt.input)
			if got != tt.want {
				t.Errorf("escapePowerShellSingleQuote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
