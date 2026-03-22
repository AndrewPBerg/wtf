[![codecov](https://codecov.io/github/AndrewPBerg/wtf/graph/badge.svg?token=DTXJ31R8RX)](https://codecov.io/github/AndrewPBerg/wtf)
[![CI](https://github.com/AndrewPBerg/wtf/actions/workflows/ci.yaml/badge.svg)](https://github.com/AndrewPBerg/wtf/actions/workflows/ci.yaml)

# WorkTreeForge (WTF)

A fast git worktree workflow tool. Create, switch, and clean up worktrees with automated project setup.

## Install

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/AndrewPBerg/wtf/main/install.sh | sh

# Windows (PowerShell) — download and run manually, don't pipe to iex
Invoke-WebRequest -Uri https://raw.githubusercontent.com/AndrewPBerg/wtf/main/install.ps1 -OutFile install.ps1
.\install.ps1
Remove-Item install.ps1

# Go (all platforms)
go install github.com/AndrewPBerg/wtf/cmd/wtf@latest

# Windows Package Manager (once published)
winget install AndrewPBerg.wtf
```

## Quickstart

```bash
# Set up shell integration (one-time)
wtf setup shell

# List all worktrees
wtf ls

# Create a worktree for a new branch
wtf new feature/auth
# Created worktree at /code/myrepo--feature-auth

# Create a worktree and switch to it in one step
wtf news feature/auth

# Switch to a worktree (cd's automatically with shell wrapper)
wtf sw auth

# Switch across all registered repos
wtf sw -g auth

# Remove a worktree and its branch
wtf rm feature/auth

# Clean up merged/prunable worktrees
wtf clean --dry-run
wtf clean

# Generate a .wt-forge.toml with auto-detected defaults
wtf config init

# Re-run setup in current worktree
wtf setup
```

## Project Setup (`.wt-forge.toml`)

Drop a `.wt-forge.toml` in your repo root to automate worktree setup:

```toml
[env]
strategy = "symlink"
files = [".env", ".env.local"]

[[setup]]
name = "install"
run = "pnpm install"

[[setup]]
name = "migrate"
run = "pnpm db:migrate"
if = "file exists 'prisma/schema.prisma'"
```

Generate one automatically with `wtf config init`. See [docs/wt-forge-toml.md](docs/wt-forge-toml.md) for the full reference.

## Shell Integration

`wtf sw` prints the worktree path to stdout (a subprocess can't `cd` your shell). The shell wrapper intercepts `wtf sw` to `cd` automatically. Tab completions are also included — one line handles everything.

```bash
# Automatic setup (recommended)
wtf setup shell

# Or manually add to your profile:

# bash — ~/.bashrc
eval "$(wtf init bash)"

# zsh — ~/.zshrc
eval "$(wtf init zsh)"

# fish — ~/.config/fish/config.fish
wtf init fish | source
```

After setup, `wtf sw auth` will `cd` into the matching worktree directly, and `wtf sw <TAB>` will show available worktrees.

## Worktree Path Convention

Worktrees are created as sibling directories to the main repo. Slashes in branch names become dashes:

```
/code/myrepo                    (main)
/code/myrepo--feature-auth      (feature/auth)
/code/myrepo--hotfix-bug        (hotfix/bug)
```

## Commands

### Worktree Operations

| Command     | Description                                          |
|-------------|------------------------------------------------------|
| `wtf ls`    | List worktrees (`--json`, `--global`)                |
| `wtf new`   | Create a worktree (`--base` to set origin branch)    |
| `wtf sw`    | Switch to a worktree (`--global` to search all repos)|
| `wtf rm`    | Remove a worktree and branch (`--force`)             |
| `wtf clean` | Remove merged/prunable worktrees (`--dry-run`)       |
| `wtf news`  | Create a worktree and switch to it (`--base`)        |

### Setup & Configuration

| Command            | Description                                            |
|--------------------|--------------------------------------------------------|
| `wtf setup`        | Run project setup in current worktree (`--env`, `--install`) |
| `wtf setup shell`  | Auto-configure shell integration (one-time)            |
| `wtf config init`  | Generate default `.wt-forge.toml` with auto-detection  |
| `wtf init`         | Print shell functions + tab completions for eval/source |
| `wtf completion`   | Generate shell completion script (`--shell`, `--install`) |

### Registry & Monitoring

| Command          | Description                                             |
|------------------|---------------------------------------------------------|
| `wtf repos`      | List all registered repos                               |
| `wtf unregister` | Remove a repo from the registry                         |
| `wtf watch`      | Watch PRs for changes and send notifications (`-g`, `-i`) |

### Tooling

| Command          | Description                                      |
|------------------|--------------------------------------------------|
| `wtf version`    | Print version                                    |
| `wtf update`     | Update to the latest version                     |
| `wtf uninstall`  | Remove the wtf binary (`--force` to skip prompt) |

See [docs/](docs/) for detailed command documentation.

## Development

Requires [Task](https://taskfile.dev):

```bash
task all            # fmt -> lint -> test -> build
task test-coverage  # run tests with 90% coverage gate
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full dev loop.
