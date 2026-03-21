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
