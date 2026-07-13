# Setup Entries

A **setup entry** performs a system change that config files alone cannot: enabling a
systemd unit, registering a shell completion, installing a git hook. It pairs a `check`
command (does this already hold?) with a `run` command (make it hold).

Setup entries run during `tidydots restore`, in the order they appear under `entries`.

## Example

The `vicinae` launcher ships a systemd user unit. Installing the package and symlinking
the config is not enough — the unit must also be enabled:

```yaml
  - package:
      managers:
        yay: vicinae-bin
    name: vicinae
    entries:
      - targets:
          linux: ~/.config/vicinae
        name: config
        backup: ./Linux/vicinae
        files: [settings.json]

      - name: enable-service
        check:
          linux: systemctl --user is-enabled --quiet vicinae.service
        run:
          linux: systemctl --user enable --now vicinae.service
```

On restore, tidydots runs the check. If the unit is already enabled the check exits 0 and
nothing happens. If it is not, the run command executes, and the check runs a second time
to confirm the change actually took effect.

## How it works

```
1. no `run` command for this OS  -> skip
2. `check` exits 0               -> skip, report "Set up"
3. --dry-run                     -> report what would run; never runs it
4. execute `run`                 -> a non-zero exit is an error
5. re-run `check`                -> still failing is an error
```

Step 5 catches a script that exits 0 without doing its job.

## Fields

| Field | Type | Description |
| --- | --- | --- |
| `name` | string | Required. Identifies the step in output and in the TUI. |
| `check` | map: OS → command | Required. Exit 0 means "already set up". |
| `run` | map: OS → command | Required. Runs only when `check` fails. |
| `sudo` | bool | Optional. Runs `run` with elevated privileges. `check` never uses sudo. |

A sub-entry is either a **config entry** (it has a `backup`) or a **setup entry** (it has
a `run`). It cannot be both, and a setup entry cannot declare `targets`.

Every OS listed under `run` must also be listed under `check`, and vice versa. This is
enforced at load time.

## The OS map is the platform gate

An absent OS key means the step does not apply there. The entry above has no `windows:`
key, so it is skipped entirely on Windows. No `when:` clause is needed for this — though
the application's own `when:` still gates the whole group.

## Commands

Commands run through `sh -c` on Unix and `powershell -Command` on Windows, with the
configurations repo root as the working directory. That means multi-line commands and
repo-relative script paths both work:

```yaml
      - name: install-hooks
        check:
          linux: test -x /etc/pacman.d/hooks/pkg-backup-aur.hook
        run:
          linux: sh ./Linux/pacman/install-hooks.sh
        sudo: true
```

## The check contract

**Check commands must be side-effect free and fast.**

They are executed:

- on every `tidydots restore`,
- during `--dry-run` (this is how dry-run can truthfully report whether the setup *would*
  run),
- and on every TUI state-detection pass.

`systemctl --user is-enabled --quiet vicinae.service` satisfies both requirements. A check
that mutates state, prompts, or takes seconds to return will make tidydots feel broken.

tidydots cannot enforce this — it is a contract you accept when you write a setup entry.

## No state is stored

There is no database and no marker file. The check *is* the state. If you later disable
the vicinae unit by hand, the next restore notices and re-enables it.

This is deliberate: a "we already ran this once" record would go stale the moment its
effect was undone, and the step would never repair itself.

## Security

Setup commands are arbitrary shell commands from your configuration file. Only use
configurations you trust.
