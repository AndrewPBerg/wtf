# wtf setup

One-time interactive command that adds shell integration to your profile.

## Usage

```bash
wtf setup
```

## What It Does

1. Detects your shell (from `$SHELL` or parent process)
2. Finds the appropriate RC file (`~/.bashrc`, `~/.zshrc`, or `~/.config/fish/config.fish`)
3. Checks if `wtf init` is already present
4. Prompts for confirmation
5. Appends the eval/source line

## Examples

```bash
$ wtf setup
Detected shell: zsh
RC file: /home/user/.zshrc
Will add: eval "$(wtf init)"
Proceed? [y/N] y
Added to /home/user/.zshrc. Restart your shell or run: source /home/user/.zshrc
```

If already configured:

```bash
$ wtf setup
Shell integration already configured in /home/user/.zshrc
```
