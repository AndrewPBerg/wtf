# wtf ls

List all git worktrees for the current repository.

## Usage

```bash
wtf ls [--json]
```

## Flags

| Flag     | Description              |
|----------|--------------------------|
| `--json` | Output in JSON format    |

## Examples

```bash
# Table output (default)
$ wtf ls
BRANCH        PATH                          HEAD
main *        /code/myrepo                  abc1234
feature/auth  /code/myrepo--feature-auth    def4567

# JSON output
$ wtf ls --json
[
  {
    "path": "/code/myrepo",
    "branch": "main",
    "head": "abc1234...",
    "is_main": true,
    "is_bare": false,
    "is_detached": false,
    "prunable": false
  }
]
```
