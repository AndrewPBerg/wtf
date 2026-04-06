# wtf unregister

Remove a repo from the wtf registry (`~/.wtf/repos.json`). This stops wtf from including the repo in global operations like `wtf sw -g` and `wtf rm -g`.

**This command does not modify your repo, its `.git` directory, or any worktrees.** It only removes the entry from wtf's own registry file.

## Usage

```bash
wtf unregister [path]
```

If `[path]` is omitted, unregisters the repo you're currently inside.

## Examples

```bash
# Unregister the current repo
$ cd ~/projects/old-project
$ wtf unregister
✔ Unregistered /home/user/projects/old-project

# Unregister a specific repo by path
$ wtf unregister ~/projects/archived-repo
✔ Unregistered /home/user/projects/archived-repo

# Tab-complete from registered repos
$ wtf unregister <TAB>
/home/user/projects/my-app    /home/user/projects/my-lib
```

## Behavior

1. Resolves the given path (or the current repo root) to an absolute path
2. Removes the path from `~/.wtf/repos.json`
3. If the path is not in the registry, returns an error

**What it does NOT do:**

- Does not delete the repo directory
- Does not modify `.git` or any git configuration
- Does not remove worktrees
- Does not touch any files in the repo

It purely removes wtf's awareness of the repo for global commands.

## Notes

- Repos are auto-registered when you run any wtf command inside them, so re-registering is as simple as running `wtf sw` in the repo again.
- To see which repos are currently registered, check `~/.wtf/repos.json`.
- Stale entries (deleted repos) are also cleaned up automatically by `wtf` via pruning.
