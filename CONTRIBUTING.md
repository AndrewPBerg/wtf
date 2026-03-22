# Contributing to WorkTreeForge (WTF)

## Prerequisites

- **Go 1.22+** — [install guide](https://go.dev/doc/install)
- **Task** (Taskfile runner) — `brew install go-task` or `go install github.com/go-task/task/v3/cmd/task@latest`
- **golangci-lint** — `brew install golangci-lint` or see [install docs](https://golangci-lint.run/welcome/install/)
- **prek** (pre-commit hooks) — `cargo install prek` or see [prek repo](https://github.com/j178/prek)

## Getting Started

```bash
# Clone the repo
git clone https://github.com/AndrewPBerg/wtf.git
cd wtf

# Install the CLI locally from source
go install ./cmd/wtf

# Install pre-commit hooks
prek install
```

After `go install ./cmd/wtf`, the `wtf` binary is available in your `$GOPATH/bin` (or `$GOBIN`). Make sure that directory is in your `$PATH`.

## Dev Loop

```bash
task fmt            # format code (gofmt + goimports)
task lint           # run golangci-lint
task test           # run all tests
task build          # build binary to bin/wtf
task all            # fmt → lint → test → build (the full pipeline)
task test-coverage  # run tests with coverage report (fails below 90%)
task tidy           # go mod tidy
task clean          # remove build artifacts
```

## Pre-commit Hooks (prek)

We use [prek](https://github.com/j178/prek), a fast Rust-based pre-commit hook runner. Config lives in `.pre-commit-config.yaml`.

```bash
# Install hooks into your local git repo (one-time setup)
prek install

# Run all hooks manually (useful before pushing)
prek run --all-files
```

Hooks run automatically on every `git commit`. If a hook fails and modifies files (e.g., formatting), review the changes, `git add` them, and commit again.

### What the hooks check

- **golangci-lint** — linting
- **go-fmt** — formatting
- **go-vet-mod** — vet with modules
- **go-build-mod** — build check
- **go-test-mod** — runs tests
- **trailing-whitespace** / **end-of-file-fixer** — file hygiene
- **check-yaml** / **check-merge-conflict** — safety checks
- **check-added-large-files** — blocks files over 500KB

## Code Conventions

- All business logic lives in `internal/` — never in `cmd/`
- Use `cobra` for CLI commands
- Wrap errors with context: `fmt.Errorf("doing X: %w", err)`
- Use sentinel errors (e.g., `git.ErrWorktreeNotFound`) for errors that callers need to match on
- No global state — pass dependencies explicitly
- Table-driven tests as the default pattern
- Test files live next to the code they test

## Testing

- **90% coverage minimum** enforced by CI
- Use `testify` for assertions (`assert`, `require`)
- Integration tests create real temp git repos via `initTestRepo(t)`
- Run `task test-coverage` to check your coverage locally

## Project Structure

```
cmd/wtf/          # CLI entry point — wiring only, no logic
internal/
  cli/            # Cobra commands, output formatting
  git/            # Git/worktree operations (Executor interface)
  config/         # Configuration loading, registry (~/.wtf/repos.json), .wt-forge.toml
  setup/          # Project setup automation, shell detection, env file handling
docs/             # One markdown file per command
```

## Submitting Changes

1. Create a feature branch from `main`
2. Make your changes
3. Run `prek run --all-files` to verify all hooks pass
4. Run `task all` for the full pipeline
5. Open a PR against `main`
