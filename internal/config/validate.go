package config

import (
	"fmt"
	"strings"
)

// ValidationError represents a validation error for an entry
type ValidationError struct {
	EntryName string
	Field     string
	Message   string
}

func (e ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("entry '%s': %s - %s", e.EntryName, e.Field, e.Message)
	}

	return fmt.Sprintf("entry '%s': %s", e.EntryName, e.Message)
}

// ValidatePath checks a path for potential security issues.
// It returns an error if the path contains null bytes or suspicious patterns.
func ValidatePath(path string) error {
	// Check for null bytes
	if strings.ContainsRune(path, '\x00') {
		return fmt.Errorf("path contains null byte")
	}

	// Check for path traversal patterns
	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains traversal pattern '..'")
	}

	return nil
}

// ValidateConfig validates the entire config including all applications
func ValidateConfig(cfg *Config) []error {
	var errs []error

	// Validate version
	if cfg.Version != 3 {
		errs = append(errs, fmt.Errorf("%w: %d (expected 3)", ErrUnsupportedVersion, cfg.Version))
	}

	// Validate applications
	appNames := make(map[string]bool)

	for _, app := range cfg.Applications {
		if app.Name == "" {
			errs = append(errs, fmt.Errorf("%w: application has empty name", ErrInvalidConfig))

			continue
		}

		if appNames[app.Name] {
			errs = append(errs, fmt.Errorf("%w: duplicate application name %q", ErrInvalidConfig, app.Name))
		}

		appNames[app.Name] = true

		// Validate sub-entries
		subNames := make(map[string]bool)
		for _, entry := range app.Entries {
			if entry.Name == "" {
				errs = append(errs, fmt.Errorf("%w: application %q has entry with empty name", ErrInvalidConfig, app.Name))

				continue
			}

			if subNames[entry.Name] {
				errs = append(errs, fmt.Errorf("%w: application %q has duplicate entry name %q", ErrInvalidConfig, app.Name, entry.Name))
			}

			subNames[entry.Name] = true
		}
	}

	return errs
}
