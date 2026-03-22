# wtf completion

Generate shell completion scripts for wtf commands.

## Usage

```bash
wtf completion [--shell bash|zsh|fish|powershell]
```

## Flags

| Flag      | Description                                              |
|-----------|----------------------------------------------------------|
| `--shell` | Shell type (bash, zsh, fish, powershell). Auto-detected from `$SHELL` if omitted. |

## Examples

```bash
# Auto-detect shell and print completion script
$ wtf completion

# Generate for a specific shell
$ wtf completion --shell zsh

# Install completions (bash)
$ wtf completion --shell bash > /etc/bash_completion.d/wtf

# Install completions (zsh)
$ wtf completion --shell zsh > "${fpath[1]}/_wtf"

# Install completions (fish)
$ wtf completion --shell fish > ~/.config/fish/completions/wtf.fish
```
