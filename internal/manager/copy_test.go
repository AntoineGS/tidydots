package manager

import (
	"io/fs"
	"testing"

	"github.com/AntoineGS/tidydots/internal/cmdexec"
	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/fsys"
	"github.com/AntoineGS/tidydots/internal/platform"
)

const fsModeSymlink = fs.ModeSymlink

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

func copyEntry(sudo bool) config.SubEntry {
	return config.SubEntry{
		Name: "e", Method: config.MethodCopy, Sudo: sudo,
		Backup: "/backup", Files: []string{"f"},
		Targets: map[string]string{"linux": "/etc"},
	}
}

func TestRestoreFileCopy_CreatesWhenMissing(t *testing.T) {
	t.Parallel()
	mgr, mem := newMemManager(t)
	_ = mem.MkdirAll("/backup", 0755)
	_ = mem.WriteFile("/backup/f", []byte("payload"), 0644)

	if err := mgr.restoreFileCopy(copyEntry(false), "/backup/f", "/etc/f"); err != nil {
		t.Fatalf("restoreFileCopy: %v", err)
	}
	got, err := mem.ReadFile("/etc/f")
	if err != nil || string(got) != "payload" {
		t.Errorf("target = (%q, %v), want (\"payload\", nil)", got, err)
	}
}

func TestRestoreFileCopy_SkipsWhenInSync(t *testing.T) {
	t.Parallel()
	mgr, mem := newMemManager(t)
	_ = mem.MkdirAll("/backup", 0755)
	_ = mem.MkdirAll("/etc", 0755)
	_ = mem.WriteFile("/backup/f", []byte("same"), 0644)
	_ = mem.WriteFile("/etc/f", []byte("same"), 0644)

	if err := mgr.restoreFileCopy(copyEntry(false), "/backup/f", "/etc/f"); err != nil {
		t.Fatalf("restoreFileCopy: %v", err)
	}
	// In-sync is a no-op; content stays identical.
	got, _ := mem.ReadFile("/etc/f")
	if string(got) != "same" {
		t.Errorf("target = %q, want \"same\"", got)
	}
}

func TestRestoreFileCopy_OverwritesOnDrift(t *testing.T) {
	t.Parallel()
	mgr, mem := newMemManager(t)
	_ = mem.MkdirAll("/backup", 0755)
	_ = mem.MkdirAll("/etc", 0755)
	_ = mem.WriteFile("/backup/f", []byte("new"), 0644)
	_ = mem.WriteFile("/etc/f", []byte("old"), 0644)

	if err := mgr.restoreFileCopy(copyEntry(false), "/backup/f", "/etc/f"); err != nil {
		t.Fatalf("restoreFileCopy: %v", err)
	}
	got, _ := mem.ReadFile("/etc/f")
	if string(got) != "new" {
		t.Errorf("target = %q, want \"new\"", got)
	}
}

func TestRestoreFileCopy_ReplacesExistingSymlink(t *testing.T) {
	t.Parallel()
	mgr, mem := newMemManager(t)
	_ = mem.MkdirAll("/backup", 0755)
	_ = mem.MkdirAll("/etc", 0755)
	_ = mem.WriteFile("/backup/f", []byte("real"), 0644)
	// Pre-existing symlink at the target (the migration case).
	_ = mem.Symlink("/backup/f", "/etc/f")

	if err := mgr.restoreFileCopy(copyEntry(false), "/backup/f", "/etc/f"); err != nil {
		t.Fatalf("restoreFileCopy: %v", err)
	}
	info, err := mem.Lstat("/etc/f")
	if err != nil {
		t.Fatalf("Lstat target: %v", err)
	}
	if info.Mode()&fsModeSymlink != 0 {
		t.Error("target is still a symlink, want a real file")
	}
	got, _ := mem.ReadFile("/etc/f")
	if string(got) != "real" {
		t.Errorf("target = %q, want \"real\"", got)
	}
}

func TestRestoreFileCopy_DryRunWritesNothing(t *testing.T) {
	t.Parallel()
	mgr, mem := newMemManager(t)
	mgr.DryRun = true
	_ = mem.MkdirAll("/backup", 0755)
	_ = mem.WriteFile("/backup/f", []byte("payload"), 0644)

	if err := mgr.restoreFileCopy(copyEntry(false), "/backup/f", "/etc/f"); err != nil {
		t.Fatalf("restoreFileCopy dry-run: %v", err)
	}
	if _, err := mem.Lstat("/etc/f"); err == nil {
		t.Error("target created during dry-run, want no write")
	}
}

func TestRestoreFileCopy_Sudo_MigratesSymlinkThenCopies(t *testing.T) {
	t.Parallel()
	mgr, mem, stub := newSudoManager(t)
	_ = mem.MkdirAll("/backup", 0755)
	_ = mem.MkdirAll("/etc", 0755)
	_ = mem.WriteFile("/backup/f", []byte("real"), 0644)
	_ = mem.Symlink("/backup/f", "/etc/f") // isSymlink(dst) via MemFS is true

	if err := mgr.restoreFileCopy(copyEntry(true), "/backup/f", "/etc/f"); err != nil {
		t.Fatalf("restoreFileCopy sudo: %v", err)
	}
	// Expect a sudo `rm -f` (remove symlink) followed by a sudo `cp`.
	if len(stub.Calls) != 2 ||
		stub.Calls[0].Name != "rm" || stub.Calls[1].Name != "cp" {
		t.Errorf("expected sudo rm then cp, got %+v", stub.Calls)
	}
}

func TestRestoreFiles_CopyEntry_RoutesToCopy(t *testing.T) {
	t.Parallel()
	mgr, mem := newMemManager(t)
	_ = mem.MkdirAll("/backup", 0755)
	_ = mem.WriteFile("/backup/f", []byte("payload"), 0644)

	err := mgr.RestoreFiles(copyEntry(false), "/backup", "/etc")
	if err != nil {
		t.Fatalf("RestoreFiles: %v", err)
	}
	info, err := mem.Lstat("/etc/f")
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}
	if info.Mode()&fsModeSymlink != 0 {
		t.Error("copy entry produced a symlink, want a real file")
	}
	got, _ := mem.ReadFile("/etc/f")
	if string(got) != "payload" {
		t.Errorf("target = %q, want \"payload\"", got)
	}
}
