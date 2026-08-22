[![codecov](https://codecov.io/github/AndrewPBerg/wtf/graph/badge.svg?token=DTXJ31R8RX)](https://codecov.io/github/AndrewPBerg/wtf)
[![CI](https://github.com/AndrewPBerg/wtf/actions/workflows/ci.yaml/badge.svg)](https://github.com/AndrewPBerg/wtf/actions/workflows/ci.yaml)

# WorkTreeForge (WTF)

A fast worktree workflow tool for **git and [Jujutsu](https://jj-vcs.github.io/jj/)**. Create, switch, and clean up git worktrees or jj workspaces with automated project setup — zero config required.

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
```

## Quickstart

```bash
# Set up shell integration (one-time)
wtf setup shell

# List / pick worktrees interactively
wtf sw

# Create a worktree for a new branch
wtf new feature/auth
# ✔ Created worktree at /code/myrepo--feature-auth

# Create a worktree and switch to it in one step
wtf news feature/auth

# Checkout a PR by number
wtf new --pr 42

# Switch to a worktree (cd's automatically with shell wrapper)
wtf sw auth

# Switch across all registered repos
wtf sw -g auth

# Remove a worktree and its branch
wtf rm feature/auth

# Clean up merged/prunable worktrees
wtf clean --dry-run
wtf clean

# Re-run setup in current worktree
wtf setup
```

## Git and Jujutsu

The same commands drive both. wtf uses git worktrees in a `.git` repo and jj
workspaces in a `.jj` repo:

```bash
$ wtf new feat/auth          # in a jj repo
✔ Created workspace at /code/feat-auth--myrepo
  env: .env → symlink
  install: pnpm install
```

This matters most for setup: jj honors `.gitignore`, so a fresh jj workspace has no
`.env` and no installed dependencies until wtf links and installs them.

A repo that is **both** (colocated — jj's default layout) is the only ambiguous case.
wtf asks once and remembers your answer per-repo; `--vcs git|jj` and `WTF_VCS`
override it. Non-interactive shells never prompt: wtf infers the backend from the
checkouts the repo already has, so existing scripts are unaffected.

```
$ wtf new feat/auth
? myrepo is both a git and a jj repo — which should wtf use?
  [1] jj    workspace   (jj manages the working copy here)
  [2] git   worktree
Use which? [1-2] (saved for this repo)
```

See [docs/jj.md](docs/jj.md) for workspace naming, bookmarks, revsets, and listing.

## Automatic Setup

When you create a worktree with `wtf new` or `wtf news`, WTF automatically:

1. **Handles env files** (`.env`, `.env.local`, `.env.development`, `.env.development.local`) from the main worktree (symlink by default, copy with `--copy-env`)
2. **Detects your package manager** and runs install (pnpm, bun, yarn, npm, uv, poetry, go, cargo, and more)

No config file needed. Override with flags:

```bash
wtf new feature/auth --copy-env     # copy env files for isolated agent worktrees
wtf new feature/auth --no-setup     # skip everything
wtf new feature/auth --no-env       # skip env file handling
wtf new feature/auth --no-install   # skip package install
```

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

Worktrees and jj workspaces are created as sibling directories to the main repo. Slashes in names become dashes:

```
/code/myrepo                    (main)
/code/myrepo--feature-auth      (feature/auth)
/code/myrepo--hotfix-bug        (hotfix/bug)
```

## Commands

### Worktree Operations

| Command     | Description                                          |
|-------------|------------------------------------------------------|
| `wtf new`   | Create a worktree (`--base`, `--pr`, `--no-setup`)   |
| `wtf sw`    | Switch/list worktrees (`--global`, `--prs`, `--json`)|
| `wtf rm`    | Remove a worktree and branch (`--force`)             |
| `wtf clean` | Remove merged/prunable worktrees (`--dry-run`)       |
| `wtf news`  | Create a worktree and switch to it (`--base`)        |

All of these work on jj workspaces too. `--vcs git|jj` forces a backend in a repo
that is both.

### Setup

| Command            | Description                                            |
|--------------------|--------------------------------------------------------|
| `wtf setup`        | Run project setup in current worktree (`--env`, `--install`) |
| `wtf setup shell`  | Auto-configure shell integration (one-time)            |
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
| `wtf git-diff`   | Create or refresh Zed/Git diff metadata for a JJ workspace |
| `wtf version`    | Print version                                    |
| `wtf update`     | Update to the latest version                     |
| `wtf uninstall`  | Remove the wtf binary (`--force` to skip prompt) |

See [docs/](docs/) for detailed command documentation, and [docs/jj.md](docs/jj.md) for Jujutsu specifics.

## Pi extension

The canonical Pi extension lives in [`packages/pi-extension/`](packages/pi-extension/).
It keeps agent checkout creation on WTF's managed path instead of raw `git worktree
add` or `jj workspace add` commands.

Install or refresh it locally, then run `/reload` in existing Pi sessions:

```bash
./scripts/install-pi-extension.sh
```

## Development

Requires [Task](https://taskfile.dev):

```bash
task all                 # fmt -> lint -> test -> build
task test-coverage       # run tests with 90% coverage gate
task pi-extension-check  # lint, typecheck, and test the Pi extension
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full dev loop.
