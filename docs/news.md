# wtf news

Create a new worktree and switch to it in one step. Combines `wtf new` and `wtf sw`.

## Usage

```bash
wtf news <branch> [--base <branch>]
wtf news <number>
wtf news --branch <name>
wtf news --pr <number|branch|title>
```

## Flags

| Flag              | Short | Default | Description                                          |
|-------------------|-------|---------|------------------------------------------------------|
| `--base`          |       | `main`  | Base branch to create the worktree from (positional mode only) |
| `--branch`        | `-b`  |         | Fetch and track an existing remote branch            |
| `--pr`            | `-P`  |         | Checkout a pull request (number, branch, or title)   |
| `--no-setup`      |       |         | Skip all post-create setup (env files and install)   |
| `--no-env`        |       |         | Skip env file handling                               |
| `--copy-env`      |       |         | Copy env files instead of symlinking (agent-safe)    |
| `--no-install`    |       |         | Skip package manager install                         |
| `--no-git-diff`   |       |         | Skip default Git metadata for JJ editor diffs        |

The three modes (positional branch, `--branch`, `--pr`) are mutually exclusive.

Numeric positional arguments (e.g. `42` or `#42`) are automatically detected as PR numbers.

## What It Does

1. Creates a new worktree for `<branch>` (same as `wtf new`)
2. Prints the worktree path to stdout (for the shell wrapper to `cd`)
3. Handles env files from the main worktree (symlink by default, copy with `--copy-env`) and runs package install
4. Creates Git diff metadata by default for secondary JJ workspaces
5. Setup failures are warnings -- the worktree is still created

## Examples

```bash
# Create and switch to a feature branch
$ wtf news feature/auth
/code/myrepo--feature-auth
✔ Created worktree at /code/myrepo--feature-auth

# Create from a specific base branch
$ wtf news hotfix/bug --base release/2.0
/code/myrepo--hotfix-bug
✔ Created worktree at /code/myrepo--hotfix-bug

# Fetch remote branch and switch
$ wtf news --branch feature-x
/code/myrepo--feature-x
✔ Created worktree at /code/myrepo--feature-x

# Checkout PR and switch
$ wtf news 42
/code/myrepo--pr-42
✔ Checked out #42 → /code/myrepo--pr-42

# Checkout PR by title
$ wtf news -P "fix login bug"
/code/myrepo--pr-43
✔ Checked out #43 → /code/myrepo--pr-43

# Create an agent-safe worktree with copied env files
$ wtf news feature/agent --copy-env

# Skip setup
$ wtf news feature/quick --no-setup
```

## See Also

- [wtf new](new.md) -- Create a worktree without switching
- [wtf sw](sw.md) -- Switch to an existing worktree
- [wtf setup](setup.md) -- Re-run setup on an existing worktree
