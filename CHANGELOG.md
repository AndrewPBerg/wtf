# Changelog

All notable changes to WorkTreeForge (wtf) will be documented in this file.

## [Unreleased]

### Added

- **Automatic tab completions via `wtf init`** — `eval "$(wtf init)"` now includes both the shell wrapper and tab completions. No separate completion script needed.
- **Dynamic completions** for `wtf sw`, `wtf rm`, `wtf new`, `wtf clean`, and `wtf pr` (active worktrees, remote branches, merged/prunable worktrees, open PRs)
- **`wtf completion --install`** — Write completion file to standard user-local path as an alternative to inline completions

## [0.2.0] — 2026-03-21

### Added

- **`wtf setup`** — Run project setup in current worktree (`--env`, `--install`)
- **`wtf setup shell`** — Auto-configure shell integration (moved from `wtf setup`)
- **`wtf config init`** — Generate default `.wt-forge.toml` with auto-detection
- **`wtf news`** — Create a new worktree and switch to it with automatic setup
- **`.wt-forge.toml`** — Declarative project configuration:
  - `[worktree]` — root path, default base branch
  - `[env]` — env file handling (symlink, copy, or none)
  - `[[setup]]` — ordered setup steps with conditional execution
  - `[hooks]` — lifecycle hooks (on_create, on_switch, on_remove)
- Auto-detect package manager from lockfile (pnpm, bun, npm, yarn, uv, poetry, pip, go, cargo, bundle, composer, maven, gradle, dotnet, mix, swift)
- Setup conditions (if-DSL): `branch contains`, `file exists`, `env VAR is set`
- Shell integration for `wtf news` (cd's into the new worktree)

## [0.1.0] — 2025-12-01

### Added

- **`wtf ls`** — List all worktrees with status (`--json`, `--global`)
- **`wtf new`** — Create a new worktree (`--base`)
- **`wtf sw`** — Switch to a worktree with fuzzy match (`--global`)
- **`wtf rm`** — Remove a worktree and clean up (`--force`, `--global`)
- **`wtf clean`** — Remove merged or stale worktrees (`--dry-run`, `--force`)
- **`wtf version`** — Print version
- **`wtf init`** — Print shell functions for eval/source
- **`wtf completion`** — Generate shell completion scripts
- **`wtf update`** — Self-update to latest version
- **`wtf uninstall`** — Remove the wtf binary
- Global repo registry (`~/.wtf/repos.json`) for cross-repo operations
- Shortcut commands: `wtf lsg`, `wtf swg`, `wtf rmg`
- Integration tests against real temp git repos
- 90% test coverage gate
- golangci-lint + prek pre-commit hooks
