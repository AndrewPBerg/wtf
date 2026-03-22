# wtf config

Manage project configuration.

## Subcommands

### `wtf config init`

Generate a default `.wt-forge.toml` for the current repository.

```bash
wtf config init
```

Auto-detects:
- **Default branch** from the main worktree
- **Env files** that exist in the repo (falls back to `.env`, `.env.local`)
- **Package manager** from lockfiles

If `.wt-forge.toml` already exists, prompts before overwriting. The generated config is also added to `.gitignore` automatically.

## Examples

```bash
# Generate config with auto-detected defaults
$ wtf config init
✔ Created .wt-forge.toml
✔ Added .wt-forge.toml to .gitignore

# Overwrite existing config
$ wtf config init
.wt-forge.toml already exists. Overwrite? [y/N] y
✔ Created .wt-forge.toml
```

## See Also

- [.wt-forge.toml reference](wt-forge-toml.md) — full config file documentation
