# Changelog

All notable changes to WorkTreeForge (wtf) will be documented in this file.

## [Unreleased]

## [0.11.2] — 2026-08-23

### Fixed

- Installed the configured JJ version in CI so JJ integration coverage is
  measured consistently with the local quality gate.
- Set a pragmatic 75% adjusted coverage floor while retaining the expanded
  behavioral contract tests.

## [0.11.1] — 2026-08-23

### Fixed

- Corrected source/build version metadata that remained at `0.10.0` in the
  `v0.11.0` tag and reconciled the release and command documentation.
- Failed closed when a managed resource target cannot be inspected because an
  ancestor is not a directory.

### Changed

- Expanded identity, resource, cleanup, workspace-contract, and JJ Git-diff
  coverage and made the adjusted coverage floor an enforced CI gate.

## [0.11.0] — 2026-08-22

### Added

- **Resource substrate** — strict optional `.wtf.toml` intent, UUID-owned file/port
  resource state, read-only `workspace current`, `capabilities`, `resources`, and
  `doctor` JSON contracts, plus fail-closed managed-file cleanup.

### Notes

- Pluckmd dogfood covered managed Markdown, nested secret-marked env links, named
  port leases, drift diagnosis, cleanup debt, and repair. A second VCS-backed
  project remains post-release confidence work rather than an unshipped feature.

## [0.10.0] — 2026-08-22

### Added

- **Versioned automation contract** — workspace inspection/listing and cleanup
  results identify JSON schema version 1, include canonical WTF identities, and
  report Git-diff shadow health (`absent`, `present`, `stale`, or `unavailable`).
- **Idempotent automation operations** — `wtf new --ensure`, cleanup planning and
  apply, and UUID cleanup retries provide explicit repeat-safe semantics.
- **Repairable cleanup debt** — partial lifecycle finalization is retained as
  `cleanup_failed` and can be finalized through an exact UUID retry.

### Changed

- The Pi extension remains a thin WTF policy adapter with no Agent Bridge dependency.

## [0.9.0] — 2026-08-22

### Added

- **Read-only workspace inspection** — `wtf workspace inspect` and `wtf workspace
  list` expose durable workspace identity separately from current physical/JJ state,
  including JJ operation identity and Git-diff shadow status where available.
- **Workspace cleanup plans** — `wtf cleanup plan` emits an identity-bound JSON
  artifact; `wtf cleanup apply` requires that unchanged artifact before performing
  the existing server, VCS, port, and identity cleanup lifecycle. Existing `wtf rm`
  behavior is unchanged.

### Changed

- Refined the v0.9 boundary: WTF provides stable structured workspace/JJ/shadow
  inspection and deterministic fail-closed removal planning/apply. Agent Bridge
  owns WorkUnits, scope ownership, semantic change shaping, integration,
  verification, and orchestration; WTF does not own
  `integrate --source/--target`, rebase, multi-parent integration, or publication.
- Documented that persistent random WTF workspace UUIDs and deterministic Agent
  Bridge scope UUIDs are separate ID domains and require an explicit mapping.

## [0.8.0] — 2026-08-22

Adds canonical, durable repository and workspace identity as WTF's stable substrate
for humans, Agent Bridge, and other automation.

### Added

- **Canonical repository/workspace UUIDs** — Persistent lowercase UUIDs survive
  workspace moves and renames, are never reused, and remain as tombstones after
  removal.
- **Globally unique active workspace names** — Canonical names use
  `<repository>/<workspace>`. JJ uses the canonical name natively; Git branches
  remain independent forge refs.
- **Locked atomic identity store** — `~/.wtf/state.json` uses cross-process locking,
  strict schema validation, atomic durable replacement, corruption fail-closed
  behavior, and Unix/Windows implementations.
- **Repository identity markers** — Backend-shared `repository-id` markers reconcile
  local repository identity with global state and support safe incremental adoption.
- **Identity-aware JSON and selectors** — Creation, listing, switching, and removal
  expose `repository_id`, `workspace_id`, `name`, and `native_name`; `sw` and `rm`
  accept exact workspace UUIDs.
- **Identity lifecycle** — Workspace creation reserves `pending` identity before VCS
  mutation, activates after required setup, and records removal/cleanup history with
  deterministic repair boundaries.

### Changed

- `new`/`news` now create canonical JJ workspace names while preserving Git branch
  semantics and existing human-facing output fields.
- Existing legacy workspaces remain listable and are never silently renamed.
- Removal coordinates physical cleanup, identity tombstones, server shutdown, and
  port release; UUID retry can finalize state after an interrupted tombstone write.

### Fixed

- CLI tests now unconditionally isolate `WTF_HOME`, preventing test identity data
  from escaping into user state.
- Synchronized the forge cache test mock so the complete race suite is clean.

### Notes

- Agent Bridge owns WorkUnits and orchestration; WTF stores only deterministic local
  repository/workspace identity and lifecycle.
- `.wtf.toml`, richer named resources, database hooks, gather orchestration, and
  Charm UI removal remain deferred.

## [0.7.0] — 2026-08-22

Makes JJ workspaces first-class in Zed while keeping JJ authoritative, and lays the
repository boundary for future workspace/resource primitives consumed by Agent Bridge.

### Added

- **Default-on Git shadows for secondary JJ workspaces** — `wtf new` and `wtf news`
  create private Git metadata so Git-first editors such as Zed can display the JJ
  working-copy diff. The shadow borrows JJ's object database, has no publishing
  remote, and is excluded from WTF repository detection and registration.
- **`wtf git-diff`** — Create or refresh a workspace's editor baseline after a JJ
  operation changes its working-copy parent.
- **Explicit shadow opt-outs** — `--no-git-diff` and `WTF_JJ_GIT_DIFF=0`; project
  manifest configuration remains intentionally deferred.
- **Canonical Pi extension source** — WTF's thin Pi policy adapter, tests, and local
  installer now live under `packages/pi-extension/`.
- **JJ-native lifecycle and simplification plans** — Documents the Zed-first,
  Agent-Bridge-consumed substrate direction and evidence-driven removal of UI bloat.

### Changed

- Updated the active Go dependency graph.
- The WTF repository itself is now dogfooded as a colocated JJ/Git repository.

### Fixed

- **JJ 0.44 prunable workspace detection** — Deleted workspace directories may be
  rendered with an empty `self.root()` rather than an error string; both forms are
  now treated conservatively as prunable registrations.

### Notes

- Git shadows are presentation-only. Continue to use JJ for all VCS mutations.
- Multi-parent JJ working copies do not yet have a single Git-shadow baseline.
- `.wtf.toml`, canonical workspace IDs, strict active-name uniqueness, and richer
  resource configuration remain future substrate work.

## [0.6.0] — 2026-08-03

Adds Jujutsu (jj) support: the same commands drive git worktrees and jj workspaces,
including automatic env-file linking and package install.

### Added

- **Jujutsu (jj) support** — `new`, `news`, `sw`, `swg`, `rm`, `rmg`, `clean`, `port`, and `setup` all drive jj workspaces as well as git worktrees. jj workspaces honor `.gitignore`, so `.env` and `node_modules/` are not carried across — wtf links and installs them the same way it does for git worktrees. See [docs/jj.md](docs/jj.md).
- **`--vcs git|jj` and `WTF_VCS`** — Force a backend in a repo that is both git and jj. Colocated repos (jj's default layout) prompt once and remember the answer per-repo. Non-interactive shells never prompt — the backend is inferred from the checkouts the repo already has, and otherwise from whether jj owns the working copy (jj leaves git's HEAD detached), falling back to git only when neither is conclusive.
- **jj-aware listing** — `wtf sw` in a jj repo shows `WORKSPACE`, `BOOKMARK`, and `CHANGE` columns instead of `BRANCH`/`HEAD`, naming what jj actually tracks. Global listings tag every row with its backend, and a colocated repo with no saved preference contributes both sets.
- **Cross-backend hints** — In a colocated repo, wtf points out checkouts held by the backend you are not currently using, and a `sw` miss under one backend reports a match under the other.
- **`internal/vcs`** — Backend-agnostic worktree model and `Manager` interface; `internal/git` and the new `internal/jj` both implement it, and `internal/cli` talks only to the interface.

### Changed

- **Repo discovery is backend-aware** — Previously wtf located repos with `git rev-parse --show-toplevel`, which fails outright inside a jj workspace because those contain no `.git`. Discovery now goes through the detected backend, so wtf works from inside a jj workspace.
- **Repo registry format** — `~/.wtf/repos.json` is now an object carrying an optional per-repo backend preference. The previous array-of-paths format is still read and upgraded in place on the next write. **This is one-way:** wtf 0.5.1 and earlier cannot read the new file and will fail on global commands until upgraded.
- **jj state location** — Per-repo state (ports, forge cache, watch state) lives under `.jj/repo/wtf/` for jj repos, mirroring `.git/wtf/` for git repos.
- **Error wording** — "not a git repository" is now "not a git or jj repository", and the main-checkout removal hint no longer names git specifically.

### Fixed

- **git invoked with inherited repo-location environment** — `wtf` run where `GIT_DIR`, `GIT_INDEX_FILE`, or `GIT_WORK_TREE` are already set — which git does for every hook it runs — pointed git at the wrong repository, failing with errors like `fatal: .git/index: index file open failed`. wtf always names the repo it means, so those variables are now stripped from git invocations; credential helpers, `GIT_SSH_COMMAND`, and proxy settings are preserved. Pre-existing, not specific to jj.
- **Table column alignment with multi-byte characters** — Column widths were measured in bytes rather than runes, so any non-ASCII value (a unicode branch name, or the new "no bookmark" dash) shifted every column after it.

### Notes

- Requires the `jj` binary on `PATH` only for repos that need it. Tested against jj 0.43.
- Git-only repos behave exactly as before: same wording, same columns, no new prompts.
- Known limits: PR matching under jj goes through bookmarks, so a bookmark-less workspace will not match an open PR in `wtf sw --prs`; `wtf clean` under jj will not remove a workspace whose change is non-empty even if it has landed.

## [0.3.0] – [0.5.1] — released

> These entries shipped across the v0.3.0 through v0.5.1 tags but were never
> stamped with a version at the time, so per-release attribution was not recorded.


### Added

- **Jujutsu (jj) support** — `new`, `news`, `sw`, `swg`, `rm`, `rmg`, `clean`, `port`, and `setup` all drive jj workspaces as well as git worktrees, including automatic env-file linking and package install. jj workspaces honor `.gitignore`, so `.env` and `node_modules/` are not carried across — wtf links and installs them the same way it does for git worktrees. See [docs/jj.md](docs/jj.md).
- **`--vcs git|jj` and `WTF_VCS`** — Force a backend in a repo that is both git and jj. Colocated repos (jj's default layout) prompt once and remember the answer per-repo. Non-interactive shells never prompt — the backend is inferred from the checkouts the repo already has, and otherwise from whether jj owns the working copy (jj leaves git's HEAD detached), falling back to git only when neither is conclusive. Existing scripts are unaffected.
- **jj-aware listing** — `wtf sw` in a jj repo shows `WORKSPACE`, `BOOKMARK`, and `CHANGE` columns instead of `BRANCH`/`HEAD`, naming what jj actually tracks. Global listings tag every row with its backend and let a colocated repo contribute both sets.
- **Cross-backend hints** — In a colocated repo, wtf points out checkouts held by the backend you are not currently using instead of hiding them, and a `sw` miss under one backend reports a match under the other.

### Changed

- **Zero-config setup** — Removed `.wt-forge.toml` config file. Setup now works automatically with sensible defaults: symlink env files + auto-detect package manager. No config file needed.
- **CLI flags replace config** — `--no-setup`, `--no-env`, `--no-install` flags on `wtf new` and `wtf news` control post-create behavior.
- **`wtf setup`** — No longer reads a config file. Symlinks env files from main worktree and runs detected package install by default.
- **`wtf watch`** — Interval and notification settings are now CLI-flag-only (`-i`, `--no-desktop`).
- **`wtf sw` / `wtf swg` absorb `ls` / `lsg`** — Running `wtf sw` with no arguments launches an interactive worktree picker (j/k or arrows to navigate, enter to switch). `wtf swg` does the same across all registered repos. Non-TTY output falls back to the static table. `--prs` and `--json` flags are supported.
- **`wtf rm` / `wtf rmg` interactive picker** — Running with no arguments launches a multi-select picker (space to toggle, enter to confirm removal).
- **Repo registry format** — `~/.wtf/repos.json` is now an object carrying an optional per-repo backend preference. The previous array-of-paths format is still read, and upgraded in place on the next write.
- **Repo discovery is backend-aware** — Previously wtf located repos with `git rev-parse --show-toplevel`, which fails outright inside a jj workspace because those contain no `.git`. Discovery now goes through the detected backend, so wtf works from inside a jj workspace.
- **Error wording** — "not a git repository" is now "not a git or jj repository", and the main-checkout removal hint no longer names git specifically.
- **jj state location** — Per-repo state (ports, forge cache, watch state) lives under `.jj/repo/wtf/` for jj repos, mirroring `.git/wtf/` for git repos.

### Fixed

- **`wtf new --branch` and `--pr` under jj** — Both shelled out to `git fetch` unconditionally, which fails outright in a `--no-colocate` jj repo ("fatal: not a git repository") because the backing git repo lives inside `.jj`. Fetching is now a backend operation: the jj backend fetches into the backing repo and runs `jj git import`. `jj git fetch` is not used because it cannot express PR refspecs like `pull/42/head:pr-42`.
- **Fresh `jj git clone` resolved to git in scripts** — A newly cloned colocated repo has no checkouts to infer from, so non-interactive runs created a git worktree instead of a jj workspace. Detached-HEAD detection now identifies jj-owned working copies.
- **PR cache directory under jj** — `wtf new --pr` cached forge data via git's `--git-common-dir`, bypassing the backend's state directory.
- **Backend labels in the local picker** — Every row was tagged with its backend even in single-backend repos, so plain git users saw `(git)` on every line. Labels now appear only in colocated repos and global listings, where they disambiguate.
- **Table column alignment with multi-byte characters** — Column widths were measured in bytes rather than runes, so any non-ASCII value (a unicode branch name, or the new "no bookmark" dash) shifted every column after it.

### Removed

- **`.wt-forge.toml`** — Declarative config file and all related machinery (setup steps, conditions, hooks, watch config).
- **`wtf config init`** — No config file to generate.
- **Lifecycle hooks** — `on_create`, `on_switch`, `on_remove`, `on_pr_create`, `on_pr_switch`, `on_pr_delete` hooks removed.
- **`go-toml/v2` dependency** — No longer needed.
- **`wtf ls` / `wtf lsg`** — Replaced by `wtf sw` / `wtf swg` with no arguments.

### Added

- **Agent-safe env copying** — `wtf new`, `wtf news`, and `wtf setup` now support `--copy-env` to copy discovered `.env*` files (including nested files like `app/.env`) instead of symlinking them.
- **Automatic tab completions via `wtf init`** — `eval "$(wtf init)"` now includes both the shell wrapper and tab completions. No separate completion script needed.
- **Dynamic completions** for `wtf sw`, `wtf rm`, `wtf new`, and `wtf clean` (active worktrees, remote branches, merged/prunable worktrees, open PRs)
- **`wtf completion --install`** — Write completion file to standard user-local path as an alternative to inline completions
- **Interactive TUI pickers** — Built on bubbletea + lipgloss for keyboard-navigable worktree selection

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
