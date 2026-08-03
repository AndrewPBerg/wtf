# wtf clean

Remove worktrees for branches that are merged into main or marked as prunable.

## Usage

```bash
wtf clean [--dry-run] [--force]
```

## Flags

| Flag        | Description                                        |
|-------------|----------------------------------------------------|
| `--dry-run` | List worktrees that would be removed without acting |
| `--force`   | Force remove even with uncommitted changes          |

## Examples

```bash
# Preview what would be cleaned
$ wtf clean --dry-run
Would remove feature/done (merged)
Would remove old-experiment (prunable)

# Actually clean up
$ wtf clean
Removed feature/done (merged)
Removed old-experiment (prunable)
```

## Behavior

1. Lists all worktrees
2. Identifies the main branch
3. Gets branches merged into main (`git branch --merged <main>`)
4. Identifies prunable worktrees (broken gitdir references)
5. Removes matching worktrees (or lists them with `--dry-run`)

## Jujutsu (jj)

jj has no merged branch to test, so the equivalent of "merged" is a workspace whose
working-copy commit is empty *and* whose parent is already contained in `trunk()`.
Workspaces whose directory was deleted outside wtf are reported as `prunable`.
Anything holding real changes is left alone. See [jj.md](jj.md).
