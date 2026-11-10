# Public and private repositories

tidydots can combine a primary configuration repository with an optional,
machine-local private repository. Each repository keeps its own `tidydots.yaml`
and template-state database. The merged application list is a view only; edits
are written to the document that owns the selected entry.

## Independent repositories

Register an existing private checkout with the supported `init --private`
workflow. Registration never clones a repository. The directory must already
contain a valid v3 `tidydots.yaml`. If the private checkout is unavailable,
select public-only operation rather than silently continuing with incomplete
configuration. `--dir` remains isolated unless paired explicitly with
`--private-dir`; `--public-only` and `--private-dir` are mutually exclusive.

Public and private storage must be independent Git repositories. tidydots
rejects linked worktrees, shared canonical Git object directories, and object
alternates or environment overrides that prevent proving storage independence.
A commit made through shared storage could otherwise write private objects into
the public repository. This check is local and does not inspect remotes or
contact a server. Non-Git directories remain usable where Git is not used; an
existing Git repository that cannot be inspected fails closed. Relevant Git
storage environment overrides are rejected even when the config roots lack
physical `.git` metadata, since Git may still resolve them to shared storage.

An application may contribute entries in both repositories. Entry names must
remain unique across the merged application. A package belongs to one scope and
is not implicitly merged or moved when an entry changes owner. App descriptions
and `when` expressions remain scope-specific; saving metadata or renaming one
contribution preserves each repository's own condition. Moving an entry into an
existing app contribution still requires compatible trimmed app conditions.

## Moving an entry

The Public/Private toggle changes ownership only when the move is applied. Review
the move summary first. tidydots checks both complete manifests, including
currently inactive definitions, for shared sources and target collisions. A
cross-repository target remains reserved even when its app or entry `when`
condition is false on this machine. This prevents an active whole-folder
deployment from adopting bytes owned by an inactive private entry. Nested or
overlapping definitions inside one repository remain permitted for existing
same-owner folder composition; only cross-repository overlaps are rejected.

Moves preserve the app name and compatible trimmed app condition. The move
summary shows the app and entry conditions, old/new entry identity, and the
resulting setup check/run commands (including previous values when they are
changed). A new app contribution inherits only the name and `when`; it does not
inherit package settings or unrelated metadata. Setup entries move as YAML only:
their checks and commands are never run as part of relocation.

Moves support both whole-folder entries (`files` omitted or empty) and explicit
file selections. They require static, repository-contained source paths.
When an explicitly selected path is a directory, the move snapshots and verifies
its entire subtree. A new, missing, or changed member blocks publication; a new
member after commit is left untouched and keeps the transaction journal pending.
Other siblings outside the selected directory are not part of that move.
Shared/dynamic ownership, escaping or special files, absolute source symlinks,
hard-linked regular files, and conflicting simultaneous source/selection/target/
method/kind changes are rejected. A valid entry rename may accompany an ownership
move. Save source-path or deployment-method edits separately, then retry the
ownership change. Selected templates carry only verified, attributable rendered
artifacts and render history. Artifact attribution uses captured symlink aliases
or source-matched render history; if a generated-artifact path exists but its
ownership cannot be proven, the move is rejected rather than guessing from its
suffix or leaving possible recovery data behind. Copy deployments keep their live
target bytes unchanged; any existing
`<target>.tidydots.bak` remains at that target and is not a repository artifact
that the move relocates.

Only verified current-machine symlink deployments are retargeted. Missing
targets remain missing, unrelated files and links are not adopted, and a
symlink-parent traversal into a source repository is rejected. If a verified
live link requires privileged replacement, the move fails before mutation when
the available workspace operation cannot safely perform that update.

## Recovery and local ignore rules

Moves and multi-document app edits use a protected journal in the local tidydots
state directory, outside both repositories. Source bytes and rollback snapshots
are never printed in logs. Transactions use local Git excludes for the reserved
`.tidydots-txn/` staging directory, transaction config files, `.tidydots.db`,
and generated template artifacts. `tidydots init --private` installs these rules
in both registered roots when they are Git repositories. Non-Git config
directories remain usable. Its `--dry-run` verifies effective Git ignores
without writing and reports missing rules rather than claiming they are
installed. An embedding integration can call
`workspace.RegisterGitExcludes(root)` to install the local rules; it performs no
remote operations.

An incomplete transaction blocks mutation-oriented workspace loading and
mutations for the affected roots. Read-only views and dry-run previews can still
load valid manifests, but they cannot authorize a write. Inspect the transaction ID from the error and run the
documented `tidydots recover <id>` command. Recovery without `--dry-run` rolls
back an uncommitted transaction or completes cleanup after the commit marker;
repeating recovery is safe. With `--dry-run`, it inspects the journal without
changing files. If an editor changed a manifest, source file, rendered result,
or live link during a transaction, recovery stops rather than overwriting that
change. Keep the journal until the reported conflict is resolved.

The move rechecks the selected source set, generated-template artifacts, render
history, and effective Git ignore rules while holding the repository mutation
locks. If a new attributable conflict/render artifact or render-history row is
added after planning or after commit, recovery retains the source and journal
instead of claiming or deleting the new data. Do not work around this by
manually removing the journal.

Recovery is not forensic erasure. Existing Git objects, prior database copies,
and external copy-target backups are not rewritten or removed by ownership
moves. Git-history cleanup is a separate operation.
