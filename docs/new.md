# wtf new

Create a new worktree for a branch. If the branch doesn't exist, it's created from the base branch.

## Usage

```bash
wtf new <branch> [--base <branch>]
```

## Flags

| Flag            | Default | Description                        |
|-----------------|---------|------------------------------------|
| `--base`        | `main`  | Base branch to create from         |

## Examples

```bash
# Create worktree for a new branch
$ wtf new feature/auth
Created worktree at /code/myrepo--feature-auth

# Create from a specific base
$ wtf new hotfix/bug --base release/v2
Created worktree at /code/myrepo--hotfix-bug
```

## Worktree Path Convention

Worktrees are created as sibling directories to the main repo:
- Main repo: `/code/myrepo`
- Branch `feature/auth` → `/code/myrepo--feature-auth`

Slashes in branch names are replaced with dashes.

## Automatic Setup

After creating the worktree, `wtf new` automatically runs project setup:

1. Loads `.wt-forge.toml` if present
2. Handles env files (symlink/copy)
3. Runs setup steps with condition evaluation
4. Runs `on_create` hooks

Without a `.wt-forge.toml`, it auto-detects the package manager and runs install.

Setup failures are reported as warnings — the worktree is still created. See [setup](setup.md) and [.wt-forge.toml](wt-forge-toml.md) for details.
