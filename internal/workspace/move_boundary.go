package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AntoineGS/tidydots/internal/state"
)

func verifySnapshotTreeAncestry(repositoryRoot, backup string, files []fileSnapshot) error {
	if err := verifyPathParents(repositoryRoot, backup); err != nil {
		return err
	}
	rootInfo, err := os.Lstat(backup)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return fmt.Errorf("snapshot backup root is not a real directory: %s", backup)
	}
	for _, file := range files {
		if file.rel == "." {
			continue
		}
		path := filepath.Join(backup, file.rel)
		if err := verifyPathParents(repositoryRoot, path); err != nil {
			return err
		}
		for parent := filepath.Dir(path); parent != backup; parent = filepath.Dir(parent) {
			info, statErr := os.Lstat(parent)
			if statErr != nil {
				return statErr
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("snapshot path has a substituted or non-directory ancestor: %s", parent)
			}
		}
	}
	return nil
}

// verifyRemainingSourceAncestry validates live source members against the
// journal's cleanup progress. An outstanding unlink intent may be absent or
// may still match its exact before-image; completed removals must stay absent.
func verifyRemainingSourceAncestry(repositoryRoot, backup string, j *moveJournal) error {
	pending, err := remainingSourceSnapshots(backup, j)
	if err != nil || len(pending) == 0 {
		return err
	}
	if err := verifyPathParents(repositoryRoot, backup); err != nil {
		return err
	}
	rootInfo, err := os.Lstat(backup)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return fmt.Errorf("remaining source backup root is not a real directory: %s", backup)
	}
	expected := make(map[string]fileSnapshot, len(pending))
	for _, file := range pending {
		if err := verifyRemainingSourcePath(repositoryRoot, backup, file); err != nil {
			return err
		}
		expected[filepath.Clean(file.rel)] = file
	}
	if !j.WholeFolder {
		return nil
	}
	return verifyRemainingWholeFolder(backup, expected)
}

func remainingSourceSnapshots(backup string, j *moveJournal) ([]fileSnapshot, error) {
	cleaned := make(map[string]bool, len(j.CleanedPaths))
	for _, rel := range j.CleanedPaths {
		cleaned[filepath.Clean(rel)] = true
	}
	intentRel := ""
	if strings.HasPrefix(j.Intent, "cleanup-source:") {
		intentRel = filepath.Clean(strings.TrimPrefix(j.Intent, "cleanup-source:"))
	}
	var pending []fileSnapshot
	for _, file := range j.Files {
		rel := filepath.Clean(file.rel)
		path := backup
		if rel != "." {
			path = filepath.Join(backup, rel)
		}
		if cleaned[rel] {
			if err := verifyCompletedRemovalAncestry(backup, rel, cleaned, intentRel); err != nil {
				return nil, err
			}
			if _, err := os.Lstat(path); err == nil {
				return nil, fmt.Errorf("completed source removal was recreated: %s", path)
			} else if !os.IsNotExist(err) {
				return nil, err
			}
			continue
		}
		if rel == intentRel {
			if _, err := os.Lstat(path); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return nil, err
			}
		}
		pending = append(pending, file)
	}
	return pending, nil
}

func verifyCompletedRemovalAncestry(backup, rel string, cleaned map[string]bool, intentRel string) error {
	if rel == "." {
		return nil
	}
	for parent := filepath.Dir(rel); ; parent = filepath.Dir(parent) {
		path := backup
		if parent != "." {
			path = filepath.Join(backup, parent)
		}
		parentRel := filepath.Clean(parent)
		if cleaned[parentRel] {
			if _, err := os.Lstat(path); err == nil {
				return fmt.Errorf("completed source ancestor removal was recreated: %s", path)
			} else if !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		info, err := os.Lstat(path)
		if os.IsNotExist(err) && parentRel == intentRel {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("completed source path has a substituted or non-directory ancestor: %s", path)
		}
		if parentRel == "." {
			return nil
		}
	}
}

func verifyRemainingSourcePath(repositoryRoot, backup string, file fileSnapshot) error {
	rel := filepath.Clean(file.rel)
	path := backup
	if rel != "." {
		path = filepath.Join(backup, rel)
		if err := verifyPathParents(repositoryRoot, path); err != nil {
			return err
		}
		for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
			info, err := os.Lstat(parent)
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("remaining source path has a substituted or non-directory ancestor: %s", parent)
			}
			if parent == backup {
				break
			}
		}
	}
	if err := verifyOneSnapshot(path, file); err != nil {
		return fmt.Errorf("remaining source path changed: %w", err)
	}
	return nil
}

func verifyRemainingWholeFolder(backup string, expected map[string]fileSnapshot) error {
	return filepath.WalkDir(backup, func(path string, _ os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := "."
		if path != backup {
			var err error
			rel, err = filepath.Rel(backup, path)
			if err != nil {
				return err
			}
		}
		file, ok := expected[filepath.Clean(rel)]
		if !ok {
			return fmt.Errorf("unexpected source path appeared during cleanup: %s", rel)
		}
		return verifyOneSnapshot(path, file)
	})
}

func isGeneratedSnapshotPath(rel string) bool {
	return strings.HasSuffix(rel, ".tmpl.rendered") || strings.HasSuffix(rel, ".tmpl.conflict") || strings.HasSuffix(rel, ".tmpl.rendered.bak")
}

func revalidatePlannedArtifacts(root, backupRel, method string, files []fileSnapshot, records []state.RenderRecord, stateKeys []string, cleaned []string) error {
	cleanedSet := make(map[string]bool, len(cleaned))
	for _, rel := range cleaned {
		cleanedSet[filepath.Clean(rel)] = true
	}
	expected := make(map[string]fileSnapshot)
	for _, file := range files {
		if isGeneratedSnapshotPath(file.rel) {
			if !cleanedSet[filepath.Clean(file.rel)] {
				expected[filepath.Clean(file.rel)] = file
			}
		}
	}
	rootPath := filepath.Join(root, filepath.FromSlash(backupRel))
	actualGenerated, err := discoverAttributedArtifacts(rootPath, backupRel, method, files, records, stateKeys)
	if err != nil {
		return err
	}
	for rel := range cleanedSet {
		path := filepath.Join(rootPath, rel)
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("completed generated-artifact removal was recreated: %s", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if len(actualGenerated) != len(expected) {
		return fmt.Errorf("attributable generated artifact membership changed")
	}
	for rel, want := range expected {
		got, ok := actualGenerated[rel]
		if !ok {
			return fmt.Errorf("attributable generated artifact changed: %s", rel)
		}
		if !sameSnapshot(got, want) {
			return fmt.Errorf("attributable generated artifact changed: %s", rel)
		}
	}
	return nil
}

func discoverAttributedArtifacts(rootPath, backupRel, method string, files []fileSnapshot, records []state.RenderRecord, stateKeys []string) (map[string]fileSnapshot, error) {
	remaps := make(map[string]string)
	for _, key := range stateKeys {
		if from, to, ok := decodeStateRemap(key); ok {
			remaps[from] = to
		}
	}
	result := make(map[string]fileSnapshot)
	for _, file := range files {
		if !strings.HasSuffix(file.rel, ".tmpl") || file.dir || file.link != "" {
			continue
		}
		sourceKey := "./" + filepath.ToSlash(filepath.Join(backupRel, file.rel))
		historyOwned := renderHistoryOwnsSource(sourceKey, records, remaps)
		aliasOwned := method == "symlink" && hasCapturedTemplateAlias(rootPath, file.rel, files)
		for _, suffix := range []string{templateRenderedSuffix, templateConflictSuffix, templateRenderedSuffix + ".bak"} {
			rel := filepath.Clean(file.rel + suffix)
			if !historyOwned && !aliasOwned {
				if _, err := os.Lstat(filepath.Join(rootPath, rel)); err == nil {
					return nil, fmt.Errorf("cannot prove ownership of generated template artifact %q", rel)
				} else if !os.IsNotExist(err) {
					return nil, err
				}
				continue
			}
			artifact, exists, err := captureGeneratedArtifact(rootPath, rel)
			if err != nil {
				return nil, err
			}
			if exists {
				result[rel] = artifact
			}
		}
	}
	return result, nil
}

func sameSnapshot(a, b fileSnapshot) bool {
	return filepath.Clean(a.rel) == filepath.Clean(b.rel) && a.mode == b.mode && a.dir == b.dir && a.link == b.link && bytes.Equal(a.data, b.data)
}

func revalidateCommittedSourceSet(ctx context.Context, j *moveJournal) error {
	if len(j.Files) == 0 {
		return nil
	}
	rootRemoved, err := sourceRootRemovalComplete(j)
	if err != nil {
		return err
	}
	cleaned := append([]string(nil), j.CleanedPaths...)
	if strings.HasPrefix(j.Intent, "cleanup-source:") {
		rel := strings.TrimPrefix(j.Intent, "cleanup-source:")
		path := filepath.Join(j.SourceRoot, filepath.FromSlash(j.SourceRel), filepath.FromSlash(rel))
		if rel == "." {
			path = filepath.Join(j.SourceRoot, filepath.FromSlash(j.SourceRel))
		}
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			cleaned = append(cleaned, rel)
		} else if err != nil {
			return err
		}
	}
	if !rootRemoved {
		if err := revalidatePlannedArtifacts(j.SourceRoot, j.SourceRel, j.SourceMethod, j.Files, j.StateRecords, j.StateKeys, cleaned); err != nil {
			return fmt.Errorf("source artifact set changed after commit: %w", err)
		}
	}
	if err := revalidateMoveSourceState(ctx, j); err != nil {
		return err
	}
	return nil
}

func sourceRootRemovalComplete(j *moveJournal) (bool, error) {
	if !j.WholeFolder {
		return false, nil
	}
	rootSnapshot, err := wholeFolderRootSnapshot(j)
	if err != nil {
		return false, err
	}
	root := filepath.Join(j.SourceRoot, filepath.FromSlash(j.SourceRel))
	cleaned, intent := sourceRootProgress(j)
	_, err = os.Lstat(root)
	if os.IsNotExist(err) {
		if cleaned || intent {
			return true, nil
		}
		return false, fmt.Errorf("source backup root disappeared without cleanup progress: %s", root)
	}
	if err != nil {
		return false, err
	}
	if cleaned {
		return false, fmt.Errorf("completed source-root removal was recreated: %s", root)
	}
	if intent {
		if err := verifyOneSnapshot(root, rootSnapshot); err != nil {
			return false, fmt.Errorf("source root changed under removal intent: %w", err)
		}
	}
	return false, nil
}

func wholeFolderRootSnapshot(j *moveJournal) (fileSnapshot, error) {
	for _, file := range j.Files {
		if file.rel == "." && file.dir {
			return file, nil
		}
	}
	return fileSnapshot{}, fmt.Errorf("whole-folder journal is missing its root snapshot")
}

func sourceRootProgress(j *moveJournal) (cleaned, intent bool) {
	for _, rel := range j.CleanedPaths {
		if filepath.Clean(rel) == "." {
			cleaned = true
			break
		}
	}
	return cleaned, j.Intent == "cleanup-source:."
}

func revalidateMoveIgnores(ctx context.Context, j *moveJournal) error {
	if len(j.Files) == 0 {
		return nil
	}
	root := filepath.Join(j.SourceRoot, filepath.FromSlash(j.SourceRel))
	rootRemoved, err := sourceRootRemovalComplete(j)
	if err != nil {
		return err
	}
	all := append([]fileSnapshot(nil), j.Files...)
	if !rootRemoved {
		files, err := discoverAttributedArtifacts(root, j.SourceRel, j.SourceMethod, j.Files, j.StateRecords, j.StateKeys)
		if err != nil {
			return err
		}
		for rel, file := range files {
			found := false
			for _, existing := range j.Files {
				if filepath.Clean(existing.rel) == rel {
					found = true
					break
				}
			}
			if !found {
				all = append(all, file)
			}
		}
	}
	if err := checkGeneratedIgnoresAt(ctx, j.SourceRoot, j.SourceRel, all); err != nil {
		return err
	}
	return checkGeneratedIgnoresAt(ctx, j.DestinationRoot, j.DestinationRel, all)
}
