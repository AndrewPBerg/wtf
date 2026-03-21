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
