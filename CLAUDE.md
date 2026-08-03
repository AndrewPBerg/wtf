# WorkTreeForge (WTF)

Go CLI tool. Module: `github.com/AndrewPBerg/wtf`

## Build & Dev Commands

Uses [Task](https://taskfile.dev) (not Make). Install: `brew install go-task` or `go install github.com/go-task/task/v3/cmd/task@latest`

```bash
task build          # build the binary to bin/wtf
task test           # run all tests
task test-coverage  # run tests with coverage report (fail below 90%)
task lint           # run golangci-lint
task fmt            # gofmt + goimports
task tidy           # go mod tidy
task all            # fmt -> lint -> test -> build
task clean          # remove build artifacts
```

## Project Principles

1. **Linting/formatting is non-negotiable** — `golangci-lint` is the standard. `gofmt` + `goimports` baked into Taskfile. [prek](https://github.com/j178/prek) pre-commit hooks prevent committing unformatted code. Run `prek install` to set up git hooks locally.

2. **90% test coverage from day one** — unit test every `internal/` package in isolation. Integration tests shell out to git in temp repos. Use `testify` for assertions. CI fails below threshold.

3. **Modular design** — `internal/` packages are independently testable (`git`, `config`, `setup`, `cache`, `forge`). `cmd/` just wires them together, almost no logic lives there.

4. **Roadmap lives in the repo** — `ROADMAP.md` with versioned milestones. v0.1 core worktree ops, v0.2 setup automation, v0.3 PR/forge integration, v0.4 JSON output.

5. **Docs and examples while developing** — `docs/` folder with one markdown per command. Real examples, not synthetic. Dogfood on this repo itself.

6. **Contributing/README always current** — README has a working quickstart that actually runs. `CONTRIBUTING.md` covers the dev loop. Changelog from commit one.

## Code Conventions

- All business logic lives in `internal/` — never in `cmd/`
- Use `cobra` for CLI command structure
- Error handling: wrap errors with context using `fmt.Errorf("doing X: %w", err)`
- No global state — pass dependencies explicitly
- Table-driven tests as the default test pattern
- Test files live next to the code they test (`foo_test.go` beside `foo.go`)

## Architecture

```
cmd/              # CLI entry points, wiring only
internal/
  vcs/            # backend-agnostic Worktree model, Manager interface, detection
  git/            # git worktree operations (implements vcs.Manager)
  jj/             # jj workspace operations (implements vcs.Manager)
  config/         # repo registry (~/.wtf/repos.json) + per-repo backend preference
  setup/          # project setup (env symlinking, PM detection, shell integration)
  forge/          # GitHub/GitLab integration
docs/             # one markdown per command, plus jj.md
```

## Version Control Backends

wtf drives both git worktrees and jj workspaces through `vcs.Manager`. `internal/cli`
never talks to `git` or `jj` directly — it resolves a `vcs.Manager` once per command
and calls through the interface.

- A git worktree and a jj workspace are both a `vcs.Worktree`. For jj, the workspace
  *name* fills the role of a branch name; jj enforces uniqueness, so it is a valid key.
- **Never create jj bookmarks implicitly.** Bookmarks are a push-time concern the
  user owns. wtf reads them for display only.
- Repo discovery must go through the backend. A secondary jj workspace contains no
  `.git`, so `git rev-parse --show-toplevel` fails there.
- Colocated repos (`.git` + `.jj`) are jj's *default* layout, not an edge case, so
  this path matters. They prompt once and persist the answer; non-TTY infers the
  backend from existing checkouts rather than guessing.
- Read-only jj commands pass `--ignore-working-copy` so listing has no side effects.

## Setup Model

Zero config. When a worktree is created (`wtf new` / `wtf news`), setup runs automatically:
1. Symlinks env files (`.env`, `.env.local`, etc.) from main worktree
2. Auto-detects package manager from lockfiles and runs install

CLI flags (`--no-setup`, `--no-env`, `--no-install`) override defaults. No config file.

## Pre-commit (prek)

Uses [prek](https://github.com/j178/prek) — a fast, Rust-based drop-in replacement for pre-commit. Config in `.pre-commit-config.yaml`.

```bash
prek install        # install git hooks locally
prek run --all-files  # run all hooks manually
```

**After making code changes, always run `prek run --all-files` before committing** to catch lint errors, formatting issues, and test failures early. If a hook fails and modifies files (e.g., formatting), review the changes and re-run until all hooks pass.

## CI Requirements

- All of `task all` must pass
- All prek hooks must pass
- Coverage gate: 90% minimum
- Lint must pass with zero warnings
- Tests must pass on Go 1.22+
