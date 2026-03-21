# WorkTreeForge (wtf) Roadmap

A fast, opinionated git worktree workflow tool with forge integrations, automated project setup, and abus-ready JSON output.

## v0.1.0 — Core Worktree Operations

**Status:** in-progress

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

- [ ] 90% test coverage gate (cli package at ~78%)
- [x] Integration tests against real temp git repos
- [x] golangci-lint passing
- [x] `task all` green

---

## v0.2.0 — Setup Automation

**Status:** planned

**Goals:**
- Zero-friction worktree setup — no manual `pnpm install` or `.env` copying
- Declarative `.wt-forge.toml` per project
- Auto-detect package manager and env files

### Features

- [ ] Auto-detect package manager from lockfile:
  - `pnpm-lock.yaml` → `pnpm install`
  - `bun.lockb` → `bun install`
  - `package-lock.json` → `npm install`
  - `yarn.lock` → `yarn install`
  - `uv.lock` → `uv sync`
  - `pyproject.toml` → `uv sync`
- [ ] Env file handling (symlink | copy | none):
  - `.env`, `.env.local`, `.env.development`, `.env.development.local`
- [ ] `.wt-forge.toml` support:
  - `[worktree]` — root path, default base branch
  - `[env]` — strategy and file list
  - `[setup]` — ordered steps with optional if-conditions
  - `[hooks]` — on_create, on_switch, on_remove
- [ ] Setup conditions (if-DSL):
  - `branch contains 'feature'`
  - `file exists 'path'`
  - `env VAR is set`

### Commands

- [ ] `wtf setup` — Re-run setup steps on current worktree (`--env`, `--install`)
---

## v0.3.0 — Forge Integration (GitHub & GitLab)

**Status:** planned

**Goals:**
- Checkout PRs directly as worktrees
- See open PR status inline in `wtf ls`
- Fast completions backed by cached PR list and last checked warning, and worker subthreads to easily fetch non-blocking async

### Features

- [ ] Forge auto-detection from origin remote URL:
  - `github.com` → GitHub API via `gh` token
  - `gitlab.com` → GitLab API via `glab` token
- [ ] Auth: reuse existing `gh` / `glab` tokens, no custom credential management
- [ ] PR list cache (TTL 5m, `.git/wtf/pr-cache.json`, stale-while-revalidate)
- [ ] `wtf ls --prs` shows PR number, title, author, age, review status

### Commands

- [ ] `wtf pr <number|branch>` — Checkout a PR as a worktree
- [ ] `wtf ls --prs` — List worktrees with PR status inline

### Completions

- [ ] `wtf pr <TAB>` → open PRs with number, branch, author, age
- [ ] `wtf sw <TAB>` → active worktrees
- [ ] `wtf rm <TAB>` → active worktrees
- [ ] `wtf new <TAB>` → remote branches not yet checked out
- [ ] `wtf clean <TAB>` → merged/stale worktrees

---

## v0.4.0 — abus-Ready & Polish

**Status:** planned

**Goals:**
- Every command has `--json` output for abus consumption
- Shell completions fully working across bash, zsh, fish
- Dogfooded on this repo, README quickstart verified

### Features

- [ ] `--json` flag on all commands (machine-readable, stable schema)
- [ ] Background branch/PR cache refresh
- [ ] CONTRIBUTING.md and docs/ complete
- [ ] GitHub Actions CI with coverage gate and lint

### Commands

- [ ] `wtf completions <shell>` — Generate shell completion script (bash, zsh, fish)
- [ ] `wtf doctor` — Verify environment health (git, gh/glab, tokens)

### Internals

- [ ] Completion dynamic hooks for branch and PR names
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
