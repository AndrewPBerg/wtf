# WorkTreeForge (wtf) Roadmap

A fast, opinionated git worktree workflow tool with forge integrations, automated project setup, and abus-ready JSON output.

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
