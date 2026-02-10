---
hide:
  - navigation
  - toc
---

# tidydots

**A cross-platform dotfile management tool**

Keep your configuration files organized, versioned, and portable across machines.
tidydots manages symlinks, renders templates, installs packages, and clones git
repositories --- all from a single YAML file in your dotfiles repo.

---

<div class="grid cards" markdown>

-   :material-link-variant:{ .lg .middle } **Symlink-based config management**

    ---

    Configs are symlinked from your dotfiles repo. Edits sync instantly ---
    no copying, no drift.

-   :material-monitor:{ .lg .middle } **Cross-platform**

    ---

    First-class support for Linux and Windows with OS-specific target paths
    in every config entry.

-   :material-file-code:{ .lg .middle } **Template rendering**

    ---

    Go templates with [sprout](https://github.com/go-sprout/sprout) functions
    let you generate machine-specific configs from a single source file.

-   :material-package-variant:{ .lg .middle } **Multi-package-manager support**

    ---

    Install packages through pacman, yay, paru, apt, dnf, brew, winget,
    scoop, choco, or custom installers.

-   :material-console:{ .lg .middle } **Interactive TUI**

    ---

    A Bubble Tea terminal interface for browsing, selecting, and managing
    your applications visually.

-   :material-git:{ .lg .middle } **Git repository management**

    ---

    Clone and update git repositories as packages --- great for plugin
    managers, themes, and tools.

-   :material-download:{ .lg .middle } **Smart adopt workflow**

    ---

    Migrates existing configs into your dotfiles repo automatically, then
    replaces them with symlinks.

-   :material-eye:{ .lg .middle } **Dry-run mode**

    ---

    Preview every operation before it touches your filesystem with the
    `-n` flag.

</div>

---

## Install

```bash
go install github.com/AntoineGS/tidydots/cmd/tidydots@latest
```

## Get started

Ready to organize your dotfiles?

[Get started :material-arrow-right:](getting-started/quick-start.md){ .md-button .md-button--primary }
