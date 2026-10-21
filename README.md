# WorkTreeForge (WTF) 1.0.1

WTF manages Jujutsu workspaces, local project hooks, and recorded tasks. JJ owns
revisions and bookmarks; WTF owns workspace setup, navigation, and cleanup.
The human CLI uses Charm's terminal UI and the active Omarchy palette when available.
Automation gets a single versioned JSON result.

**Supported platform: Linux.** JJ 0.46+ is required; GitHub CLI and the existing Codex
Linear MCP connection are optional metadata integrations. No model turns run.

```sh
# Install the published Linux release (verify/review the installer first).
curl -fsSL https://raw.githubusercontent.com/AndrewPBerg/wtf/v1.0.1/install.sh -o /tmp/wtf-install.sh
mkdir -p "$HOME/.local/bin"
VERSION=1.0.1 INSTALL_DIR="$HOME/.local/bin" bash /tmp/wtf-install.sh

# Inside an existing JJ repository:
wtf setup
wtf new .
wtf new feature-a -f @
wtf ls
```

New workspaces inherit config, schema, hooks, and tasks as independent copies.
`.wtf/local/` remains controlled by explicit project hooks. Setup starts with
empty hooks/tasks and ignores all local configuration. Nothing installs project
dependencies automatically.

```toml
# .wtf/config.toml
version = 1
[new]
after = ["hello_world"]
```

```lua
-- .wtf/hooks/hello_world.lua
local wtf = require("wtf")
return function(event)
  wtf.notify("Hello from " .. event.workspace.name, "success")
end
```

[Documentation](docs/README.md) · [Getting started](docs/getting-started.md) ·
[Command reference](docs/cli.md) · [Lua API](docs/lua-substrate.md) ·
[1.0.1 release notes](docs/releases/1.0.1.md) · [Migration](docs/migration.md)

## Commands

| Command | Purpose |
|---|---|
| `new PATH` | Create or prepare a workspace |
| `list`, `ls`, `sw` | Browse/select workspaces |
| `change [NAME] -m TEXT` | Update a workspace description |
| `remove`, `rm` | Review and remove owned workspaces |
| `setup`, `setup teardown` | Scaffold/remove local configuration |
| `hook list`, `hook run NAME` | Inspect/invoke registered hooks |
| `run TASK` | Execute a configured task |
| `ports list/claim/release` | Manage repository-local port claims |
| `doctor`, `doc` | Diagnose configuration, tools, and state |
| `completion SHELL`, `version` | Shell completion and version |

Names, aliases, and flag spellings are case-insensitive; values preserve case.
`-j` means JSON; `-f` means `--from`. Creation defaults to `trunk()`; `-f @`
selects the invoking workspace's current JJ change. `sw` has the same behavior as
`ls`. Source [scripts/wtf.bash](scripts/wtf.bash) for Bash directory navigation.

## Development

Go 1.26.1 and JJ are required for integration tests. Keep validation serial:

```sh
GOMAXPROCS=1 go build -p 1 -o bin/wtf ./cmd/wtf
GOMAXPROCS=1 go test -p 1 ./...
GOMAXPROCS=1 golangci-lint run --concurrency 1
GOMAXPROCS=1 go generate ./internal/cli
```

Release archives include this README, LICENSE, docs, examples, and the Bash
navigation wrapper. Verify downloads with `SHA256SUMS`. See
[release procedures](docs/releasing.md) for exact-source validation.
