# wtf news

Create a new worktree and switch to it in one step. Combines `wtf new` and `wtf sw`, plus runs project setup automatically.

## Usage

```bash
wtf news <branch> [--base <branch>]
```

## Flags

| Flag     | Default | Description                        |
|----------|---------|------------------------------------|
| `--base` | `main`  | Base branch to create the worktree from |

## What It Does

1. Creates a new worktree for `<branch>` (same as `wtf new`)
2. Prints the worktree path to stdout (for the shell wrapper to `cd`)
3. Loads `.wt-forge.toml` and runs project setup (env files, install, custom steps)
4. Setup failures are warnings — the worktree is still created

## Examples

```bash
# Create and switch to a feature branch
$ wtf news feature/auth
/code/myrepo--feature-auth
✔ Created worktree at /code/myrepo--feature-auth
✔ Setup complete

# Create from a specific base branch
$ wtf news hotfix/bug --base release/2.0
/code/myrepo--hotfix-bug
✔ Created worktree at /code/myrepo--hotfix-bug
```

## See Also

- [wtf new](new.md) — Create a worktree without switching
- [wtf sw](sw.md) — Switch to an existing worktree
- [wtf setup](setup.md) — Re-run setup on an existing worktree
