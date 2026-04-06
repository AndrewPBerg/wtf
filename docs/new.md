# wtf new

Create a new worktree for a branch, from a remote branch, or from a pull request.

## Usage

```bash
wtf new <branch> [--base <branch>]
wtf new <number>
wtf new --branch <name>
wtf new --pr <number|branch|title>
```

## Flags

| Flag              | Short | Default | Description                                          |
|-------------------|-------|---------|------------------------------------------------------|
| `--base`          |       | `main`  | Base branch to create from (positional mode only)    |
| `--branch`        | `-b`  |         | Fetch and track an existing remote branch            |
| `--pr`            | `-P`  |         | Checkout a pull request (number, branch, or title)   |
| `--no-setup`      |       |         | Skip all post-create setup (env files and install)   |
| `--no-env`        |       |         | Skip env file symlinking                             |
| `--no-install`    |       |         | Skip package manager install                         |

The positional branch, `--branch`, and `--pr` modes are mutually exclusive.

Numeric positional arguments (e.g. `42` or `#42`) are automatically detected as PR numbers -- no `--pr` flag needed.

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

# Checkout PR by number (auto-detected)
$ wtf new 42
→ Detected PR number, checking out PR #42…
✔ Checked out #42 → /code/myrepo--pr-42

# Same thing with a hash prefix
$ wtf new #42
→ Detected PR number, checking out PR #42…
✔ Checked out #42 → /code/myrepo--pr-42

# Checkout PR by branch name or title (requires --pr flag)
$ wtf new -P feature-auth
✔ Checked out #42 → /code/myrepo--pr-42

$ wtf new -P "fix login"
✔ Checked out #43 → /code/myrepo--pr-43

# Create worktree without running setup
$ wtf new feature/auth --no-setup

# Create worktree but skip package install
$ wtf new feature/auth --no-install
```

## Worktree Path Convention

Worktrees are created as sibling directories to the main repo:
- Main repo: `/code/myrepo`
- Branch `feature/auth` → `/code/myrepo--feature-auth`

Slashes in branch names are replaced with dashes.

## Automatic Setup

After creating the worktree, `wtf new` automatically:

1. **Symlinks env files** from the main worktree (`.env`, `.env.local`, `.env.development`, `.env.development.local` -- only files that exist)
2. **Auto-detects the package manager** from lockfiles and runs install

No config file needed. Use `--no-setup`, `--no-env`, or `--no-install` to skip.

Setup failures are reported as warnings -- the worktree is still created successfully. See [setup](setup.md) for details.

## PR Authentication

When using `--pr`, authentication reuses existing CLI tokens:

- **GitHub**: `gh auth token` (install [gh](https://cli.github.com/))
- **GitLab**: `glab auth token` or `GITLAB_TOKEN` env var (install [glab](https://gitlab.com/gitlab-org/cli))
