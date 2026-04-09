package packages

import (
	"fmt"
	"regexp"
	"strings"
)

// validPackageName matches common package name characters: letters, digits, dots,
// hyphens (not leading), underscores, slashes, colons, @, and +.
var validPackageName = regexp.MustCompile(`^[a-zA-Z0-9@][a-zA-Z0-9._\-/:@+]*$`)

// ValidatePackageName checks that a package name is safe for use as a CLI argument.
// It rejects empty names, names starting with "-" (flag injection), names containing
// null bytes, and names with unexpected characters.
func ValidatePackageName(name string) error {
	if name == "" {
		return fmt.Errorf("package name must not be empty")
	}

	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("package name %q must not start with '-' (possible flag injection)", name)
	}

	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("package name %q contains null bytes", name)
	}

	if !validPackageName.MatchString(name) {
		return fmt.Errorf("package name %q contains invalid characters", name)
	}

	return nil
}

// ValidateGitBranch rejects branch names that git would misinterpret or that
// could smuggle flags into `git clone -b`. An empty branch is allowed and
// means "use the repository default".
func ValidateGitBranch(branch string) error {
	if branch == "" {
		return nil
	}
	if strings.HasPrefix(branch, "-") {
		return fmt.Errorf("git branch %q must not start with '-'", branch)
	}
	if strings.Contains(branch, "..") {
		return fmt.Errorf("git branch %q must not contain '..'", branch)
	}
	for _, r := range branch {
		if r == 0 || r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return fmt.Errorf("git branch %q contains whitespace or control characters", branch)
		}
	}
	return nil
}
