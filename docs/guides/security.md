# Security

This page covers the security model of tidydots, the risks you should be aware of, and best practices for safe usage.

## Trust model

!!! danger "tidydots trusts your configuration completely"
    When you run tidydots, it reads your `tidydots.yaml` and executes whatever it says -- creating symlinks, cloning repositories, running package install commands, and rendering templates. If you use someone else's dotfiles repository, their configuration can execute arbitrary commands on your machine.

    **Always review a third-party dotfiles repo before running `tidydots restore` or `tidydots install` against it.**

tidydots does not sandbox or restrict the operations defined in your configuration. It assumes you trust the content of your dotfiles repository the same way you trust any script you run on your machine.

## Command execution

Several package installation methods execute arbitrary shell commands:

| Method | Execution mechanism | Example |
|--------|---------------------|---------|
| `installer` | Runs the value as a shell command | `installer: "curl -sS https://... \| bash"` |
| `custom` | Runs the value as a shell command | `custom: "make install"` |
| `url` | Downloads and executes an installer script | `url: "https://example.com/install.sh"` |

!!! warning "Shell command execution"
    The `installer`, `custom`, and `url` package methods execute arbitrary shell commands via `sh -c` on Linux/macOS and `powershell -Command` on Windows. These commands run with the same privileges as the tidydots process.

    Always review these entries before running `tidydots install`. Use `--dry-run` to preview what commands will be executed:

    ```bash
    tidydots install -n -v
    ```

## Sudo operations

When `sudo: true` is set on config entries, tidydots performs symlink creation, directory removal, and other filesystem operations with elevated privileges. This is necessary for managing system-level configuration files under paths like `/etc/`.

!!! danger "Review sudo entries carefully"
    Sudo operations can modify or overwrite critical system files. A misconfigured sudo entry could:

    - Overwrite system configuration files (e.g., `/etc/hosts`, `/etc/fstab`)
    - Create symlinks in protected directories
    - Remove directories owned by root

    Always audit entries with `sudo: true` before running restore:

    ```bash
    # Preview sudo operations without making changes
    tidydots restore -n -v
    ```

## Template environment variables

Templates have access to the full process environment via the `.Env` context variable. This means any environment variable available to the tidydots process can be referenced in template expressions.

!!! warning "Avoid sensitive data in templates"
    Rendered template output is stored in two places:

    1. **`.tmpl.rendered` files** -- written to the backup directory alongside the source `.tmpl` file
    2. **`.tidydots.db` database** -- the SQLite state store keeps a copy of each pure render output for 3-way merge

    If you reference sensitive environment variables (API keys, tokens, passwords) in template expressions, those values will be persisted in these files. Avoid using sensitive variables in templates:

    ```
    # BAD -- secret will be stored in .tmpl.rendered and .tidydots.db
    export API_KEY={{ index .Env "SECRET_API_KEY" }}

    # OK -- non-sensitive system information
    export DISPLAY={{ index .Env "DISPLAY" }}
    ```

## `.tidydots.db` sensitivity

The `.tidydots.db` SQLite database stores rendered template content in its `pure_render` column. If any templates reference environment variables or other dynamic data, that data is persisted in the database.

!!! warning "Protect the state database"
    Ensure `.tidydots.db` is handled securely:

    - **Gitignore it** -- add `.tidydots.db` to your `.gitignore` so it is never committed to version control
    - **File permissions** -- the database is created with default permissions; on shared systems, consider restricting access:

        ```bash
        chmod 600 ~/dotfiles/.tidydots.db
        ```

    - **Backup awareness** -- if you back up your dotfiles directory with other tools, be aware that `.tidydots.db` may contain sensitive rendered content

    Recommended `.gitignore` entries:

    ```
    *.tmpl.rendered
    *.tmpl.conflict
    .tidydots.db
    ```

## URL security

tidydots validates URLs used in git package definitions and URL-based installers. Only the following schemes are accepted:

- `https://`
- `http://`

!!! tip "Dangerous schemes are rejected"
    Schemes such as `file://` and `ext::` are rejected to prevent local file access exploits and arbitrary command execution through git's external transport mechanism. If you encounter a URL validation error, verify that your URL uses `https://` or `http://`.

## Best practices

Follow these guidelines to use tidydots safely:

!!! tip "Security checklist"

    1. **Review third-party configs before use.** Never blindly clone and run someone else's dotfiles repository. Read their `tidydots.yaml` and inspect any `.tmpl` files, `installer`, `custom`, and `url` entries.

    2. **Use `--dry-run` to preview operations.** Before running `restore` or `install`, use the `-n` flag to see exactly what tidydots will do:

        ```bash
        tidydots restore -n -v
        tidydots install -n -v
        ```

    3. **Audit sudo entries.** Review every entry with `sudo: true` to ensure it targets the correct system paths. A typo in a sudo target path could overwrite the wrong system file.

    4. **Avoid secrets in templates.** Do not reference API keys, tokens, passwords, or other secrets via `.Env` in template expressions. Use a dedicated secrets manager instead.

    5. **Keep `.tidydots.db` gitignored.** The state database may contain rendered content from templates. Never commit it to version control.

    6. **Use HTTPS URLs.** Always prefer `https://` URLs for git repositories to ensure encrypted transport and server authentication.

    7. **Review `installer` and `custom` commands.** These fields execute arbitrary shell commands. Treat them with the same caution as any shell script you download from the internet.
