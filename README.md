[![codecov](https://codecov.io/github/AndrewPBerg/wtf/graph/badge.svg?token=DTXJ31R8RX)](https://codecov.io/github/AndrewPBerg/wtf)
[![CI](https://github.com/AndrewPBerg/wtf/actions/workflows/ci.yaml/badge.svg)](https://github.com/AndrewPBerg/wtf/actions/workflows/ci.yaml)

# WorkTreeForge (WTF)

A fast git worktree workflow tool. Create, switch, and clean up worktrees with short commands.

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
wtf setup

# List all worktrees
wtf ls

# Create a worktree for a new branch
wtf new feature/auth
# Created worktree at /code/myrepo--feature-auth

# Switch to a worktree (cd's automatically with shell wrapper)
wtf sw auth

# Switch across all registered repos
wtf sw -G auth

# Remove a worktree and its branch
wtf rm feature/auth

# Clean up merged/prunable worktrees
wtf clean --dry-run
wtf clean
```

## Shell Integration

`wtf sw` prints the worktree path to stdout (a subprocess can't `cd` your shell). The shell wrapper intercepts `wtf sw` to `cd` automatically.

```bash
# Automatic setup (recommended)
wtf setup

# Or manually add to your profile:

# bash/zsh — ~/.bashrc or ~/.zshrc
eval "$(wtf init)"

# fish — ~/.config/fish/config.fish
wtf init fish | source
```

After setup, `wtf sw auth` will `cd` into the matching worktree directly.

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

### Shell & Setup

| Command          | Description                                      |
|------------------|--------------------------------------------------|
| `wtf init`       | Print shell functions for eval/source            |
| `wtf setup`      | Auto-configure shell integration                 |
| `wtf completion` | Generate shell completion script (`--shell`)     |

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
