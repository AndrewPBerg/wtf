# wtf init

Print shell functions for wtf integration. Outputs code meant to be eval'd by your shell profile.

## Usage

```bash
wtf init [shell]
```

The `[shell]` argument is optional. If omitted, wtf auto-detects from `$SHELL` or the parent process.

## Shell Setup

### bash / zsh

Add to `~/.bashrc` or `~/.zshrc`:

```bash
eval "$(wtf init)"
```

### fish

Add to `~/.config/fish/config.fish`:

```fish
wtf init fish | source
```

## What It Does

Wraps the `wtf` command as a shell function that intercepts `wtf sw` to `cd`
into the matched worktree directory. All other subcommands pass through to the binary.

## Automatic Setup

Instead of editing your profile manually, run `wtf setup` to configure this automatically.

## Examples

```bash
# Print bash/zsh function
$ wtf init bash
wtf() { if [ "$1" = "sw" ]; then shift; builtin cd "$(command wtf sw "$@")" || return; else command wtf "$@"; fi; }

# Auto-detect shell
$ wtf init
wtf() { if [ "$1" = "sw" ]; then shift; ...
```
