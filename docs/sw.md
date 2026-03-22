# wtf sw

Switch to a worktree by branch name (fuzzy substring match). Prints the path to stdout.

## Usage

```bash
wtf sw <query> [-g|--global]
wtf swg <query>
```

## Flags

| Flag              | Description                                    |
|-------------------|------------------------------------------------|
| `-g`, `--global`  | Search across all registered repos             |

`swg` is a shortcut for `sw -g`.

Since a subprocess can't change the parent shell's directory, `wtf sw` prints the path. Use `wtf setup shell` for automatic configuration, or see `wtf init --help` for manual setup.

```bash
# Automatic setup (recommended — includes tab completions)
wtf setup shell

# Or manually add to ~/.bashrc or ~/.zshrc
eval "$(wtf init bash)"
```

## Matching Behavior

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
