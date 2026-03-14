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
