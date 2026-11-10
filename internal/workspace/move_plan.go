package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AntoineGS/tidydots/internal/config"
	"github.com/AntoineGS/tidydots/internal/platform"
	"github.com/AntoineGS/tidydots/internal/state"
)

// MoveRequest identifies one source-owned entry and its destination owner.
type MoveRequest struct {
	Ref         EntryRef
	Destination Role
	Updated     config.SubEntry
}

// MovePlan is an immutable summary of a preflighted ownership move. Private
// snapshots are deliberately kept out of exported/loggable fields.
type MovePlan struct {
	Request        MoveRequest
	SourceRef      EntryRef
	DestinationRef EntryRef
	Summary        []string
	data           *moveData
}

type moveData struct {
	source, destination         *Repository
	app                         config.Application
	entry                       config.SubEntry
	newEntry                    config.SubEntry
	sourceFiles                 []fileSnapshot
	targets                     []linkSnapshot
	publicConfig, privateConfig []byte
	publicHash, privateHash     string
	backupRel                   string
	stateKeys                   []string
	stateRecords                []state.RenderRecord
	platform                    *platform.Platform
}

type fileSnapshot struct {
	rel  string
	mode os.FileMode
	data []byte
	link string
	dir  bool
}
type linkSnapshot struct{ path, old, next, forwardTemp, rollbackTemp string }

// PlanMove checks all ownership and path constraints without mutating disk.
//
//nolint:gocyclo // ordered read-only preflight rejects unsafe ownership transitions.
func (w *Workspace) PlanMove(ctx context.Context, req MoveRequest, plat *platform.Platform) (*MovePlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.Ref.Repository == req.Destination {
		return nil, fmt.Errorf("entry already belongs to %s repository", req.Destination)
	}
	if req.Destination != Public && req.Destination != Private {
		return nil, fmt.Errorf("invalid destination role %q", req.Destination)
	}
	if plat == nil {
		return nil, fmt.Errorf("platform is required to plan a move")
	}
	source, app, oldEntry, err := w.Lookup(req.Ref)
	if err != nil {
		return nil, err
	}
	destination, err := w.Repository(req.Destination)
	if err != nil {
		return nil, err
	}
	if req.Updated.Name == "" {
		req.Updated = oldEntry
	}
	if req.Updated.Backup != oldEntry.Backup || !sameStrings(req.Updated.Files, oldEntry.Files) || req.Updated.EffectiveMethod() != oldEntry.EffectiveMethod() || !sameStringsMap(req.Updated.Targets, oldEntry.Targets) || req.Updated.IsConfig() != oldEntry.IsConfig() {
		return nil, fmt.Errorf("save source, file selection, method, target, or entry-kind changes separately before changing ownership")
	}
	if err := validateDestinationName(w, req.Ref, destination, req.Updated.Name); err != nil {
		return nil, err
	}
	var destinationApp *config.Application
	for i := range destination.Config.Applications {
		if destination.Config.Applications[i].Name == app.Name {
			destinationApp = &destination.Config.Applications[i]
			break
		}
	}
	if destinationApp != nil && strings.TrimSpace(app.When) != strings.TrimSpace(destinationApp.When) {
		return nil, fmt.Errorf("application conditions differ: %s has %q, %s has %q", source.Role, strings.TrimSpace(app.When), destination.Role, strings.TrimSpace(destinationApp.When))
	}
	newEntry := req.Updated
	data := &moveData{source: source, destination: destination, app: app, entry: oldEntry, newEntry: newEntry, platform: plat}
	publicRepo, err := w.Repository(Public)
	if err != nil {
		return nil, err
	}
	privateRepo, err := w.Repository(Private)
	if err != nil {
		return nil, err
	}
	data.publicConfig, err = readFreshConfigSnapshot(publicRepo)
	if err != nil {
		return nil, err
	}
	data.privateConfig, err = readFreshConfigSnapshot(privateRepo)
	if err != nil {
		return nil, err
	}
	data.publicHash = digest(data.publicConfig)
	data.privateHash = digest(data.privateConfig)
	if oldEntry.IsSetup() {
		plan := newMovePlan(req, app, oldEntry, source, destination, nil, data)
		if err := w.validateTargetsFromRoleMove(plan); err != nil {
			return nil, err
		}
		return plan, nil
	}
	backup, err := staticBackup(source, oldEntry.Backup)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(source.Root, backup)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("source backup escapes %s repository", source.Role)
	}
	data.backupRel = rel
	if filepath.IsAbs(oldEntry.Backup) {
		data.newEntry.Backup = "./" + filepath.ToSlash(rel)
	}
	if err := checkExclusiveSources(w, req.Ref, backup, oldEntry); err != nil {
		return nil, err
	}
	data.sourceFiles, err = snapshotOwned(backup, oldEntry.Files)
	if err != nil {
		return nil, err
	}
	if len(data.sourceFiles) == 0 {
		return nil, fmt.Errorf("move source has no files: %s", backup)
	}
	if err := planRenderTransfer(ctx, data, plat); err != nil {
		return nil, err
	}
	data.sourceFiles, err = expandAttributedTemplateArtifacts(
		backup,
		data.sourceFiles,
		filepath.ToSlash(rel),
		oldEntry.EffectiveMethod(),
		data.stateRecords,
		data.stateKeys,
	)
	if err != nil {
		return nil, err
	}
	if err := checkExpandedSourceOwnership(w, req.Ref, backup, data.sourceFiles); err != nil {
		return nil, err
	}
	destinationFiles := snapshotRelativePaths(data.sourceFiles)
	if oldEntry.IsFolder() {
		destinationFiles = nil
	}
	if err := verifyDestinationFree(destination.Root, rel, destinationFiles); err != nil {
		return nil, err
	}
	if err := verifyMoveDestinationParents(destination.Root, rel, destinationFiles); err != nil {
		return nil, err
	}
	if err := checkGeneratedIgnoresAt(ctx, source.Root, filepath.ToSlash(rel), data.sourceFiles); err != nil {
		return nil, err
	}
	if err := checkGeneratedIgnoresAt(ctx, destination.Root, filepath.ToSlash(rel), data.sourceFiles); err != nil {
		return nil, err
	}
	if oldEntry.EffectiveMethod() == config.MethodSymlink {
		data.targets, err = findVerifiedLinks(oldEntry, source, destination, plat, data.sourceFiles)
		if err != nil {
			return nil, err
		}
	}
	plan := newMovePlan(req, app, oldEntry, source, destination, data.targets, data)
	if err := w.validateTargetsFromRoleMove(plan); err != nil {
		return nil, err
	}
	return plan, nil
}

func snapshotRelativePaths(files []fileSnapshot) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		if file.rel != "." {
			paths = append(paths, file.rel)
		}
	}
	return paths
}

func readFreshConfigSnapshot(repo *Repository) ([]byte, error) {
	if repo.ConfigDigest == "" {
		return nil, fmt.Errorf("%s repository has no loaded config fingerprint; reload before moving", repo.Role)
	}
	data, err := os.ReadFile(repo.ConfigPath)
	if err != nil {
		return nil, err
	}
	if digest(data) != repo.ConfigDigest {
		return nil, fmt.Errorf("%s configuration changed outside tidydots; reload before moving", repo.Role)
	}
	return data, nil
}

func repositoryByRole(w *Workspace, role Role) *Repository { r, _ := w.Repository(role); return r }
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func sameStringsMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func pathsOverlap(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	return a == b || pathIsWithin(a, b) || pathIsWithin(b, a)
}

func validateDestinationName(w *Workspace, ref EntryRef, destination *Repository, name string) error {
	for _, app := range destination.Config.Applications {
		if app.Name != ref.App {
			continue
		}
		for _, e := range app.Entries {
			if e.Name == name {
				return fmt.Errorf("destination already defines %s/%s", ref.App, name)
			}
		}
	}
	apps, err := w.Applications()
	if err != nil {
		return err
	}
	for _, app := range apps {
		if app.Name == ref.App {
			continue
		}
		for _, e := range app.Entries {
			if e.Ref.Repository == destination.Role && e.Ref.App == ref.App && e.Ref.Name == name {
				return fmt.Errorf("destination entry name is already in use")
			}
		}
	}
	return nil
}

func staticBackup(repo *Repository, raw string) (string, error) {
	if raw == "" || strings.Contains(raw, "{{") || strings.ContainsAny(raw, "$%") {
		return "", fmt.Errorf("move requires a static backup path; normalize templated or environment-dependent paths first")
	}
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(repo.Root, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	if err := verifyPathParents(repo.Root, path); err != nil {
		return "", fmt.Errorf("backup path is unsafe: %w", err)
	}
	rel, err := filepath.Rel(repo.Root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("backup path %q escapes its repository", raw)
	}
	return path, nil
}

func verifyMoveDestinationParents(root, backupRel string, files []string) error {
	backup := filepath.Join(root, backupRel)
	if err := verifyPathParents(root, backup); err != nil {
		return fmt.Errorf("destination backup path is unsafe: %w", err)
	}
	if info, err := os.Lstat(backup); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination backup path may not be a symlink: %s", backup)
		}
		if !info.IsDir() && len(files) > 0 {
			return fmt.Errorf("destination backup path is not a directory: %s", backup)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, file := range files {
		path := filepath.Join(backup, file)
		if err := verifyPathParents(root, path); err != nil {
			return fmt.Errorf("destination path for %q is unsafe: %w", file, err)
		}
	}
	return nil
}

func newMovePlan(req MoveRequest, app config.Application, old config.SubEntry, src, dst *Repository, links []linkSnapshot, data *moveData) *MovePlan {
	return &MovePlan{
		Request:        req,
		SourceRef:      req.Ref,
		DestinationRef: EntryRef{Repository: dst.Role, App: app.Name, Name: req.Updated.Name},
		Summary:        moveSummary(req, app, old, req.Updated, src, dst, links),
		data:           data,
	}
}

func moveSummary(req MoveRequest, app config.Application, oldEntry, entry config.SubEntry, src, dst *Repository, links []linkSnapshot) []string {
	resultingName := req.Updated.Name
	out := []string{
		fmt.Sprintf("Move %s/%s from %s to %s", req.Ref.App, oldEntry.Name, src.Role, dst.Role),
		fmt.Sprintf("Entry identity: %s -> %s", oldEntry.Name, resultingName),
		fmt.Sprintf("Application condition: %s", conditionLabel(app.When)),
		fmt.Sprintf("Entry condition: %s -> %s", conditionLabel(oldEntry.When), conditionLabel(entry.When)),
		fmt.Sprintf("Source: %s -> %s", filepath.Join(src.Root, oldEntry.Backup), filepath.Join(dst.Root, entry.Backup)),
	}
	if entry.IsSetup() {
		out = append(out, "Setup definition metadata only; setup commands will not run")
		out = appendSetupCommands(out, "Resulting check", entry.Check)
		out = appendSetupCommands(out, "Resulting run", entry.Run)
		if !sameStringsMap(oldEntry.Check, entry.Check) {
			out = appendSetupCommands(out, "Previous check", oldEntry.Check)
		}
		if !sameStringsMap(oldEntry.Run, entry.Run) {
			out = appendSetupCommands(out, "Previous run", oldEntry.Run)
		}
		return out
	}
	if entry.IsConfig() && entry.EffectiveMethod() == config.MethodCopy {
		out = append(out, "Copy deployment targets remain unchanged; external .tidydots.bak recovery files remain at live targets")
	}
	if len(links) > 0 {
		out = append(out, fmt.Sprintf("Retarget %d verified current-machine symlink(s)", len(links)))
	}
	return out
}

func conditionLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<none>"
	}
	return strings.TrimSpace(value)
}

func appendSetupCommands(out []string, label string, commands map[string]string) []string {
	if len(commands) == 0 {
		out = append(out, label+" commands: <none>")
		return out
	}
	keys := make([]string, 0, len(commands))
	for key := range commands {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out = append(out, fmt.Sprintf("%s command (%s): %s", label, key, commands[key]))
	}
	return out
}
