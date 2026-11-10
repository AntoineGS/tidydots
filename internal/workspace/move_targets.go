package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	tmpl "github.com/AntoineGS/tidydots/internal/template"
)

//nolint:gocyclo // verifies current-platform targets and known template aliases.
func findVerifiedLinks(entry config.SubEntry, source, destination *Repository, plat *platform.Platform, captured ...[]fileSnapshot) ([]linkSnapshot, error) {
	rawTarget := entry.GetTarget(plat.OS)
	if rawTarget == "" {
		return nil, nil
	}
	target := rawTarget
	if strings.Contains(target, "{{") {
		rendered, e := tmpl.NewEngine(tmpl.NewContextFromPlatform(plat)).RenderString("workspace move target", target)
		if e != nil {
			return nil, e
		}
		target = rendered
	}
	target = config.ExpandPath(target, plat.EnvVars)
	if !filepath.IsAbs(target) {
		target = filepath.Join(".", target)
	}
	target = filepath.Clean(target)
	backup, err := staticBackup(source, entry.Backup)
	if err != nil {
		return nil, err
	}
	files := []fileSnapshot(nil)
	if len(captured) > 0 {
		files = captured[0]
	} else {
		files, err = snapshotOwned(backup, entry.Files)
		if err != nil {
			return nil, err
		}
	}
	newBackup := filepath.Join(destination.Root, filepath.Base(backup))
	rel, err := filepath.Rel(source.Root, backup)
	if err == nil {
		newBackup = filepath.Join(destination.Root, rel)
	}
	var paths []struct{ target, old, new string }
	if entry.IsFolder() {
		paths = append(paths, struct{ target, old, new string }{target, backup, newBackup})
	} else {
		for _, f := range entry.Files {
			old := filepath.Join(backup, f)
			nextSource := filepath.Join(newBackup, f)
			targetRel := f
			if tmpl.IsTemplateFile(f) {
				targetRel = filepath.Join(filepath.Dir(f), tmpl.TargetName(f))
				old = filepath.Join(backup, filepath.Join(filepath.Dir(f), tmpl.TargetName(f)))
				nextSource = filepath.Join(newBackup, filepath.Join(filepath.Dir(f), tmpl.TargetName(f)))
			}
			paths = append(paths, struct{ target, old, new string }{filepath.Join(target, targetRel), old, nextSource})
		}
	}
	var result []linkSnapshot
	for _, p := range paths {
		// A link is retargeted only when both its current referent and its
		// intended destination are represented by the exact captured move set.
		// In particular, an ordinary suffix-free file is not a template alias
		// merely because a selected .tmpl file has the corresponding basename.
		if !snapshotContainsPath(files, backup, p.old) || !snapshotContainsPath(files, newBackup, p.new) {
			continue
		}
		if err := rejectSourceSymlinkParents(p.target, source.Root); err != nil {
			return nil, err
		}
		info, e := os.Lstat(p.target)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		if info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		link, e := os.Readlink(p.target)
		if e != nil {
			return nil, e
		}
		resolved := link
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(filepath.Dir(p.target), resolved)
		}
		resolved, e = filepath.Abs(resolved)
		if e != nil {
			return nil, e
		}
		if filepath.Clean(resolved) != filepath.Clean(p.old) {
			continue
		}
		result = append(result, linkSnapshot{path: p.target, old: link, next: relativeLink(p.target, p.new)})
	}
	if len(result) > 0 && entry.Sudo {
		return nil, fmt.Errorf("privileged live-link retargeting is unavailable in workspace moves; verified live links require elevated replacement, no files were changed (controller support is required)")
	}
	return result, nil
}

func snapshotContainsPath(files []fileSnapshot, root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	rel = filepath.Clean(rel)
	for _, file := range files {
		if filepath.Clean(file.rel) == rel {
			return true
		}
	}
	return false
}

func rejectSourceSymlinkParents(target, sourceRoot string) error {
	parent := filepath.Dir(target)
	for {
		info, err := os.Lstat(parent)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			resolved, e := filepath.EvalSymlinks(parent)
			if e != nil {
				return e
			}
			absolute, e := filepath.Abs(resolved)
			if e != nil {
				return e
			}
			if sameOrChild(sourceRoot, filepath.Clean(absolute)) {
				return fmt.Errorf("target parent %q traverses a symlink into the source repository; refusing to retarget through it", parent)
			}
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		parent = next
	}
	return nil
}

func relativeLink(target, source string) string {
	rel, err := filepath.Rel(filepath.Dir(target), source)
	if err != nil {
		return source
	}
	return rel
}

func writeLink(link linkSnapshot) error {
	current, err := os.Readlink(link.path)
	if err != nil {
		return err
	}
	if current != link.old && current != link.next {
		return fmt.Errorf("live link changed during move: %s", link.path)
	}
	if current == link.next {
		return nil
	}
	return replaceLink(link.path, link.next, link.forwardTemp)
}

func restoreLink(link linkSnapshot) error {
	current, err := os.Readlink(link.path)
	if err != nil {
		return err
	}
	if current == link.old {
		return discardOwnedLinkTemp(link.forwardTemp, link.next)
	}
	if current != link.next {
		return fmt.Errorf("live link changed since transaction: %s", link.path)
	}
	return replaceLink(link.path, link.old, link.rollbackTemp)
}

// replaceLink uses a path reserved in the durable journal for this transaction.
// Symlink creation is exclusive, so an occupied temporary path is never removed.
// If recovery finds the exact expected link at the journaled path, it may finish
// the interrupted atomic rename; unrelated contents fail closed.
func replaceLink(target, destination, temporary string) error {
	if temporary == "" {
		return fmt.Errorf("transaction link temporary path is missing for %s", target)
	}
	if err := os.Symlink(destination, temporary); err != nil {
		if !os.IsExist(err) {
			return err
		}
		actual, readErr := os.Readlink(temporary)
		if readErr != nil || actual != destination {
			return fmt.Errorf("transaction link temporary path is occupied: %s", temporary)
		}
	}
	if err := os.Rename(temporary, target); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(target))
}

func discardOwnedLinkTemp(temporary, expected string) error {
	if temporary == "" {
		return nil
	}
	actual, err := os.Readlink(temporary)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || actual != expected {
		return fmt.Errorf("transaction link temporary path is occupied during recovery: %s", temporary)
	}
	if err := os.Remove(temporary); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(temporary))
}
