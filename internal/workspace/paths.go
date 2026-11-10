package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	tmpl "github.com/AntoineGS/tidydots/internal/template"
)

type targetClaim struct {
	paths  []string
	owner  EntryRef
	active bool
}

// ValidateTargets is an alias for ValidateCrossRepositoryTargets.
// Same-repository overlaps are intentionally permitted for folder composition.
func (w *Workspace) ValidateTargets(plat *platform.Platform) error {
	return w.ValidateCrossRepositoryTargets(plat)
}

// ValidateCrossRepositoryTargets checks current-platform deployments for
// cross-repository target collisions. Ownership claims reserve targets
// regardless of current conditions: an inactive entry still owns its path.
// Overlaps within one repository are permitted for intentional folder
// composition.
func (w *Workspace) ValidateCrossRepositoryTargets(plat *platform.Platform) error {
	if plat == nil {
		return fmt.Errorf("platform is required to validate workspace targets")
	}
	renderer := tmpl.NewEngine(tmpl.NewContextFromPlatform(plat))
	claims := make([]targetClaim, 0)
	for _, repository := range w.Repositories {
		var err error
		claims, err = validateRepositoryTargets(repository, plat, renderer, claims)
		if err != nil {
			return err
		}
	}
	return nil
}

func validateRepositoryTargets(repository *Repository, plat *platform.Platform, renderer config.PathRenderer, claims []targetClaim) ([]targetClaim, error) {
	for _, app := range repository.Config.Applications {
		appActive := config.EvaluateWhen(app.When, renderer)
		for _, entry := range app.Entries {
			newClaims, err := entryTargetClaims(repository, app, entry, plat, renderer, appActive)
			if err != nil {
				return nil, err
			}
			for _, next := range newClaims {
				if err := appendTargetClaim(&claims, next); err != nil {
					return nil, err
				}
			}
		}
	}
	return claims, nil
}

func entryTargetClaims(repository *Repository, app config.Application, entry config.SubEntry, plat *platform.Platform, renderer config.PathRenderer, appActive bool) ([]targetClaim, error) {
	if !entry.IsConfig() {
		return nil, nil
	}
	target := entry.GetTarget(plat.OS)
	if target == "" {
		return nil, nil
	}
	expanded, err := renderPath(target, plat, renderer)
	if err != nil {
		return nil, fmt.Errorf("expand target for %s/%s (%s): %w", app.Name, entry.Name, repository.Role, err)
	}
	base, err := filepath.Abs(expanded)
	if err != nil {
		return nil, fmt.Errorf("resolve target for %s/%s (%s): %w", app.Name, entry.Name, repository.Role, err)
	}
	base = filepath.Clean(base)
	owner := EntryRef{Repository: repository.Role, App: app.Name, Name: entry.Name}
	active := appActive && config.EvaluateWhen(entry.When, renderer)
	claims, err := deploymentClaims(base, entry, owner)
	if err != nil {
		return nil, fmt.Errorf("plan targets for %s/%s (%s): %w", app.Name, entry.Name, repository.Role, err)
	}
	for i := range claims {
		claims[i].active = active
	}
	return claims, nil
}

func appendTargetClaim(claims *[]targetClaim, next targetClaim) error {
	for _, current := range *claims {
		if targetsOverlap(current, next) {
			// One repository may intentionally compose nested folder entries;
			// the privacy boundary is ownership across repository roots.
			if current.owner.Repository == next.owner.Repository {
				continue
			}
			return fmt.Errorf("target conflict between %s/%s (%s) at %q and %s/%s (%s) at %q", current.owner.App, current.owner.Name, current.owner.Repository, current.paths[0], next.owner.App, next.owner.Name, next.owner.Repository, next.paths[0])
		}
	}
	*claims = append(*claims, next)
	return nil
}

func deploymentClaims(base string, entry config.SubEntry, owner EntryRef) ([]targetClaim, error) {
	if entry.IsFolder() {
		paths, err := destinationPaths(base)
		if err != nil {
			return nil, err
		}
		return []targetClaim{{paths: paths, owner: owner}}, nil
	}
	claims := make([]targetClaim, 0, len(entry.Files))
	for _, file := range entry.Files {
		destination := filepath.Clean(filepath.Join(base, managerTargetName(file)))
		paths, err := destinationPaths(destination)
		if err != nil {
			return nil, err
		}
		claims = append(claims, targetClaim{paths: paths, owner: owner})
	}
	return claims, nil
}

// managerTargetName mirrors RestoreFiles' literal path joining and selected
// template output naming. The manager does not template-render or expand
// environment/home expressions in files: ordinary filenames pass through
// filepath.Join, while selected .tmpl files use newSelectedTemplate's cleaned
// relative path and suffix-free target alias.
func managerTargetName(file string) string {
	if !tmpl.IsTemplateFile(file) {
		return file
	}
	clean := filepath.Clean(file)
	return filepath.Join(filepath.Dir(clean), tmpl.TargetName(clean))
}

// destinationPaths retains both the requested destination namespace and the
// path reached by resolving existing symlink parents. Keeping both catches
// collisions through directory aliases without treating a final folder
// symlink's referent as the folder entry being replaced.
func destinationPaths(path string) ([]string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	requested := filepath.Clean(absolute)
	resolved, err := resolveTargetParents(requested)
	if err != nil {
		return nil, err
	}
	if targetComparisonPath(requested) == targetComparisonPath(resolved) {
		return []string{requested}, nil
	}
	return []string{requested, resolved}, nil
}

func renderPath(path string, plat *platform.Platform, renderer config.PathRenderer) (string, error) {
	if strings.Contains(path, "{{") {
		rendered, err := renderer.RenderString("workspace target", path)
		if err != nil {
			return "", err
		}
		path = rendered
	}
	return config.ExpandPath(path, plat.EnvVars), nil
}

// resolveTargetAncestors resolves an existing path or, if it does not exist,
// its nearest existing ancestor. This is used for entry target directories.
func resolveTargetAncestors(path string) (string, error) {
	path = filepath.Clean(path)
	ancestor := path
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("no existing ancestor for %q", path)
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = parent
	}
}

// resolveTargetParents canonicalizes a file target's parent directory while
// retaining the final path component, which may be a symlink being replaced.
func resolveTargetParents(path string) (string, error) {
	parent, err := resolveTargetAncestors(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}

func targetsOverlap(left, right targetClaim) bool {
	for _, leftPath := range left.paths {
		for _, rightPath := range right.paths {
			leftPath = targetComparisonPath(leftPath)
			rightPath = targetComparisonPath(rightPath)
			if leftPath == rightPath || pathIsWithin(leftPath, rightPath) || pathIsWithin(rightPath, leftPath) {
				return true
			}
		}
	}
	return false
}

func pathIsWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func targetComparisonPath(path string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(filepath.Clean(path))
	}
	return filepath.Clean(path)
}
