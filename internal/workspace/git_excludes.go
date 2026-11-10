package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var moveGitIgnorePatterns = []string{
	transactionIgnoreDir,
	transactionConfigIgnore,
	stateDatabaseName,
	"*.tmpl.rendered",
	"*.tmpl.conflict",
}

//nolint:gocyclo // effective exclusion requires resolution, safe write and proof.
func registerGitExcludes(root string) error {
	git, hasGit, err := gitExecutableOrNonGit(root, "register transaction excludes")
	if err != nil {
		return err
	}
	if !hasGit {
		return nil
	}
	_, err = gitOutput(git, root, "rev-parse", "--absolute-git-dir")
	if err != nil {
		if isNotGitRepository(err) {
			return nil
		}
		return fmt.Errorf("resolve Git directory for %s: %w", root, err)
	}
	excludePath, err := gitOutput(git, root, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return fmt.Errorf("resolve effective Git exclude path for %s: %w", root, err)
	}
	if !filepath.IsAbs(excludePath) {
		excludePath = filepath.Join(root, excludePath)
	}
	excludePath, err = filepath.Abs(excludePath)
	if err != nil {
		return err
	}
	excludePath = filepath.Clean(excludePath)
	if err := verifyNoSymlinkPath(excludePath); err != nil {
		return fmt.Errorf("unsafe Git exclude path %q: %w", excludePath, err)
	}
	contents, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read Git exclude file %q: %w", excludePath, err)
	}
	var missing []string
	for _, pattern := range moveGitIgnorePatterns {
		if !containsGitIgnorePattern(contents, pattern) {
			missing = append(missing, pattern)
		}
	}
	if len(missing) > 0 {
		if err := appendGitExcludeNoFollow(excludePath, contents, missing); err != nil {
			return fmt.Errorf("update Git exclude file %q: %w", excludePath, err)
		}
	}
	for _, pattern := range moveGitIgnorePatterns {
		probe := pattern
		switch pattern {
		case ".tidydots-txn/":
			probe = ".tidydots-txn/marker"
		case ".tidydots-txn-config-*":
			probe = ".tidydots-txn-config-probe"
		case "*.tmpl.rendered":
			probe = "nested/file.tmpl.rendered"
		case "*.tmpl.conflict":
			probe = "nested/file.tmpl.conflict"
		}
		cmd := exec.CommandContext(context.Background(), git, "check-ignore", "-q", "--no-index", "--", probe)
		cmd.Dir = root
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("git exclude rule %q is not effective for %q in %s: %w", pattern, probe, root, err)
		}
	}
	return nil
}

func gitOutput(git, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(context.Background(), git, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

// gitExecutableOrNonGit permits a truly non-Git root to work when Git is not
// installed, while refusing to bypass ignore validation for a Git checkout.
func gitExecutableOrNonGit(root, operation string) (string, bool, error) {
	git, err := exec.LookPath("git")
	if err == nil {
		return git, true, nil
	}
	isGit, inspectErr := rootHasGitMetadata(root)
	if inspectErr != nil {
		return "", false, errors.Join(fmt.Errorf("%s: locate git: %w", operation, err), fmt.Errorf("inspect root metadata: %w", inspectErr))
	}
	if !isGit {
		return "", false, nil
	}
	return "", false, fmt.Errorf("%s: Git repository %s requires Git to verify effective excludes: %w", operation, root, err)
}

func rootHasGitMetadata(root string) (bool, error) {
	if os.Getenv("GIT_DIR") != "" || os.Getenv("GIT_WORK_TREE") != "" {
		return true, nil
	}
	current, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(current)
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("root %q is not a directory", current)
	}
	for {
		if _, err := os.Lstat(filepath.Join(current, ".git")); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false, nil
		}
		current = parent
	}
}

func verifyNoSymlinkPath(path string) error {
	volume := filepath.VolumeName(path)
	root := string(filepath.Separator)
	if volume != "" {
		root = volume + string(filepath.Separator)
	}
	rel, err := filepath.Rel(root, filepath.Clean(path))
	if err != nil {
		return err
	}
	current := root
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink component %q", current)
		}
	}
	return nil
}

func containsGitIgnorePattern(contents []byte, pattern string) bool {
	for _, line := range bytes.Split(contents, []byte("\n")) {
		if strings.TrimSpace(string(line)) == pattern {
			return true
		}
	}
	return false
}
