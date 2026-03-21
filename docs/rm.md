# wtf rm

Remove a worktree and delete its branch.

## Usage

```bash
wtf rm <branch> [--force]
```

## Flags

| Flag      | Description                                    |
|-----------|------------------------------------------------|
| `--force` | Force remove even with uncommitted changes     |

## Examples

```bash
# Remove a worktree (branch must be fully merged)
$ wtf rm feature/auth
Removed worktree for feature/auth

# Force remove with uncommitted changes
$ wtf rm feature/wip --force
Removed worktree for feature/wip
```

## Behavior

1. Finds the worktree by substring match on branch name
2. Runs `git worktree remove <path>` (with `--force` if specified)
3. Deletes the branch with `git branch -d` (or `-D` with `--force`)
4. Refuses to remove the main worktree
