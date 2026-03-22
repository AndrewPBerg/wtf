# wtf new

Create a new worktree for a branch, from a remote branch, or from a pull request.

## Usage

```bash
wtf new <branch> [--base <branch>]
wtf new --branch <name>
wtf new --pr <number|branch|title>
```

## Flags

| Flag              | Short | Default | Description                                          |
|-------------------|-------|---------|------------------------------------------------------|
| `--base`          |       | `main`  | Base branch to create from (positional mode only)    |
| `--branch`        | `-b`  |         | Fetch and track an existing remote branch            |
| `--pr`            | `-P`  |         | Checkout a pull request (number, branch, or title)   |

The three modes (positional branch, `--branch`, `--pr`) are mutually exclusive.

## Examples

```bash
# Create worktree for a new branch
$ wtf new feature/auth
✔ Created worktree at /code/myrepo--feature-auth

# Create from a specific base
$ wtf new hotfix/bug --base release/v2
✔ Created worktree at /code/myrepo--hotfix-bug

# Fetch and track a remote branch
$ wtf new --branch feature-x
✔ Created worktree at /code/myrepo--feature-x

# Checkout PR by number
$ wtf new --pr 42
✔ Checked out #42 → /code/myrepo--pr-42

# Checkout PR by branch name
$ wtf new -P feature-auth
✔ Checked out #42 → /code/myrepo--pr-42

# Checkout PR by title (fuzzy match)
$ wtf new -P "fix login"
✔ Checked out #43 → /code/myrepo--pr-43
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
4. Runs `on_create` hooks (or `on_pr_create` for PR mode)

Without a `.wt-forge.toml`, it auto-detects the package manager and runs install.

Setup failures are reported as warnings — the worktree is still created. See [setup](setup.md) and [.wt-forge.toml](wt-forge-toml.md) for details.

## PR Authentication

When using `--pr`, authentication reuses existing CLI tokens:

- **GitHub**: `gh auth token` (install [gh](https://cli.github.com/))
- **GitLab**: `glab auth token` or `GITLAB_TOKEN` env var (install [glab](https://gitlab.com/gitlab-org/cli))
