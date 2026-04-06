# wtf rm

Remove one or more worktrees and delete their branches. With no arguments, launches an interactive multi-select picker.

## Usage

```bash
wtf rm [branch...] [--force] [-g|--global]
wtf rmg [branch...] [--force]
```

## Flags

| Flag             | Description                                    |
|------------------|------------------------------------------------|
| `--force`        | Force remove even with uncommitted changes     |
| `-g`, `--global` | Remove worktree across all registered repos    |

## No Arguments — Interactive Picker

When called with no arguments in a TTY, `wtf rm` launches an interactive multi-select picker:

- Navigate with **j/k** or **up/down** arrows
- Press **Space** to toggle selection on the current worktree
- Press **Enter** to confirm and remove all selected worktrees
- Press **q**, **Esc**, or **Ctrl+C** to cancel

The main worktree and the worktree you're currently inside are excluded from the picker.

```bash
# Interactive multi-select
$ wtf rm
Select worktrees (space=toggle, enter=confirm, q=cancel)

▸ [ ] feature/auth    /code/myrepo--feat-auth   def4567
  [x] feature/wip     /code/myrepo--feat-wip    ghi7890
  [ ] bugfix/typo     /code/myrepo--bugfix-typo abc1234

# Interactive global multi-select
$ wtf rmg
Select worktrees (space=toggle, enter=confirm, q=cancel)

▸ [ ] feature/auth    /code/myrepo--feat-auth   def4567  (myrepo)
  [ ] feature/api     /code/other--feat-api     abc1234  (other)
```

When not in a TTY (piped/scripted), bare `rm`/`rmg` returns an error asking for branch names.

## Examples

```bash
# Remove a worktree (branch must be fully merged)
$ wtf rm feature/auth
✔ Removed worktree for feature/auth

# Remove multiple worktrees at once
$ wtf rm feature/auth feature/wip bugfix/typo
✔ Removed worktree for feature/auth
✔ Removed worktree for feature/wip
✔ Removed worktree for bugfix/typo

# Force remove with uncommitted changes
$ wtf rm feature/wip --force
✔ Removed worktree for feature/wip

# Remove a worktree from any registered repo
$ wtf rm -g feature/auth
✔ Removed worktree for feature/auth (my-project)

# Remove multiple worktrees globally
$ wtf rmg feature/auth feature/wip
✔ Removed worktree for feature/auth (my-project)
✔ Removed worktree for feature/wip (my-project)
```

## Behavior

1. Finds each worktree by substring match on branch name
2. Runs `git worktree remove <path>` (with `--force` if specified)
3. Deletes the branch with `git branch -d` (or `-D` with `--force`)
4. Refuses to remove the main worktree
5. When removing multiple branches, continues on failure and reports a summary (e.g. "failed to remove 1 of 3 worktrees")

### Global mode (`-g` / `rmg`)

1. Searches all registered repos (`~/.wtf/repos.json`) for matching worktrees
2. If exactly one match is found per branch, removes it
3. If multiple matches are found for a branch, prints them and asks you to disambiguate
4. If no match is found, reports an error
5. Processes each branch independently — partial failures don't stop the rest
