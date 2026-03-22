# wtf setup

Run project setup in the current worktree.

## Usage

```bash
wtf setup              # full setup (env files + install + custom steps)
wtf setup --env        # only handle env files
wtf setup --install    # only run package install
wtf setup shell        # configure shell integration (one-time)
```

## What It Does

1. Loads [`.wt-forge.toml`](https://github.com/AndrewPBerg/wtf/blob/main/docs/wt-forge-toml.md) if present
2. Handles env files according to the configured strategy
3. Auto-detects the package manager and runs install
4. Runs custom setup steps from config
5. Runs `on_create` hooks

Without a `.wt-forge.toml`, it auto-detects the package manager and runs install. See the [`.wt-forge.toml` reference](https://github.com/AndrewPBerg/wtf/blob/main/docs/wt-forge-toml.md) for full config options.

## Examples

```bash
# Re-run setup in current worktree
$ wtf setup
✔ Setup complete

# Only install packages
$ wtf setup --install
✔ Ran pnpm install

# Only handle env files
$ wtf setup --env
✔ Env files handled
```

## Shell Integration

Shell integration has moved to `wtf setup shell`:

```bash
$ wtf setup shell
Detected shell: zsh
RC file: /home/user/.zshrc
Will add: eval "$(wtf init)"
Proceed? [y/N] y
✔ Added to /home/user/.zshrc. Restart your shell or run: source /home/user/.zshrc
```

If already configured:

```bash
$ wtf setup shell
✔ Shell integration already configured in /home/user/.zshrc
```
