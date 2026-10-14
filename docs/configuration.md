# Configuration

Project configuration lives in `.wtf/config.toml`; `wtf setup` generates it and
an editor schema, with empty hooks/tasks. Configuration format version is `1`;
it is separate from the executable's 1.0.0 release version. Unknown fields and
invalid types fail validation. Use `wtf doctor` to check without running callbacks.

```toml
#:schema ./schema.json
version = 1

[commands.list]
pr = false
checks = false
hook = true

[new]
after = ["copy"]

[tasks.test]
command = ["go", "test", "-p", "1", "./internal/..."]
timeout_seconds = 120
```

Hook registration and task definitions belong to the selected workspace. Command
flag defaults also inherit from global/parent configuration with the shared
resolver; hook arrays do not merge through that defaults mechanism. Per flag,
CLI wins, then leaf, nearest parent, outer parents, global, factory default.
Explicit false and empty values remain meaningful. See [schema](config-schema.md)
for configuration file selection and generated command definitions.

Defaults are loaded from `$WTF_HOME/config.toml` (normally
`~/.wtf-v1/config.toml`), then `.wtf/config.toml` files from the filesystem root
down to the invoking directory. Each defaults file requires `version = 1`.
Command/flag keys ignore case and aliases resolve to their canonical command.
Unknown commands, flags, or incorrectly typed values fail rather than silently
falling back. Lifecycle/task definitions are loaded from the JJ workspace root.

Use ordered before/after/failed/finally arrays on new/list/remove/invocation.
`[new].hooks` is the compatible post-creation shorthand. Alternatively select a
Lua registration entrypoint with `lua = "hooks/init.lua"`; entrypoint and
declarative phase arrays cannot be combined. [Hooks](hooks.md) covers both modes.

Only the named modules run. New workspaces copy the `.wtf/AGENTS.md` agent guide and template checksum,
configuration, schema, hooks,
tasks, and ignore rules before preparation; existing workspaces preserve edits.
`.wtf/local/` and runtime metadata are excluded. Task files are carried with the
tree but never automatically discovered as executable tasks.
