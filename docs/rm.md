# wtf rm

Remove a worktree and delete its branch.

## Usage

```bash
wtf rm <branch> [--force] [-g|--global]
wtf rmg <branch> [--force]
```

## Flags

| Flag             | Description                                    |
|------------------|------------------------------------------------|
| `--force`        | Force remove even with uncommitted changes     |
| `-g`, `--global` | Remove worktree across all registered repos    |

## Examples

```bash
# Remove a worktree (branch must be fully merged)
$ wtf rm feature/auth
Removed worktree for feature/auth

# Force remove with uncommitted changes
$ wtf rm feature/wip --force
Removed worktree for feature/wip

# Remove a worktree from any registered repo
$ wtf rm -g feature/auth
Removed worktree for feature/auth (my-project)

# Same, using the shortcut command
$ wtf rmg feature/auth
Removed worktree for feature/auth (my-project)
```

## Behavior

1. Finds the worktree by substring match on branch name
2. Runs `git worktree remove <path>` (with `--force` if specified)
3. Deletes the branch with `git branch -d` (or `-D` with `--force`)
4. Refuses to remove the main worktree

### Global mode (`-g` / `rmg`)

1. Searches all registered repos (`~/.wtf/repos.json`) for a matching worktree
2. If exactly one match is found, removes it
3. If multiple matches are found, prints them and asks you to disambiguate
4. If no match is found, reports an error
