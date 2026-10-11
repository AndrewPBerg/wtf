# wtf new

Create or prepare a JJ workspace at an exact destination path.

```sh
wtf new PATH [--from REVISION] [--message TEXT] [--json] [-- HOOK_ARGS...]
```

Relative paths resolve from the invoking directory. Absolute paths also work.
Bare `wtf` shows help; use `wtf new .` to prepare the current workspace.

| Destination | Behavior |
|---|---|
| Existing JJ workspace root | Register and prepare it in place |
| Missing path | Create a workspace at that exact location, then prepare it |
| Existing file or ordinary directory | Return an error without changing it |
| Directory inside a workspace | Return an error; specify the workspace root |

For creation, the destination basename supplies the native JJ workspace name.
It must be unique in the repository and contain 1–80 letters, digits, dots,
underscores or dashes, starting with a letter or digit. Parent paths retain their
case and may contain spaces. The source must be a registered JJ repository;
run `wtf new .` there first. No bookmark is created.

| Flag | Short | Default | Meaning |
|---|---|---|---|
| `--from` | `-f` | `trunk()` | JJ revision for creation; unused for existing workspaces |
| `--message` | `-m` | unset | Workspace description; an empty string clears it |
| `--json` | `-j` | `false` | Emit one versioned JSON result |

```sh
# Prepare the current workspace
wtf new .

# Create a sibling at the exact path, based on the current revision
wtf new ../AgentCase -f @

# Create at a local path (no automatic sibling placement or repository suffix)
wtf new ./experiment -f @

# Describe a workspace
wtf new hello_world -m "hello this me testing"

# Prepare an existing workspace, even from outside a repository
wtf new /code/AgentCase

# Pass project-owned options unchanged to preparation hooks
wtf new ../sandbox -f @ -- --compose --test

# Retry preparation at the same path
wtf new ../sandbox -- --compose --test
```

Both modes invoke the configured Lua `init` override. Only creation invokes the
Lua `new` override first, allowing checks to block before mutation. Creation
requests include `request.name`, `request.from`, `request.path` (the fixed absolute
destination), and `request.args`. Project arguments must follow `--`.

Preparation failures remain errors and retain the workspace and completed stages.
Repeated calls preserve identity, ownership, and reusable stage records. Registering
an existing native workspace never grants WTF permission to delete it. Symlinks to
workspace roots resolve to the same identity. JSON data includes `code: initialized`
for first registration or creation, and `code: already_initialized` on retries.
Hook messages, child output, and diagnostics go to stderr.

Descriptions appear below the selected row in `list`/`ls` and `remove`/`rm`,
alongside any removal safety warning. They retain their case and original text in
JSON's `message` field and in Lua's `ctx.workspace.message`. Display truncates long
descriptions to fit the terminal and sanitizes terminal controls. Repeated `new`
without a message preserves the existing description; supplying `-m` updates it,
and `-m ""` clears it. Inherited `[commands.new]` message defaults use the common
flag precedence. Descriptions survive failed preparation and require no migration.

Initialization remains an internal lifecycle phase; `new PATH` is its public
entry point. See [hooks](hooks.md) for project preparation.
