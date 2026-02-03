package packages

import (
	"errors"
	"testing"
)

func TestInstallError(t *testing.T) {
	baseErr := errors.New("command failed")
	installErr := NewInstallError("vim", Pacman, baseErr)

	var ie *InstallError
	if !errors.As(installErr, &ie) {
		t.Fatal("Should be InstallError type")
	}

	if ie.Package != "vim" {
		t.Errorf("Package = %q, want %q", ie.Package, "vim")
	}

	if ie.Manager != Pacman {
		t.Errorf("Manager = %q, want %q", ie.Manager, Pacman)
	}

	if !errors.Is(installErr, baseErr) {
		t.Error("InstallError should wrap underlying error")
	}

	// Check error message format
	msg := installErr.Error()
	if msg != "install vim via pacman: command failed" {
		t.Errorf("Error() = %q, want %q", msg, "install vim via pacman: command failed")
	}
}

func TestPackagesSentinelErrors(t *testing.T) {
	tests := []struct {
		err  error
		want error
		name string
	}{
		{
			name: "no_manager_available",
			err:  ErrNoManagerAvailable,
			want: ErrNoManagerAvailable,
		},
		{
			name: "install_failed",
			err:  ErrInstallFailed,
			want: ErrInstallFailed,
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
