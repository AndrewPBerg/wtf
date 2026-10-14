# wtf new

Create or prepare a JJ workspace at an exact destination path.

```sh
wtf new PATH [--from REVISION] [--message TEXT] [--json] [-- HOOK_ARGS...]
```

Bare names create siblings of the registered main repository, without a repository
prefix or suffix. This stays flat when invoked from another workspace or a
subdirectory. Explicit relative paths (`./...`, `../...`, or paths containing `/`)
resolve from the invoking directory. Absolute paths also work. Outside a JJ
repository, a relative name can still prepare an existing workspace in that directory.
Bare `wtf` shows help; use `wtf new .` to prepare the current workspace.

```text
projects/
  my_repo/
  new_work/       # wtf new new_work from my_repo
  another_work/  # wtf new another_work from new_work
```

`wtf list` / `wtf ls` shows workspaces inside the main repository with their
relative placement, such as `./hello_vert` or `./work/deep`. Sibling workspaces
keep their plain names. These display labels do not change native JJ names or
JSON identity fields.

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

WTF stores its local identity marker in `.jj/wtf-workspace.json`, outside source
tracking. Creating a workspace does not require a project `.gitignore` rule for
this metadata. Existing `.wtf/workspace.json` markers remain readable for compatibility.

New workspaces inherit `.wtf/config.toml`, `.wtf/schema.json`, `.wtf/hooks/`,
and `.wtf/tasks/` from the invoking workspace root, even when invoked from a
subdirectory. This happens before initialization and creation callbacks. Files
are independent copies; nested directories and empty hook/task directories are
included. The source workspace's `.wtf/.gitignore` is copied, or `*` is created
when absent. Explicit legacy Lua entrypoints outside `hooks/` are copied too.

Source configuration wins over matching files checked out from the chosen JJ
revision. Existing-workspace preparation never transfers configuration or
overwrites customizations. `.wtf/local/`, identity markers, runtime state, and
other top-level `.wtf` artifacts are excluded; use explicit hooks for those.
Symlinks inside hook/task trees retain their target spelling, without following
their targets. Structural configuration entries must be real files/directories.
Copying is bounded to 8 MiB per file, 32 MiB total, and 1024 entries. Copy failures
stop preparation and retain the failed workspace for inspection; file writes are
atomic, but the complete configuration transfer is not a single transaction.

| Flag | Short | Default | Meaning |
|---|---|---|---|
| `--from` | `-f` | `trunk()` | JJ revision for creation; unused for existing workspaces |
| `--message` | `-m` | unset | Workspace description; an empty string clears it |
| `--json` | `-j` | `false` | Emit one versioned JSON result |

```sh
# Prepare the current workspace
wtf new .

# Create a sibling, based on the current revision
wtf new AgentCase -f @

# Choose an explicit local path
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
