# wtf ls

List all git worktrees for the current repository, or across all registered repos with `--global`.

## Usage

```bash
wtf ls [--json] [-g|--global]
wtf lsg [--json]
```

## Flags

| Flag              | Description                                    |
|-------------------|------------------------------------------------|
| `--json`          | Output in JSON format                          |
| `-g`, `--global`  | List worktrees across all registered repos     |

`lsg` is a shortcut for `ls -g` (same flags minus `--global`).

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
$ wtf lsg
▸ myrepo (/home/user/code/myrepo)
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

## Commit hyperlinks

In table output, commit hashes are rendered as clickable hyperlinks (using OSC 8 terminal escape sequences). Clicking a commit hash opens the commit on GitHub or GitLab. This requires a modern terminal emulator with hyperlink support. The remote URL is detected from the `origin` remote on a best-effort basis.

## Global mode

In global mode, the current repo is highlighted with a green `▸` indicator. Column widths are aligned consistently across all repos.

## Auto-registration

Repos are automatically registered in `~/.wtf/repos.json` whenever you run any `wtf` command inside a git repo. There is no manual registration step — just use `wtf` in your repos and `wtf ls --global` will find them.

Stale entries (deleted repos or non-git directories) are automatically pruned when `--global` is used.
