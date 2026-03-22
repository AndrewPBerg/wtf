# wtf init

Print shell functions and tab completions for wtf integration. Outputs code meant to be eval'd by your shell profile.

## Usage

```bash
wtf init [shell]
```

The `[shell]` argument is optional. If omitted, wtf auto-detects from `$SHELL` or the parent process.

## Shell Setup

### bash

Add to `~/.bashrc`:

```bash
eval "$(wtf init bash)"
```

### zsh

Add to `~/.zshrc`:

```bash
eval "$(wtf init zsh)"
```

### fish

Add to `~/.config/fish/config.fish`:

```fish
wtf init fish | source
```

## What It Does

1. **Shell wrapper** — Intercepts `wtf sw` and `wtf news` to `cd` into the matched worktree directory. All other subcommands pass through to the binary.
2. **Tab completions** — Registers context-aware tab completions for all commands. Completions are always in sync with the installed binary version.

This means `eval "$(wtf init)"` is all you need — no separate completion script to source or install.

## Automatic Setup

Instead of editing your profile manually, run `wtf setup shell` to configure this automatically.

## Examples

```bash
# Print bash functions + completions
$ wtf init bash
wtf() { case "$1" in sw|news) ...
# wtf completions
# bash completion V2 for wtf ...

# Print fish functions + completions
$ wtf init fish
function wtf; ...
# wtf completions
complete -c wtf ...
```
