# wtf completion

Legacy completion documentation. The current V1 CLI does not expose a completion
command. See the [README](../README.md) for Bash navigation integration.

## Recommended Setup

The legacy CLI supported file-based completion scripts as described below.

## Usage

```bash
wtf completion [--shell bash|zsh|fish|powershell] [--install]
```

## Flags

| Flag        | Description                                              |
|-------------|----------------------------------------------------------|
| `--shell`   | Shell type (bash, zsh, fish, powershell). Auto-detected from `$SHELL` if omitted. |
| `--install` | Write completion file to the standard user-local path instead of stdout. |

## Examples

```bash
# Print completion script to stdout
$ wtf completion

# Install to standard user-local path
$ wtf completion --install
✔ Installed completions to ~/.local/share/bash-completion/completions/wtf

# Generate for a specific shell
$ wtf completion --shell zsh

# Manual file-based install (alternative to --install)
$ wtf completion --shell bash > /etc/bash_completion.d/wtf
$ wtf completion --shell zsh > "${fpath[1]}/_wtf"
$ wtf completion --shell fish > ~/.config/fish/completions/wtf.fish
```

## Dynamic Completions

Several commands provide context-aware tab completions:

| Command     | Completes                                        |
|-------------|--------------------------------------------------|
| `wtf sw`    | Active worktrees                                 |
| `wtf rm`    | Active worktrees                                 |
| `wtf new`   | Remote branches; `--pr` completes open PRs       |
| `wtf clean` | Merged/stale worktrees with reason (merged/prunable) |

These completion features describe the legacy CLI, not the current V1 command tree.
