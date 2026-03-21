# wtf ls

List all git worktrees for the current repository, or across all registered repos with `--global`.

## Usage

```bash
wtf ls [--json] [--global]
```

## Flags

| Flag              | Description                                    |
|-------------------|------------------------------------------------|
| `--json`          | Output in JSON format                          |
| `--global`, `-g`  | List worktrees across all registered repos     |

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

# List worktrees across all registered repos
$ wtf ls --global
myrepo (/home/user/code/myrepo)
  BRANCH        PATH                              HEAD
  main *        /home/user/code/myrepo             abc1234
  feature/x     /home/user/code/myrepo--feat-x     def5678

other (/home/user/code/other)
  BRANCH        PATH                              HEAD
  main *        /home/user/code/other              111aaaa

# JSON output for all repos
$ wtf ls -g --json
[
  {
    "repo": "/home/user/code/myrepo",
    "worktrees": [...]
  }
]
```

## Auto-registration

Repos are automatically registered in `~/.wtf/repos.json` whenever you run any `wtf` command inside a git repo. There is no manual registration step — just use `wtf` in your repos and `wtf ls --global` will find them.

Stale entries (deleted repos or non-git directories) are automatically pruned when `--global` is used.
