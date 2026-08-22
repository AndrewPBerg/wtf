# WorkTreeForge (wtf) Roadmap

A small, opinionated workspace-lifecycle tool. WTF is JJ-first for local work,
Git-compatible at forge boundaries, Zed-first for editor integration, and usable by
both humans and Pi agents through the same CLI contract.

Current design documents:

- [JJ-native workspace lifecycle](docs/jj-workspace-lifecycle-plan.md)
- [Simplification plan](docs/simplification-plan.md)

The completed milestones below are retained as project history. New work follows the
current milestones at the end of this document and is promoted only after real
9–5 and personal-project dogfooding.

## v0.1.0 — Core Worktree Operations

**Status:** complete

**Goals:**
- Solid `internal/git/` foundation everything else builds on
- Read and write worktree operations
- Clean, tested CLI surface for day-to-day use

### Commands

- [x] `wtf ls` — List all worktrees with status (`--json`, `--global`)
- [x] `wtf new <branch>` — Create a new worktree at a managed location (`--base <branch>`)
- [x] `wtf sw <branch>` — Switch to a worktree (fuzzy match)
- [x] `wtf rm <branch>` — Remove a worktree and clean up (`--force`)
- [x] `wtf clean` — Remove merged or stale worktrees (`--dry-run`, `--force`)
- [x] `wtf version` — Print version

### Internals

- [x] `internal/git/` — ListWorktrees, AddWorktree, RemoveWorktree
- [x] `internal/cli/` — cobra wiring

### Quality

- [x] 90% test coverage gate (cli package at ~78%)
- [x] Integration tests against real temp git repos
- [x] golangci-lint passing
- [x] `task all` green

---

## v0.2.0 — Setup Automation

**Status:** complete

**Goals:**
- Zero-friction worktree setup — no manual `pnpm install` or `.env` copying
- Zero config required — sensible defaults, CLI flags for overrides
- Auto-detect package manager and env files

### Features

- [x] Auto-detect package manager from lockfile:
  - `pnpm-lock.yaml` → `pnpm install`
  - `bun.lockb` → `bun install`
  - `package-lock.json` → `npm install`
  - `yarn.lock` → `yarn install`
  - `uv.lock` → `uv sync`
  - `pyproject.toml` → `uv sync`
  - Plus: go, cargo, bundler, composer, maven, gradle, dotnet, mix, swift
- [x] Env file symlinking from main worktree (default):
  - `.env`, `.env.local`, `.env.development`, `.env.development.local`
- [x] CLI flags for setup control: `--no-setup`, `--no-env`, `--no-install`

### Commands

- [x] `wtf setup` — Re-run setup in current worktree (`--env`, `--install`)
- [x] `wtf setup shell` — Shell integration (moved from `wtf setup`)
## v0.3.0 — Platform Integration (GitHub & GitLab)

**Status:** planned

**Goals:**
- Checkout PRs directly as worktrees
- See open PR status inline in `wtf ls`
- Fast completions backed by cached PR list and last checked warning, and worker subthreads to easily fetch non-blocking async

### Features

- [x] Forge auto-detection from origin remote URL:
  - `github.com` → GitHub API via `gh` token
  - `gitlab.com` → GitLab API via `glab` token
- [x] Auth: reuse existing `gh` / `glab` tokens, no custom credential management
- [x] Lazy-loading PR cache (`.git/wtf/pr-cache.json`, instant render + background revalidation)
- [x] `wtf ls --prs` shows PR number, title, author, review status with lazy loading

### Commands

- [x] `wtf pr <number|branch>` — Checkout a PR as a worktree
- [x] `wtf ls --prs` — List worktrees with PR status inline (lazy-loaded)
- [x] news isn't working as expected

### Completions

- [x] `wtf pr <TAB>` → open PRs with number, branch, author, age
- [x] `wtf sw <TAB>` → active worktrees
- [x] `wtf rm <TAB>` → active worktrees
- [x] `wtf new <TAB>` → remote branches not yet checked out
- [x] `wtf clean <TAB>` → merged/stale worktrees
- [x] `wtf init` embeds completions inline — `eval "$(wtf init)"` handles everything
- [x] `wtf completion --install` writes to standard user-local path

--
- Changelog updated with every PR
- `docs/` has one markdown per command, updated as commands ship

## v0.6.0 — Jujutsu (jj) Support

### Backends

- [x] `internal/vcs` — backend-agnostic `Worktree` model, `Manager` interface, and repo detection
- [x] `internal/jj` — jj workspace backend driving `jj workspace add/list/forget`
- [x] Repo discovery through the backend, so wtf works from inside a jj workspace (which has no `.git`)
- [x] Per-repo state under `.jj/repo/wtf/` mirroring `.git/wtf/`

### Dispatch

- [x] `.git` only → git, `.jj` only → jj, no configuration needed
- [x] Colocated repos prompt once and persist the choice in `~/.wtf/repos.json`
- [x] `--vcs git|jj` and `WTF_VCS` for scripting
- [x] Non-TTY colocated fallback to git, preserving pre-jj script behavior

### Telling them apart

- [x] `WORKSPACE` / `BOOKMARK` / `CHANGE` columns for jj instead of `BRANCH` / `HEAD`
- [x] Backend badge on every row of a global listing
- [x] Colocated repos surface checkouts held by the other backend instead of hiding them
- [x] `sw` misses under one backend report a match under the other

### Not doing

- [ ] ~~Creating jj bookmarks on workspace creation~~ — bookmarks are a push-time
      concern the user owns; the workspace name is the identity wtf keys on

---

## v0.7.0 — Smaller Core and Zed Baseline

**Status:** released 2026-08-22; unchecked simplification work remains active

**Goal:** establish the smallest reliable product boundary before adding new
workspace-lifecycle operations.

### Repository and quality

- [x] Make the repository colocated JJ/Git and track `main@origin`
- [x] Move the canonical Pi extension source and tests into WTF
- [x] Upgrade the active Go dependency graph
- [ ] Restore the full quality gate under the configured Go and JJ versions
- [x] Fix JJ 0.44 prunable-workspace compatibility
- [ ] Introduce canonical repository/workspace UUIDs that are never reused
- [ ] Enforce globally unique names for all active WTF-managed workspaces

### Simplification

- [ ] Decide between a numbered prompt and argument-only command selection
- [ ] Replace the Bubble Tea/Lip Gloss picker surface
- [ ] Remove Charm dependencies and compare module/binary size
- [ ] Dogfood switching, removal, multi-selection, and cancellation
- [ ] Audit watch, notifications, global registry flows, self-update, and automatic
      dev-server startup one feature at a time

### Zed

- [x] Finish and land the experimental JJ Git-diff shadow, enabled by default for
      secondary JJ workspaces
- [x] Keep explicit CLI/environment opt-outs narrow; defer `.wtf.toml` control for
      disabling the shadow until the manifest milestone
- [x] Dogfood the shadow as a read-only Zed source-control projection
- [ ] Detect or clearly explain stale editor baselines
- [ ] Add an explicit `wtf open` only if opening workspaces remains repeated friction

## v0.8.0 — Canonical Workspace Identity

**Status:** released 2026-08-22; declarative resource follow-ups remain planned

**Goal:** provide strict, durable repository/workspace identity that humans and
Agent Bridge can use without coupling WTF to WorkUnits or orchestration.

- [x] Add canonical repository/workspace UUIDs that are never reused
- [x] Enforce globally unique active names and canonical JJ native names
- [x] Persist locked, atomic, corruption-checked identity state
- [x] Add repository markers and safe legacy adoption boundaries
- [x] Add identity-aware creation, listing, switching, removal, and JSON
- [x] Add exact workspace UUID selectors
- [x] Make the complete Go race suite clean

### Declarative isolation and resource follow-ups

- [ ] Specify the minimum versioned `.wtf.toml` manifest
- [ ] Add project-level configuration for disabling the otherwise default-on Zed
      Git-diff shadow
- [ ] Preserve useful zero-config behavior
- [ ] Add default-workspace policy: `allow`, `warn`, or agent-enforced denial
- [ ] Separate source env-file policy from generated workspace values
- [ ] Support multiple stable named ports per workspace ID
- [ ] Store resources against canonical workspace IDs rather than mutable names
- [ ] Harden shared per-repository state with locking and atomic writes
- [ ] Add project-defined create/drop hooks for isolated databases
- [ ] Keep failed cleanup as visible, repairable debt
- [ ] Reconsider SQLite only after JSON concurrency or recovery fails in practice

## v0.9.0 — Deterministic JJ Graph Substrate

**Status:** planned

**Goal:** expose safe physical workspace and JJ graph primitives without owning
WorkUnits, participants, integration policy, verification order, or orchestration.
Agent Bridge remains the integration specialist and supplies the exact canonical
workspace IDs and requested operation.

- [ ] Resolve canonical workspace IDs to current JJ workspace/change identities
- [ ] Expose read-only graph planning with complete source/target preconditions
- [ ] Apply a previously returned plan only when those identities remain unchanged
- [ ] Support rebase, multi-parent integration, and publication as explicit physical
      operations rather than semantic WorkUnit decisions
- [ ] Keep fetch, graph mutation, bookmark creation, push, verification, and cleanup
      as separate boundaries
- [ ] Return conflicts, resulting change IDs, recovery evidence, and bounded output
      through stable `--json`
- [ ] Resolve the Git-shadow limitation for multi-parent working copies explicitly

Every mutating operation starts with a deterministic plan and fails if graph
assumptions change before apply.

## v0.10.0 — Stable Integration API

**Status:** planned

**Goal:** make WTF a boring substrate that Agent Bridge and thin harness adapters can
consume without introducing a reverse dependency on those systems.

- [ ] Return canonical repository/workspace IDs from every structured workspace result
- [ ] Accept IDs for automation while retaining unambiguous names for human convenience
- [ ] Provide idempotent non-interactive create, inspect, plan, apply, and cleanup APIs
- [ ] Keep the Pi extension thin and installable from this repository
- [ ] Document the one-way dependency: Agent Bridge calls WTF; WTF never calls Agent Bridge
- [ ] Keep WorkUnits, actors, collisions, checkpoints, Linear, Herdr, Zed observation,
      Watchman, verification policy, and orchestration outside WTF

## Dogfood gate

For each milestone, record real workspace creation steps, manual repairs, Zed
friction, resource conflicts, gather/discard outcomes, and cleanup debt. Do not add a
new orchestration layer merely because it is possible; require repeated friction or
a concrete safety failure.
