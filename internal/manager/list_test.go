package manager

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/AntoineGS/dot-manager/internal/config"
	"github.com/AntoineGS/dot-manager/internal/platform"
)

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestList(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		BackupRoot: "/home/user/backup",
		Paths: []config.PathSpec{
			{
				Name:   "nvim",
				Files:  []string{},
				Backup: "./nvim",
				Targets: map[string]string{
					"linux": "~/.config/nvim",
				},
			},
			{
				Name:   "bash",
				Files:  []string{".bashrc", ".bash_profile"},
				Backup: "./bash",
				Targets: map[string]string{
					"linux": "~",
				},
			},
			{
				Name:   "windows-only",
				Files:  []string{"settings.json"},
				Backup: "./windows",
				Targets: map[string]string{
					"windows": "~/AppData",
				},
			},
		},
	}

	plat := &platform.Platform{OS: platform.OSLinux}
	mgr := New(cfg, plat)

	output := captureOutput(func() {
		mgr.List()
	})

	// Check output contains expected information
	if !strings.Contains(output, "nvim") {
		t.Error("Output should contain 'nvim'")
	}

	if !strings.Contains(output, "[folder]") {
		t.Error("Output should show [folder] for nvim")
	}

	if !strings.Contains(output, "bash") {
		t.Error("Output should contain 'bash'")
	}

	if !strings.Contains(output, ".bashrc") {
		t.Error("Output should contain '.bashrc'")
	}

	if !strings.Contains(output, "not applicable") {
		t.Error("Output should indicate windows-only is not applicable for linux")
	}
}

func TestListRootMode(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		BackupRoot: "/home/user/backup",
		Paths: []config.PathSpec{
			{
				Name:   "user-config",
				Backup: "./user",
				Targets: map[string]string{
					"linux": "~/.config",
				},
			},
		},
		RootPaths: []config.PathSpec{
			{
				Name:   "system-config",
				Files:  []string{"config.hook"},
				Backup: "./system",
				Targets: map[string]string{
					"linux": "/etc/hooks",
				},
			},
		},
	}

	// Test as root
	plat := &platform.Platform{OS: platform.OSLinux, IsRoot: true}
	mgr := New(cfg, plat)

	output := captureOutput(func() {
		mgr.List()
	})

	if !strings.Contains(output, "system-config") {
		t.Error("Root mode should show system-config")
	}

	if strings.Contains(output, "user-config") {
		t.Error("Root mode should not show user-config")
	}

	// Test as non-root
	plat = &platform.Platform{OS: platform.OSLinux, IsRoot: false}
	mgr = New(cfg, plat)

	output = captureOutput(func() {
		mgr.List()
	})

	if strings.Contains(output, "system-config") {
		t.Error("Non-root mode should not show system-config")
	}

	if !strings.Contains(output, "user-config") {
		t.Error("Non-root mode should show user-config")
	}
}
