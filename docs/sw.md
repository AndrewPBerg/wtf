# wtf sw

Switch to a worktree by branch name (fuzzy substring match), or list all worktrees interactively when called with no arguments.

## Usage

```bash
wtf sw [query] [-g|--global] [--prs]
wtf swg [query] [--prs]
```

## Flags

| Flag              | Description                                    |
|-------------------|------------------------------------------------|
| `-g`, `--global`  | Search across all registered repos             |
| `-p`, `--prs`     | Show PR status for each worktree (list mode)   |

`swg` is a shortcut for `sw -g`.

Since a subprocess can't change the parent shell's directory, `wtf sw` prints the path. Use `wtf setup shell` for automatic configuration, or see `wtf init --help` for manual setup.

```bash
# Automatic setup (recommended — includes tab completions)
wtf setup shell

# Or manually add to ~/.bashrc or ~/.zshrc
eval "$(wtf init bash)"
```

## No Arguments — Interactive Picker

When called with no arguments, `wtf sw` launches an interactive worktree picker:

- Navigate with **j/k** or **up/down** arrows
- Press **Enter** to switch to the highlighted worktree
- Press **q**, **Esc**, or **Ctrl+C** to cancel

When output is piped (non-TTY), a static table is printed instead.

```bash
# Interactive picker
$ wtf sw
Select a worktree to switch to (enter=select, q=cancel)

▸ main *          /code/myrepo             abc1234
  feature/auth    /code/myrepo--feat-auth  def4567

# Interactive global picker
$ wtf swg
Select a worktree to switch to (enter=select, q=cancel)

▸ main *          /code/myrepo     abc1234  (myrepo)
  feature/auth    /code/other      def4567  (other)

# Non-interactive (piped) — static table
$ wtf sw | head
BRANCH        PATH                          HEAD
main *        /code/myrepo                  abc1234
feature/auth  /code/myrepo--feature-auth    def4567
```

## With Arguments — Matching Behavior

- **One match**: prints the worktree path
- **Multiple matches**: lists matching branches and errors
- **No match**: errors with fuzzy suggestions ("Did you mean?")

## Examples

```bash
# Substring match
$ wtf sw auth
/code/myrepo--feature-auth

# With shell wrapper (cd's automatically)
$ wtf sw auth
Switched to /code/myrepo--feature-auth

# Search across all registered repos
$ wtf sw -g auth
/home/user/code/myrepo--feature-auth

# Same, using the shortcut command
$ wtf swg auth
/home/user/code/myrepo--feature-auth

# No match — shows suggestions
$ wtf sw atuh
error: no worktree found matching atuh

Did you mean?
  → feature/auth
```

## PR status (`--prs`)

When `--prs` is set (list mode, no arguments), an additional PR column shows the associated pull request for each worktree branch:

```bash
$ wtf sw --prs
BRANCH        PATH                          HEAD      PR
main *        /code/myrepo                  abc1234
feature/auth  /code/myrepo--feature-auth    def4567   #42 Add authentication ✔
fix/login     /code/myrepo--fix-login       ghi7890   #43 Fix login bug ○
```

PR numbers are clickable hyperlinks (like commit hashes). Review status icons:
- `✔` approved
- `✖` changes requested
- `○` review pending
- `draft` for draft PRs

PR data uses lazy loading for instant results. Cached data is displayed immediately, while fresh data is fetched in the background. Requires `gh` or `glab` CLI for authentication.

## Commit hyperlinks

In table output, commit hashes are rendered as clickable hyperlinks (using OSC 8 terminal escape sequences). Clicking a commit hash opens the commit on GitHub or GitLab.

## Global mode

In global mode, the current repo is highlighted with a green `▸` indicator. Column widths are aligned consistently across all repos.

## Auto-registration

Repos are automatically registered in `~/.wtf/repos.json` whenever you run any `wtf` command inside a git repo. There is no manual registration step — just use `wtf` in your repos and `wtf sw --global` will find them.

Stale entries (deleted repos or non-git directories) are automatically pruned when `--global` is used.
