# dot-manager

A cross-platform dotfile management tool written in Go. Manage configuration files through symlinks, backup and restore operations, and install packages across multiple package managers.

## Features

- **Symlink-based restoration** - Configs are symlinked from your dotfiles repo, so edits sync immediately
- **Cross-platform** - Works on Linux and Windows with OS-specific path targets
- **Interactive TUI** - Bubble Tea-based terminal UI for guided operations
- **Package management** - Install packages across pacman, apt, dnf, brew, winget, scoop, and more
- **Smart adoption** - Automatically backs up existing configs before symlinking
- **Dry-run mode** - Preview all operations before making changes
- **Root/sudo support** - Separate path configuration for system-level files

## Installation

```bash
go install github.com/antoinegs/dot-manager/cmd/dot-manager@latest
```

Or build from source:

```bash
git clone https://github.com/antoinegs/dot-manager.git
cd dot-manager
go build ./cmd/dot-manager
```

## Quick Start

1. **Initialize** with your dotfiles repository:

   ```bash
   dot-manager init /path/to/your/dotfiles
   ```

2. **Create a configuration file** (`dot-manager.yaml`) in your dotfiles repo:

   ```yaml
   version: 1
   backup_root: "."

   paths:
     - name: "neovim"
       backup: "./nvim"
       targets:
         linux: "~/.config/nvim"
         windows: "~/AppData/Local/nvim"

     - name: "zsh"
       files: [".zshrc", ".zshenv"]
       backup: "./zsh"
       targets:
         linux: "~"
   ```

3. **Restore your configs**:

   ```bash
   dot-manager restore
   ```

## Configuration

### App Config

Located at `~/.config/dot-manager/config.yaml`, stores the path to your dotfiles repository:

```yaml
config_dir: "/path/to/your/dotfiles"
```

### Repository Config

Located in your dotfiles repo as `dot-manager.yaml`:

```yaml
version: 1
backup_root: "."

# Regular user paths
paths:
  - name: "config-name"
    files: []              # Empty = entire folder
    backup: "./backup/path"
    targets:
      linux: "~/.config/app"
      windows: "~/AppData/Local/app"

# Root/sudo paths (only used when running as root)
root_paths:
  - name: "pacman-hooks"
    files: ["my-hook.hook"]
    backup: "./Linux/pacman"
    targets:
      linux: "/usr/share/libalpm/hooks"

# Package installation
packages:
  default_manager: "pacman"
  manager_priority: ["yay", "paru", "pacman"]
  items:
    - name: "neovim"
      description: "Text editor"
      managers:
        pacman: "neovim"
        apt: "neovim"
        brew: "neovim"
      tags: ["dev", "cli"]
```

### Path Configuration

| Field | Description |
|-------|-------------|
| `name` | Display name for the configuration |
| `files` | List of specific files (empty = entire folder) |
| `backup` | Path in your dotfiles repo |
| `targets` | OS-specific target paths |

### Package Configuration

| Field | Description |
|-------|-------------|
| `name` | Package identifier |
| `description` | Optional description |
| `managers` | Package names per manager |
| `tags` | Tags for filtering |
| `custom` | Custom install commands per OS |
| `url` | URL-based installation |

## Commands

| Command | Description |
|---------|-------------|
| `dot-manager` | Launch interactive TUI |
| `dot-manager init <path>` | Initialize app configuration |
| `dot-manager restore` | Restore configs by creating symlinks |
| `dot-manager backup` | Backup configs from target locations |
| `dot-manager list` | List all configured paths |
| `dot-manager install [packages...]` | Install packages |
| `dot-manager list-packages` | Display configured packages |

### Global Flags

| Flag | Description |
|------|-------------|
| `-d, --dir` | Override configurations directory |
| `-o, --os` | Override OS detection (linux/windows) |
| `-n, --dry-run` | Preview without making changes |
| `-v, --verbose` | Enable verbose output |
| `-i, --interactive` | Use interactive TUI mode |
| `-t, --tags` | Filter packages by tags |

## Examples

```bash
# Preview restore without making changes
dot-manager restore -n

# Interactive restore
dot-manager restore -i

# Backup all configs
dot-manager backup

# Install all packages
dot-manager install

# Install specific packages
dot-manager install neovim ripgrep fzf

# Install packages by tag
dot-manager install -t dev

# List packages that would be installed
dot-manager list-packages -t cli

# Override OS for cross-platform testing
dot-manager list -o windows
```

## Supported Package Managers

| Platform | Managers |
|----------|----------|
| Arch Linux | yay, paru, pacman |
| Debian/Ubuntu | apt |
| Fedora/RHEL | dnf |
| macOS | brew |
| Windows | winget, scoop, choco |

### Custom Installation Methods

```yaml
packages:
  items:
    # Custom command
    - name: "oh-my-zsh"
      custom:
        linux: "sh -c \"$(curl -fsSL https://raw.github.com/ohmyzsh/install.sh)\""

    # URL download
    - name: "binary-tool"
      url:
        linux:
          url: "https://github.com/example/releases/tool"
          command: "sudo install {file} /usr/local/bin/"
```

## How It Works

### Restore Process

1. If backup exists and target doesn't exist: create symlink
2. If backup doesn't exist but target does: **adopt** (move target to backup, then symlink)
3. If already symlinked: skip

### Backup Process

Copies files from target locations to the backup directory. Skips files that are already symlinks to avoid duplicating restored configs.

## License

MIT

