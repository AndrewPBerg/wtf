# wtf watch

Watch pull requests for changes and send native desktop notifications.

Polls the forge API at a configurable interval and notifies on:

- New PRs opened
- PRs closed or merged
- Review status changes (approved, changes requested)
- Draft status changes

## Usage

```bash
wtf watch [-g|--global] [-i|--interval <seconds>] [--no-desktop]
```

## Flags

| Flag                   | Description                                    |
|------------------------|------------------------------------------------|
| `-g`, `--global`       | Watch all registered repos                     |
| `-i`, `--interval`     | Poll interval in seconds (default 60, min 10)  |
| `--no-desktop`         | Disable desktop notifications (terminal only)  |

## Examples

```bash
# Watch current repo
$ wtf watch

# Watch all registered repos
$ wtf watch -g

# Poll every 30 seconds
$ wtf watch -i 30

# Terminal-only notifications (no desktop popups)
$ wtf watch --no-desktop
```

## Configuration

The poll interval and desktop notifications can be configured per-project in `.wt-forge.toml`:

```toml
[watch]
interval = 30     # poll interval in seconds (default 60, min 10)
desktop = false   # set to false for terminal-only notifications
```

CLI flags take precedence over config file values.

## Behavior

- **Single repo**: Polls the forge API for the current repo's PRs and prints changes to the terminal. Desktop notifications are sent by default.
- **Global mode** (`-g`): Watches all registered repos concurrently. Each repo is color-coded in terminal output.
- **Minimum interval**: The polling interval is clamped to a minimum of 10 seconds to avoid API rate limits.
- **State tracking**: PR state is persisted in `.git/wtf/` so only changes since the last poll trigger notifications.
- **Graceful shutdown**: Press `Ctrl+C` to stop watching.

## Requirements

Requires `gh` (GitHub) or `glab` (GitLab) CLI for authentication.
