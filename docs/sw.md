# wtf sw

Switch to a worktree by branch name (fuzzy substring match). Prints the path to stdout.

## Usage

```bash
wtf sw <query>
```

Since a subprocess can't change the parent shell's directory, `wtf sw` prints the path. Use `wtf setup` for automatic configuration, or see `wtf init --help` for manual setup.

```bash
# Automatic setup (recommended)
wtf setup

# Or manually add to ~/.bashrc or ~/.zshrc
eval "$(wtf init)"
```

## Matching Behavior

- **One match**: prints the worktree path
- **Multiple matches**: lists matching branches and errors
- **No match**: errors with message

## Examples

```bash
# Exact substring match
$ wtf sw auth
/code/myrepo--feature-auth

# With shell wrapper
$ wt auth
# (cds to /code/myrepo--feature-auth)
```
