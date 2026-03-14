package config

import (
	"fmt"
	"strings"
)

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

// isTemplatePath returns true if the path contains Go template delimiters,
// meaning it will be rendered later and should not be validated now.
func isTemplatePath(path string) bool {
	return strings.Contains(path, "{{")
}

// validateEntryPaths validates all path fields on a sub-entry, skipping
// paths that contain template expressions (they will be rendered later).
func validateEntryPaths(appName string, entry SubEntry) []error {
	var errs []error

	// Validate backup path
	if entry.Backup != "" && !isTemplatePath(entry.Backup) {
		if err := ValidatePath(entry.Backup); err != nil {
			errs = append(errs, NewFieldError(
				fmt.Sprintf("%s/%s", appName, entry.Name),
				"backup", entry.Backup, err,
			))
		}
	}

	// Validate target paths
	for os, target := range entry.Targets {
		if target == "" || isTemplatePath(target) {
			continue
		}

		if err := ValidatePath(target); err != nil {
			errs = append(errs, NewFieldError(
				fmt.Sprintf("%s/%s", appName, entry.Name),
				fmt.Sprintf("targets[%s]", os), target, err,
			))
		}
	}

	return errs
}

// validateGitPackagePaths validates target paths on a git package configuration.
func validateGitPackagePaths(appName string, gitPkg *GitPackage) []error {
	var errs []error

	for os, target := range gitPkg.Targets {
		if target == "" || isTemplatePath(target) {
			continue
		}

		if err := ValidatePath(target); err != nil {
			errs = append(errs, NewFieldError(
				appName,
				fmt.Sprintf("package.managers.git.targets[%s]", os), target, err,
			))
		}
	}

	return errs
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

			// Validate paths on each entry
			errs = append(errs, validateEntryPaths(app.Name, entry)...)
		}

		// Validate git package target paths
		if app.Package != nil {
			if gitPkg, ok := app.Package.GetGitPackage(); ok {
				errs = append(errs, validateGitPackagePaths(app.Name, gitPkg)...)
			}
		}
	}

	return errs
}
