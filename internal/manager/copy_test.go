package manager

import (
	"testing"

	"github.com/AntoineGS/tidydots/internal/cmdexec"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/fsys"
	"github.com/AntoineGS/tidydots/internal/platform"
)

// newSudoManager returns a MemFS-backed Manager with a StubRunner, for asserting
// on the sudo command calls without touching the real filesystem.
func newSudoManager(t *testing.T) (*Manager, *fsys.MemFS, *cmdexec.StubRunner) {
	t.Helper()
	mem := fsys.NewMemFS()
	stub := cmdexec.NewStubRunner()
	cfg := &config.Config{BackupRoot: "/backup"}
	plat := &platform.Platform{OS: platform.OSLinux, EnvVars: map[string]string{}}
	return New(cfg, plat).WithFS(mem).WithRunner(stub), mem, stub
}

func TestFilesEqual_NoSudo_EqualAndDiffer(t *testing.T) {
	t.Parallel()
	mgr, mem := newMemManager(t)
	_ = mem.WriteFile("/a", []byte("same"), 0644)
	_ = mem.WriteFile("/b", []byte("same"), 0644)
	_ = mem.WriteFile("/c", []byte("diff"), 0644)

	if eq, err := mgr.filesEqual("/a", "/b", false); err != nil || !eq {
		t.Errorf("filesEqual equal = (%v, %v), want (true, nil)", eq, err)
	}
	if eq, err := mgr.filesEqual("/a", "/c", false); err != nil || eq {
		t.Errorf("filesEqual differ = (%v, %v), want (false, nil)", eq, err)
	}
}

func TestFilesEqual_MissingDst_NotEqual(t *testing.T) {
	t.Parallel()
	mgr, mem := newMemManager(t)
	_ = mem.WriteFile("/a", []byte("x"), 0644)
	if eq, err := mgr.filesEqual("/a", "/missing", false); err != nil || eq {
		t.Errorf("filesEqual missing dst = (%v, %v), want (false, nil)", eq, err)
	}
}

func TestFilesEqual_Sudo_UsesCmpExitCode(t *testing.T) {
	t.Parallel()
	mgr, mem, stub := newSudoManager(t)
	_ = mem.WriteFile("/src", []byte("x"), 0644)
	_ = mem.WriteFile("/dst", []byte("x"), 0644) // presence only; cmp result is stubbed
	stub.AddResult("cmp", cmdexec.Result{ExitCode: 0})

	eq, err := mgr.filesEqual("/src", "/dst", true)
	if err != nil || !eq {
		t.Fatalf("filesEqual sudo equal = (%v, %v), want (true, nil)", eq, err)
	}
	if len(stub.Calls) != 1 || stub.Calls[0].Name != "cmp" || !stub.Calls[0].Sudo {
		t.Errorf("expected one sudo cmp call, got %+v", stub.Calls)
	}
}

func TestCopyFileTo_Sudo_RecordsCp(t *testing.T) {
	t.Parallel()
	mgr, mem, stub := newSudoManager(t)
	_ = mem.WriteFile("/src", []byte("x"), 0644)

	if err := mgr.copyFileTo("/src", "/dst", true); err != nil {
		t.Fatalf("copyFileTo sudo: %v", err)
	}
	if len(stub.Calls) != 1 || stub.Calls[0].Name != "cp" ||
		stub.Calls[0].Args[0] != "/src" || stub.Calls[0].Args[1] != "/dst" {
		t.Errorf("expected sudo `cp /src /dst`, got %+v", stub.Calls)
	}
}

func TestCopyFileTo_NoSudo_WritesViaFS(t *testing.T) {
	t.Parallel()
	mgr, mem := newMemManager(t)
	_ = mem.WriteFile("/src", []byte("hello"), 0644)

	if err := mgr.copyFileTo("/src", "/dst", false); err != nil {
		t.Fatalf("copyFileTo: %v", err)
	}
	got, err := mem.ReadFile("/dst")
	if err != nil || string(got) != "hello" {
		t.Errorf("dst = (%q, %v), want (\"hello\", nil)", got, err)
	}
}

func TestRemovePath_NoSudo_RemovesViaFS(t *testing.T) {
	t.Parallel()
	mgr, mem := newMemManager(t)
	_ = mem.WriteFile("/f", []byte("x"), 0644)

	if err := mgr.removePath("/f", false); err != nil {
		t.Fatalf("removePath: %v", err)
	}
	if _, err := mem.Lstat("/f"); err == nil {
		t.Error("file still exists after removePath, want removed")
	}
}

func TestRemovePath_Sudo_RecordsRmForce(t *testing.T) {
	t.Parallel()
	mgr, _, stub := newSudoManager(t)

	if err := mgr.removePath("/etc/f", true); err != nil {
		t.Fatalf("removePath sudo: %v", err)
	}
	if len(stub.Calls) != 1 || stub.Calls[0].Name != "rm" ||
		len(stub.Calls[0].Args) != 2 || stub.Calls[0].Args[0] != "-f" ||
		stub.Calls[0].Args[1] != "/etc/f" || !stub.Calls[0].Sudo {
		t.Errorf("expected one sudo `rm -f /etc/f`, got %+v", stub.Calls)
	}
}
