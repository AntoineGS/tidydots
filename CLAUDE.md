# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
# Build
go build ./cmd/dot-manager

# Run tests
go test ./...

# Run tests with verbose output
go test ./... -v

# Run a single package's tests
go test ./internal/manager/...
```

## Architecture

**dot-manager** is a cross-platform dotfile management tool written in Go. It manages configuration files through symlinks, clones git repositories, and handles package installation across multiple package managers.

### Core Components

- **cmd/dot-manager/main.go** - Cobra CLI entry point defining all commands (init, restore, backup, list, install, list-packages)
- **internal/config/** - Two-level YAML configuration: app config (`~/.config/dot-manager/config.yaml`) and repo config (`dot-manager.yaml`)
- **internal/config/entry.go** - Unified Entry type supporting config (symlinks) and git (repo clones)
- **internal/config/filter.go** - Filter system with include/exclude conditions for os, distro, hostname, user
- **internal/manager/** - Core operations (backup, restore, adopt, list) with platform-aware path selection
- **internal/platform/** - OS/distro detection (Linux/Windows), root/sudo detection, hostname/user detection
- **internal/tui/** - Bubble Tea-based interactive terminal UI with Lipgloss styling
- **internal/packages/** - Multi-package-manager support (pacman, yay, paru, apt, dnf, brew, winget, scoop, choco)

### Key Patterns

- **Unified entries**: Single `entries` array with `root: true` flag instead of separate `paths`/`root_paths`
- **Entry types**: Config entries (have `backup`) manage symlinks; Git entries (have `repo`) clone repositories
- **Filter-based selection**: Entries filtered by os, distro, hostname, user with regex support
- **Symlink-based restoration**: Configs are symlinked from the dotfiles repo rather than copied
- **Dry-run mode**: All operations support `-n` flag for safe preview
- **Table-driven tests**: Tests use `t.TempDir()` for filesystem isolation

### Configuration Format (dot-manager.yaml)

```yaml
version: 2
backup_root: "."
default_manager: "pacman"
manager_priority: ["yay", "paru", "pacman"]

entries:
  # Config entry (symlink management)
  - name: "config-name"
    files: []  # Empty = entire folder
    backup: "./path/to/backup"
    targets:
      linux: "~/.config/app"
      windows: "~/AppData/Local/app"

  # Git entry (repository clone)
  - name: "repo-name"
    repo: "https://github.com/user/repo.git"
    branch: "main"
    targets:
      linux: "~/path/to/clone"

  # Entry with package
  - name: "package"
    package:
      managers:
        pacman: "package-name"

  # Root entry with filter
  - name: "system-config"
    root: true
    backup: "./system"
    targets:
      linux: "/etc/app"
    filters:
      - include:
          distro: "arch"
```

### CLI Flags

- `-d, --dir` - Override configuration directory
- `-o, --os` - Override OS detection (linux/windows)
- `-n, --dry-run` - Preview without changes
- `-v, --verbose` - Verbose output
- `-i` - Interactive TUI mode (for restore, backup, install)
