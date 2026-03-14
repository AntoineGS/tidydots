package tui

import (
	"testing"
)

func TestShellEscape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal path stays safe",
			input: "/home/user/.config/nvim",
			want:  "'/home/user/.config/nvim'",
		},
		{
			name:  "path with spaces is properly quoted",
			input: "/home/user/my configs/file",
			want:  "'/home/user/my configs/file'",
		},
		{
			name:  "path with single quotes is properly escaped",
			input: "/home/user/it's a file",
			want:  `'/home/user/it'\''s a file'`,
		},
		{
			name:  "path with backticks prevents injection",
			input: "/home/user/config`id`.tmpl",
			want:  "'/home/user/config`id`.tmpl'",
		},
		{
			name:  "path with dollar-paren prevents injection",
			input: "/home/user/$(whoami).tmpl",
			want:  "'/home/user/$(whoami).tmpl'",
		},
		{
			name:  "path with semicolons prevents injection",
			input: "/home/user/file;rm -rf /",
			want:  "'/home/user/file;rm -rf /'",
		},
		{
			name:  "empty string",
			input: "",
			want:  "''",
		},
		{
			name:  "path with double quotes",
			input: `/home/user/"file"`,
			want:  `'/home/user/"file"'`,
		},
		{
			name:  "path with newline prevents injection",
			input: "/home/user/file\nrm -rf /",
			want:  "'/home/user/file\nrm -rf /'",
		},
		{
			name:  "path with dollar variable prevents expansion",
			input: "/home/$USER/.config",
			want:  "'/home/$USER/.config'",
		},
		{
			name:  "path with multiple single quotes",
			input: "it's a 'test' file",
			want:  `'it'\''s a '\''test'\'' file'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := shellEscape(tt.input)
			if got != tt.want {
				t.Errorf("shellEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
