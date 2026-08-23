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
| `--no-env`        |       |         | Skip env file handling                               |
| `--copy-env`      |       |         | Copy env files instead of symlinking (agent-safe)    |
| `--no-install`    |       |         | Skip package manager install                         |
| `--no-serve`      |       |         | Skip starting the detected development server        |
| `--no-git-diff`   |       |         | Skip default Git metadata for jj editor diffs        |
| `--ensure`        |       |         | Idempotently return an existing matching workspace   |
| `--vcs`           |       |         | Force a backend in a colocated repo: `git` or `jj`   |

The positional branch, `--branch`, and `--pr` modes are mutually exclusive.

Numeric positional arguments (e.g. `42` or `#42`) are automatically detected as PR numbers -- no `--pr` flag needed.

## Jujutsu (jj)

In a jj repo this creates a **workspace** rather than a worktree, and the branch
argument names the workspace. Git-aware editor metadata is created by default;
use `--no-git-diff` or `WTF_JJ_GIT_DIFF=0` to opt out. `--base` accepts any jj
revset (a bookmark, `trunk()`,
or a change id); with no `--base`, wtf uses `trunk()` when it resolves to real work.
No bookmark is created — that stays yours via `jj bookmark create` or `jj git push -c`.

```bash
$ wtf new feat/auth
✔ Created workspace at /code/feat-auth--myrepo
  env: .env → symlink
  install: pnpm install
```

Env linking and install matter more here than under git: jj honors `.gitignore`, so
`.env` and `node_modules/` are not carried into a new workspace at all. See
[jj.md](jj.md).

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

# Create agent-safe worktree with copied env files
$ wtf new feature/auth --copy-env

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

1. **Handles env files** from the main worktree (`.env`, `.env.local`, `.env.development`, `.env.development.local` -- only files that exist), symlinking by default or copying with `--copy-env`
2. **Auto-detects the package manager** from lockfiles and runs install

No config file needed. Use `--copy-env` for isolated agent worktrees, or `--no-setup`, `--no-env`, or `--no-install` to skip.

Setup failures are reported as warnings -- the worktree is still created successfully. See [setup](setup.md) for details.

## PR Authentication

When using `--pr`, authentication reuses existing CLI tokens:

- **GitHub**: `gh auth token` (install [gh](https://cli.github.com/))
- **GitLab**: `glab auth token` or `GITLAB_TOKEN` env var (install [glab](https://gitlab.com/gitlab-org/cli))
