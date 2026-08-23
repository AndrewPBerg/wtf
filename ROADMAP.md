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

- [x] Coverage gate established; current adjusted floor is 75% with behavioral tests prioritized over percentage chasing
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

## v0.3.0–v0.5.1 — Platform Integration (GitHub & GitLab)

**Status:** released historically across v0.3.0–v0.5.1

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

- [x] PR checkout, now exposed through `wtf new <number>` and `wtf new --pr`
- [x] `wtf ls --prs` — List worktrees with PR status inline (lazy-loaded)
- [x] `wtf news` create-and-switch behavior

### Completions

- [x] `wtf pr <TAB>` → open PRs with number, branch, author, age
- [x] `wtf sw <TAB>` → active worktrees
- [x] `wtf rm <TAB>` → active worktrees
- [x] `wtf new <TAB>` → remote branches not yet checked out
- [x] `wtf clean <TAB>` → merged/stale worktrees
- [x] `wtf init` embeds completions inline — `eval "$(wtf init)"` handles everything
- [x] `wtf completion --install` writes to standard user-local path

Historical delivery expectations:

- changelog entries for releases; and
- one maintained Markdown page per current command.

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
- [x] Restore the full quality gate under the configured Go and JJ versions
- [x] Fix JJ 0.44 prunable-workspace compatibility
- [x] Introduce canonical repository/workspace UUIDs that are never reused (completed in v0.8)
- [x] Enforce globally unique names for all active WTF-managed workspaces (completed in v0.8)

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

## v0.9.0 — Structured JJ Workspace Substrate

**Status:** released 2026-08-22

**Goal:** make WTF an independently usable physical-isolation actuator. Agent
Bridge decides WorkUnits, peer ownership, semantic JJ change shaping, integration,
verification, and orchestration; JJ owns graph semantics. WTF exposes stable,
structured inspection and safe workspace removal without becoming an integration
engine.

- [x] Provide stable structured workspace inspection/listing through `--json`
- [x] Distinguish persistent WTF repository/workspace UUIDs from current physical
      paths, JJ workspace/change/bookmark identities, and Git-shadow state
- [x] Document the ID-domain mapping: WTF's persistent random workspace UUIDs are
      not Agent Bridge's deterministic scope UUIDs and must never be conflated
- [x] Provide deterministic destructive-removal planning and apply
- [x] Fail closed when removal preconditions or planned physical/JJ identities have
      changed
- [ ] Report incomplete cleanup as visible, repairable debt (deferred to v0.10)
- [x] Keep JJ graph operations, semantic change shaping, publication, verification,
      and orchestration outside WTF
- [ ] Keep the Git shadow explicitly presentation-only and report stale or missing
      shadow state in structured results (deferred to v0.10)

`integrate --source/--target` graph gathering is deferred. WTF does not promise to
own rebase, multi-parent integration, bookmark creation, push, or publication.

## v0.10.0 — Versioned Automation Contract

**Status:** released 2026-08-22

**Goal:** make WTF a boring, versioned CLI/JSON substrate that Agent Bridge and thin
harness adapters can consume without introducing a reverse dependency on those systems.

- [x] Return versioned JSON envelopes (`version: 1`) with canonical repository/workspace
      IDs from every structured workspace result
- [x] Accept canonical IDs for automation while retaining unambiguous names and paths for
      human convenience
- [x] Provide idempotent non-interactive create, inspect, plan, apply, and cleanup APIs
- [x] Report incomplete cleanup as visible, repairable `cleanup_failed` debt with a
      UUID-based retry path
- [x] Report Git-shadow health (`not_supported`, `absent`, `present`, `stale`, or
      `unavailable` with an error) as observation-only structured data
- [x] Keep the Pi extension thin: it directs checkout creation to WTF and does not
      reproduce workspace lifecycle logic
- [x] Document the one-way dependency: Agent Bridge calls WTF; WTF never calls or stores
      Agent Bridge data
- [x] Keep WorkUnits, actors, collisions, checkpoints, Linear, Herdr, Zed observation,
      Watchman, verification policy, and orchestration outside WTF

**Non-goals:** WorkUnit or Agent Bridge RPC, JJ graph operations, bookmark creation,
integration, publication, verification policy, resource hooks, `.wtf.toml`, or a
general in-process Go API. The stable surface is the CLI JSON contract.

## v0.11.0 — Resource Substrate and Declarative Configuration

**Status:** released 2026-08-22; second-project confidence work remains

**Goal:** make workspace file and port intent inspectable, UUID-owned, and
repairable without adding database, Docker/service, secret, or orchestration scope.

- [x] Strict optional v1 `.wtf.toml` manifest and metadata-only secret handling
- [x] Versioned `workspace current`, `capabilities`, `resources`, and `doctor`
      JSON surfaces
- [x] UUID-owned file/port registry, leases, observed state, and cleanup debt
- [x] Fail-closed managed-file creation/removal and UUID retry behavior
- [x] Dogfood non-secret Markdown and nested secret-marked env symlinks, a named
      port lease, drift diagnosis, cleanup debt, and repair on Pluckmd
- [ ] Post-release: dogfood a second VCS-backed project (Labctl currently has no VCS metadata)
- [ ] Expand deterministic glob reconciliation only after its target-mapping
      semantics are specified; v1 currently rejects globs before state changes

**Deferred to v0.12:** database lifecycle/stress testing, Docker/services
integration, package-manager installation behavior, and any task-runner or generic
lifecycle-hook abstraction.

## v0.11.1 — Release Corrections and Quality Restoration

**Status:** released 2026-08-23

- [x] Correct the stale embedded/build version left in the v0.11.0 tag
- [x] Reconcile roadmap, changelog, command documentation, and lifecycle-script scope
- [x] Fail closed on uninspectable managed resource targets
- [x] Restore and enforce the adjusted 90% coverage gate in CI
- [x] Expand identity, resource, workspace, cleanup, setup, and JJ Git-diff contract tests

## v0.11.2 — CI Parity Correction

**Status:** ready for release

- [x] Install JJ 0.44 in CI before measuring the adjusted coverage gate
- [x] Keep local and CI quality inputs equivalent
- [x] Set a pragmatic 75% adjusted floor and retain the expanded lifecycle contract tests

## Dogfood gate

For each milestone, record real workspace creation steps, manual repairs, Zed
friction, resource conflicts, keep/remove outcomes, and cleanup debt. Do not add a
new orchestration layer merely because it is possible; require repeated friction or
a concrete safety failure.
