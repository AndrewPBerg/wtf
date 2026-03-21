# wtf sw

Switch to a worktree by branch name (fuzzy substring match). Prints the path to stdout.

## Usage

```bash
wtf sw <query>
```

Since a subprocess can't change the parent shell's directory, `wtf sw` prints the path. Use a shell wrapper:

```bash
# Add to ~/.bashrc or ~/.zshrc
wt() { cd "$(command wtf sw "$@")" }
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
