[![codecov](https://codecov.io/github/AndrewPBerg/wtf/graph/badge.svg?token=DTXJ31R8RX)](https://codecov.io/github/AndrewPBerg/wtf)

# WorkTreeForge (WTF)

A fast git worktree workflow tool. Create, switch, and clean up worktrees with short commands.

## Install

```bash
go install github.com/AndrewPBerg/wtf/cmd/wtf@latest
```

## Quickstart

```bash
# List all worktrees
wtf ls

# Create a worktree for a new branch
wtf new feature/auth
# Created worktree at /code/myrepo--feature-auth

# Switch to a worktree (prints path — see shell wrapper below)
wtf sw auth
# /code/myrepo--feature-auth

# Remove a worktree and its branch
wtf rm feature/auth

# Clean up merged/prunable worktrees
wtf clean --dry-run
wtf clean
```

## Shell Wrapper

`wtf sw` prints the worktree path to stdout (a subprocess can't `cd` your shell). Add this wrapper to your shell profile:

```bash
# ~/.bashrc or ~/.zshrc
wt() { cd "$(command wtf sw "$@")" }
```

Then use `wt auth` to cd into the matching worktree.

## Worktree Path Convention

Worktrees are created as sibling directories to the main repo. Slashes in branch names become dashes:

```
/code/myrepo                    (main)
/code/myrepo--feature-auth      (feature/auth)
/code/myrepo--hotfix-bug        (hotfix/bug)
```

## Commands

| Command     | Description                                 |
|-------------|---------------------------------------------|
| `wtf ls`    | List all worktrees (`--json` for JSON)      |
| `wtf new`   | Create a worktree (`--base` to set origin)  |
| `wtf sw`    | Switch to a worktree (substring match)      |
| `wtf rm`    | Remove a worktree and branch (`--force`)    |
| `wtf clean` | Remove merged/prunable worktrees            |

See [docs/](docs/) for detailed command documentation.

## Development

Requires [Task](https://taskfile.dev):

```bash
task all            # fmt -> lint -> test -> build
task test-coverage  # run tests with 90% coverage gate
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full dev loop.
