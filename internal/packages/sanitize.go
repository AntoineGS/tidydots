package packages

import (
	"fmt"
	"strings"
)

// validateURLScheme checks that a URL uses a safe scheme.
// It allows http:// and https:// schemes, and bare paths (no scheme) which
// are used by git for local repository clones.
// It rejects dangerous schemes like file://, ftp://, gopher://, dict://, and
// git's ext:: transport that could be exploited for local file access or
// arbitrary command execution.
func validateURLScheme(url string) error {
	lower := strings.ToLower(url)

	// Block git ext:: transport first (allows arbitrary command execution)
	if strings.HasPrefix(lower, "ext::") {
		return fmt.Errorf("URL scheme %q is not allowed: %s", "ext::", url)
	}

	// Allow http:// and https://
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return nil
	}

	// Block known-dangerous URI schemes
	for _, scheme := range []string{"file://", "ftp://", "gopher://", "dict://"} {
		if strings.HasPrefix(lower, scheme) {
			return fmt.Errorf("URL scheme %q is not allowed: %s", scheme, url)
		}
	}

	// Allow bare paths (no scheme) -- used by git for local repositories.
	// These are safe: git treats them as filesystem paths, and curl/wget
	// will reject them as invalid URLs.
	if !strings.Contains(url, "://") {
		return nil
	}

	return fmt.Errorf("URL scheme is not allowed: %s", url)
}

// escapeShellSingleQuote escapes single quotes in a string for safe
// interpolation inside a POSIX shell single-quoted string.
// Each single quote is replaced with the standard shell escape sequence:
// end-quote, backslash-quote, start-quote.
func escapeShellSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}

// escapePowerShellSingleQuote escapes single quotes in a string for safe
// interpolation inside a PowerShell single-quoted string.
// Each single quote is doubled, which is the PowerShell escape convention.
func escapePowerShellSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
