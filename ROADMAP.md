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
---

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

---

## v0.42.0 — abus-Ready & Polish

**Status:** planned

**Goals:**
- Every command has `--json` output for abus consumption
- Shell completions fully working across bash, zsh, fish
- Dogfooded on this repo, README quickstart verified

### Features

- [ ] `--json` flag on all commands (machine-readable, stable schema), alias `-j`
- [ ] CONTRIBUTING.md and docs/ complete
- [ ] add a pr watch funcationality for repos, could be global too for notifications you are subscribed to w/ native Notifivations from the process

### Commands

- [x] `wtf completion <shell>` — Generate shell completion script (bash, zsh, fish, powershell)
- [ ] `wtf doctor` — Verify environment health (git, gh/glab, tokens)

### Internals

- [x] Completion dynamic hooks for branch and PR names
- [ ] JSON output layer across all commands

---

## Principles

- All business logic in `internal/` — never in `cmd/`
- No global state — pass dependencies explicitly
- Table-driven tests as default pattern
- Errors wrapped with context: `fmt.Errorf("doing X: %w", err)`
- 90% test coverage enforced in CI at every milestone
- Changelog updated with every PR
- `docs/` has one markdown per command, updated as commands ship
