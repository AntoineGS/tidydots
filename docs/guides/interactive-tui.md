# Interactive TUI

tidydots includes an interactive terminal UI built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and styled with [Lipgloss](https://github.com/charmbracelet/lipgloss). The TUI provides a visual way to browse, edit, and manage your dotfiles configuration without memorizing CLI commands.

## Launching the TUI

There are two ways to start the interactive interface:

```bash
# Launch directly (no arguments)
tidydots

# Or use the -i flag with any command
tidydots restore -i
tidydots backup -i
tidydots install -i
```

Running `tidydots` with no arguments opens the full TUI experience. Using `-i` with a specific command opens the TUI focused on that operation.

## Main screen

The main screen displays a table view of all your applications and their entries. Each row shows:

- **Application name** and description
- **Entry names** (configs and packages) nested under their application
- **Status indicators** showing the current state of each entry

### Status indicators

| Status | Meaning |
|--------|---------|
| Ready | Backup exists, target does not -- ready to create symlink |
| Linked | Symlink is already in place and correct |
| Adopt | Target exists but backup does not -- can adopt the existing file |
| Missing | Neither backup nor target exist |
| Outdated | Symlink exists but template source has changed since last render |
| Modified | Symlink exists but the rendered file has been manually edited since last render |

## Navigation

tidydots uses vim-style keybindings alongside arrow keys for navigation.

### Core keybindings

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up |
| `↓` / `j` | Move down |
| `←` / `h` | Collapse application row |
| `→` / `l` / `enter` | Expand application row (show sub-entries) |
| `e` | Edit selected application or entry |
| `esc` | Go back or cancel (see [priority](#clearing-selections)) |
| `tab` / `space` | Toggle selection |
| `/` | Search and filter |
| `f` | Toggle filter (show/hide apps excluded by `when` expressions) |
| `s` / `ctrl+s` | Save changes |
| `i` | Context-sensitive: install package (on app row) or view diff (on modified entry) |
| `p` | Edit package dependencies (in package form) |
| `d` / `delete` / `backspace` | Delete selected item |
| `q` | Quit |

### Adding items

| Key | Action |
|-----|--------|
| `A` | Add a new application |
| `a` | Add a new sub-entry to the current application |

### Sorting

Press a sort key to sort by that column. Press the same key again to reverse the direction.

| Key | Sort by |
|-----|---------|
| `n` | Name |
| `s` | Status |
| `p` | Path |

### Search and filter

Press `/` to enter search mode. Type to filter applications and entries by name, description, target paths, or backup paths. The list updates in real time as you type. Press `enter` to confirm or `esc` to exit search mode (your selections are preserved).

Press `f` to toggle the filter. When enabled (the default), applications that do not match their `when` expression on the current machine are hidden. When disabled, all applications are shown regardless of `when` conditions.

### Mouse support

| Input | Action |
|-------|--------|
| Left click | Move cursor to clicked row |
| Right click | Move cursor and toggle selection |
| Scroll wheel | Scroll up/down by 3 rows |

## Two-phase editing

All text fields in the TUI use a two-phase editing approach. This prevents accidental edits while navigating.

### Phase 1: Navigation mode

When you navigate to a text field, it is **focused** but **not editable**. You can see the field is highlighted, but typing does not modify its value. Use `↑/k` and `↓/j` to move between fields.

### Phase 2: Edit mode

Press `enter` or `e` to enter edit mode. The field becomes active and shows a text cursor. Type to modify the value.

- Press `enter` to **save** your changes and return to navigation mode
- Press `esc` to **cancel** your changes and return to navigation mode

!!! info
    This two-phase pattern applies to all editable fields: name, description, targets, backup path, repository URL, branch, file list items, and `when` expressions.

## Multi-selection

The TUI supports selecting multiple items for batch operations.

### Selecting items

| Key | Action |
|-----|--------|
| `tab` / `space` | Toggle selection on the current item |

- **Selecting an application** automatically selects all its visible sub-entries (configs and packages)
- **Deselecting an application** deselects all its sub-entries
- **Sub-entries** can be individually toggled even when their parent application is not selected

### Selection banner

When items are selected, a banner appears at the top of the screen showing the count:

```
2 app(s), 5 item(s) selected
```

Selections persist across search filtering, screen navigation, and edit operations.

### Clearing selections

Press `esc` to clear all selections. The `esc` key follows a priority system:

1. If in search mode, `esc` exits search first (selections are kept)
2. If selections exist, `esc` clears all selections
3. Otherwise, `esc` returns to the previous screen

## Batch operations

With items selected, you can perform operations on all of them at once. Each batch operation follows a three-screen flow.

### Available operations

| Key | Operation | Description |
|-----|-----------|-------------|
| `r` | Restore | Create symlinks for all selected config entries |
| `i` | Install | Install packages for all selected applications |
| `d` | Delete | Remove configs and packages for all selected items |

### Three-screen flow

Every batch operation proceeds through three screens:

**1. Select screen (main screen)**

Browse and select the items you want to operate on. Use `tab` or `space` to toggle selections.

**2. Summary screen**

After pressing an operation key (`r`, `i`, or `d`), a summary screen appears showing exactly what will be changed. Review the list of operations, then:

- Press `y` or `enter` to confirm and proceed
- Press `n` or `esc` to cancel and return to the main screen

**3. Progress screen**

Once confirmed, a progress screen shows real-time feedback as each operation executes. A progress bar tracks completion. When finished, press any key to return to the main screen.

!!! tip
    The global `-n` (dry-run) flag works with batch operations too. When dry-run is enabled, the progress screen shows what *would* happen without making actual changes.

### Example workflow

1. Launch tidydots: `tidydots`
2. Navigate to the applications you want to restore
3. Press `tab` on each application to select it
4. Press `r` to start batch restore
5. Review the summary of symlinks to be created
6. Press `enter` to confirm
7. Watch the progress bar as symlinks are created

## Editing applications and entries

### Edit an application

Navigate to an application row and press `e` to open the edit screen. You can modify:

- **Name** -- the application identifier
- **Description** -- optional description text
- **When** -- conditional expression for machine filtering

### Editing package dependencies

When editing an application's packages section, you can manage dependencies for any standard package manager:

1. Navigate to the packages section of the application form
2. Move to a native package manager entry (e.g., `winget`, `apt`)
3. Press `p` to open the dependency editor for that manager
4. Use the list editor to add, edit, or delete dependencies:
   - `↑/k`, `↓/j` to navigate
   - `enter` or `e` to edit an item or add a new one
   - `d` or `delete` to remove a dependency
   - `esc` to exit the dependency editor
5. Dependencies are shown as a count indicator on the manager row: `winget: sxyazi.yazi (3 deps)`

### Edit a config entry

Navigate to a config entry and press `e` to edit. Editable fields include:

- **Name** -- entry identifier
- **Backup** -- path in your dotfiles repo
- **Targets** -- OS-specific target paths (linux, windows)
- **Files** -- specific file list (empty means entire folder)
- **Sudo** -- toggle for elevated privileges

### File picker

When editing the files list of a config entry, you have two ways to add files:

**Browse Files mode**

1. Navigate to the files field and press `enter`
2. Select "Browse Files" from the menu
3. An interactive file browser opens, starting in the target directory
4. Navigate with `↑/↓` or `k/j`, toggle file selection with `space` or `tab`
5. Selected files are highlighted with a purple background
6. Press `enter` to confirm your selections
7. Files are automatically converted to relative paths

**Type Path mode**

1. Navigate to the files field and press `enter`
2. Select "Type Path" from the menu
3. Type the relative file path directly
4. Press `enter` to confirm

!!! note
    The file picker only accepts files within the target directory hierarchy. Selected files are stored as relative paths.

### List field navigation

When editing a list field (like files), the field has its own internal cursor:

- `↑/k` and `↓/j` navigate within the list
- At the top or bottom of the list, navigation moves to adjacent form fields
- `enter` or `e` on a list item enters edit mode for that item
- `enter` or `e` on the "Add" button starts adding a new item
- `d`, `delete`, or `backspace` removes the selected item

## Saving changes

Press `s` or `ctrl+s` to save your changes to the `tidydots.yaml` configuration file. The TUI writes back to the same file it loaded from.

!!! warning
    Save writes to your `tidydots.yaml` immediately. If you want to preview changes first, use dry-run mode (`tidydots -n`) to confirm behavior before saving.

## Template diff & edit

When a config entry uses templates (`.tmpl` files) and you have manually edited the rendered output, the entry shows a **Modified** status in blue. You can view a diff of your changes and edit the source template to incorporate them.

### Viewing diffs

1. Navigate to a sub-entry row showing **Modified** status
2. Press `i` to launch the diff viewer
3. If the entry contains multiple modified template files, a picker appears -- select the file you want to inspect
4. Your editor opens with two panes:
    - **Left pane**: A unified diff showing your edits (read-only)
    - **Right pane**: The `.tmpl` source file (editable)
5. Edit the template to backport your changes, then save and quit your editor
6. The TUI resumes and refreshes the entry status

### Editor detection

tidydots automatically detects the best way to launch the editor:

| Mode | Condition | Behavior |
|------|-----------|----------|
| **Neovim** (default) | `nvim` is on `$PATH` | Opens both files in vertical splits with the diff pane read-only |
| **Tmux** | Running inside tmux with `$EDITOR` set | Opens template in a tmux split pane, diff in the current pane |
| **Fallback** | Neither nvim nor tmux available | Opens just the template in `$EDITOR` (or `vim`/`vi`/`nano`) |

!!! tip
    The diff compares the **pure render** (what the template produced) against the **current file on disk** (with your edits). This helps you see exactly what you changed so you can update the template source accordingly.

## Help text

Context-sensitive help is displayed at the bottom of each screen. The help text updates based on your current state:

- In navigation mode: shows navigation and action keys
- In edit mode: shows save and cancel keys
- In selection mode: shows available batch operations

## Practical examples

### Restore specific configs interactively

```bash
tidydots restore -i
```

1. Browse the list of applications and config entries
2. Select the ones you want to restore with `tab`
3. Press `r` to restore
4. Review the summary and confirm

### Install packages interactively

```bash
tidydots install -i
```

1. Browse applications that have packages configured
2. Select which applications to install packages for
3. Press `i` to install
4. Review the summary showing which packages and managers will be used
5. Confirm to proceed

### Add a new application via TUI

1. Launch `tidydots`
2. Press `A` to add a new application
3. Fill in the name, description, and `when` expression
4. Press `a` to add config entries with backup paths and targets
5. Press `s` to save the configuration

## Next steps

- [Multi-Machine Setups](multi-machine-setups.md) -- conditional configs with `when` expressions
- [Package Management](package-management.md) -- package installation details
- [System Configs](system-configs.md) -- managing files requiring sudo
