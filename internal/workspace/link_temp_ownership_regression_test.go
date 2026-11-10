package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinkReplacementPreservesLegacyAdjacentPath(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "env")
	if err := os.Symlink("public", target); err != nil {
		t.Fatal(err)
	}
	legacy := target + ".tidydots-txn-link"
	if err := os.WriteFile(legacy, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := linkSnapshot{path: target, old: "public", next: "private", forwardTemp: target + ".txn-owned-forward"}
	if err := writeLink(link); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(legacy); err != nil || string(got) != "unrelated" {
		t.Fatalf("legacy adjacent file changed: %q, %v", got, err)
	}
	if got, err := os.Readlink(target); err != nil || got != "private" {
		t.Fatalf("target link = %q, %v; want private", got, err)
	}
}

func TestLinkRollbackFailsClosedOnOccupiedJournalTemp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "env")
	if err := os.Symlink("private", target); err != nil {
		t.Fatal(err)
	}
	occupied := filepath.Join(dir, "txn-link-reserved")
	if err := os.Symlink("external", occupied); err != nil {
		t.Fatal(err)
	}
	link := linkSnapshot{path: target, old: "public", next: "private", rollbackTemp: occupied}
	if err := restoreLink(link); err == nil {
		t.Fatal("restoreLink succeeded with an occupied temporary path")
	}
	if got, err := os.Readlink(occupied); err != nil || got != "external" {
		t.Fatalf("occupied external link changed: %q, %v", got, err)
	}
	if got, err := os.Readlink(target); err != nil || got != "private" {
		t.Fatalf("target changed after refusal: %q, %v", got, err)
	}
}

func TestLinkRollbackCleansRecordedForwardTempAfterInterruptedRename(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "env")
	if err := os.Symlink("public", target); err != nil {
		t.Fatal(err)
	}
	temporary := target + ".txn-owned-forward"
	if err := os.Symlink("private", temporary); err != nil {
		t.Fatal(err)
	}
	if err := restoreLink(linkSnapshot{path: target, old: "public", next: "private", forwardTemp: temporary}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(temporary); !os.IsNotExist(err) {
		t.Fatalf("interrupted transaction temp remains: %v", err)
	}
	if got, err := os.Readlink(target); err != nil || got != "public" {
		t.Fatalf("target link = %q, %v; want public", got, err)
	}
}
