# Files, commands and environment

Import the root package or a namespace:

```lua
local wtf = require("wtf")
local fs = require("wtf.fs")
local exec = require("wtf.exec")
local env = require("wtf.env")
```

## Files

Paths are relative to the active workspace, or to the project before a workspace
exists. Set `root = "project"` or `root = "workspace"` explicitly when needed.
Copy and symlink use `from` and `to` to select source/destination roots separately.
Absolute paths and escaping `..` paths are rejected. VCS metadata, workspace
identity markers, root deletion, and recursive deletion of managed workspaces are
protected. Use the workspace removal command for workspace lifecycle changes.

```lua
fs.mkdir(".wtf/local/example")
fs.write(".wtf/local/example/message.txt", "hello world\n")
local text = fs.read(".wtf/local/example/message.txt")

fs.copy("settings.json", ".wtf/local/example/settings.json", {
  from = "project",
  to = "workspace",
})
fs.symlink("settings.json", ".wtf/local/example/settings-link", {
  from = "project",
  to = "workspace",
})
```

| API | Result / options |
|---|---|
| `exists(path, options)` | Boolean; includes broken links by default. `follow = true` checks their targets. |
| `stat(path, options)` | `name`, `type`, `size`, `mode`, optional raw link `target`; nil when missing. `follow = true` inspects the target. |
| `read(path, options)` | File contents as a Lua string, including binary bytes. |
| `write(path, contents, options)` | Outcome; `replace = true` explicitly replaces an existing file or link. Creates missing parents. |
| `copy(source, destination, options)` | Outcome; `from`, `to`, `replace`, `recursive`, `symlinks`. |
| `symlink(source, destination, options)` | Outcome; `from`, `to`, `replace`. Source may be missing if its parent exists. |
| `readlink(path, options)` | Raw stored target string, including for broken links. |
| `list(directory, options)` | Name-sorted array of records with `name`, `type`, `size`, `mode`. |
| `mkdir(path, options)` | Creates parents; outcome is `created` or `reused`. |
| `remove(path, options)` | Outcome is `removed` or `missing`. Nonempty directories need `recursive = true`. |
| `path(path, options)` | Absolute spelling under the selected root, useful as an explicit command argument. |

Every options table accepts `root`; unknown or incorrectly typed options fail.
Outcomes contain a relative `path` and a `status`: `written`, `copied`, `linked`,
`reused`, `preserved`, `created`, `removed`, or `missing` as appropriate.

Writes and copies preserve existing destinations by default. Replacement swaps
the destination entry, never writes through a destination link. An identical
symlink request is reused. Removing a link removes only the link, including links
to directories. Copying a link requires `symlinks = "preserve"` (copy its raw
target) or `"follow"` (copy target contents). Directory copy also requires
`recursive = true`; cycles and copies into the source tree are rejected.
Nonempty existing destination directories are preserved or rejected, never
recursively erased to make replacement succeed.

Reads/following links are limited to the project's and current workspace's roots.
Cross-workspace links between those roots are supported. Links into other host
directories cannot be read or followed through this API. Preserved links retain
their original target spelling; their targets may be missing or inaccessible.

Limits: 8 MiB per read/write/copied file, 32 MiB per directory copy, and 1024
entries per listing or copy. Failed writes/copies do not publish partial contents;
private staging entries are removed. Cancellation is checked before committing.
Atomic directory copy currently requires Linux, matching this pilot's platform.

## Commands

```lua
local result = exec.run({ "sh", "-c", 'printf "%s" "$GREETING"' }, {
  cwd = ".wtf/local/example",
  root = "workspace",
  env = { GREETING = "hello world", OPTIONAL = false },
  inherit_env = false,
  stdin = "",
  timeout_seconds = 5,
})
wtf.notify(result.stdout, "success")
```

Arguments are passed as argv without shell interpolation. Using `sh -c` is an
explicit project choice; pass dynamic values in argv or environment maps rather
than composing shell strings. `cwd` is a directory relative to the selected root
and defaults to that root. This bounds cwd selection; subprocesses themselves run
project-owned tools with the caller's permissions, not in a filesystem sandbox.

Results contain `ok`, `started`, `code`, `stdout`, `stderr`, `stdout_truncated`,
`stderr_truncated`, `timed_out`, and `cancelled`. Each output stream is capped at
1 MiB; additional bytes are drained and marked truncated. Stdin is capped at
8 MiB. Output is returned to Lua rather than printed or saved in host records.

Nonzero exits and timeouts raise typed errors by default. Use `check = false` to
inspect their results. Start/validation failures and caller cancellation still
raise errors. `exec.check(argv, options)` returns a boolean for process success,
including false for nonzero exits/timeouts; invalid options and start failures
remain errors. Timeout is an integer from 1 to 3600 seconds, defaulting to 120.
On Linux cancellation kills the command's process group; processes are not left
as an implicit background service.

`exec.stage(name, argv, options)` remains the recorded/reusable variant. It now
also accepts `cwd`, environment unsets, and `inherit_env`. Its cwd and declared
input paths are workspace-relative. Stage inputs remain relative to the workspace
root, even when the command runs in a component subdirectory. Live reuse checks
use the same cwd and environment. Changes to cwd, explicit environment settings,
unsets, or inheritance policy invalidate reuse. Stage output goes to hook logs;
environment values and captured command output are not stored in records.

## Environment

```lua
local label = env.get("WTF_HELLO_LABEL", "Lua sandbox")
local selected = env.select({ "WTF_HELLO_LABEL" })
local child = env.merge(selected, {
  GREETING = "hello world",
  OPTIONAL = false,
})
```

`get(name, default)` returns a string, the optional default, or nil. `has(name)`
distinguishes missing from an existing empty string. `require(name)` raises
`environment_missing` when absent. `select(names)` reads only explicitly chosen
variables. `merge(tables...)` creates a new map; later tables win without changing
their inputs. Names and values preserve case.

Command settings accept string values and `false` to unset a variable. Empty
strings remain empty strings. Inheritance defaults to true; `inherit_env = false`
starts from an empty child environment. None of these operations changes the
caller's environment or writes an env file. Values containing NUL, empty names,
and names containing equals signs or NUL are rejected. Executable lookup uses the
invocation's PATH; environment overrides apply to the child process.

## Repeatable hello-world examples

The local `.wtf/config.toml` selects `hello_world_new` after creation and
`hello_world_list` after listing. Both import `lib.hello_world`. They create
fixed sandbox files under `.wtf/local/hello-world/{new,list}`, run a small argv
command with an isolated environment, capture stdout/stderr, update a greeting
and report atomically, preserve an editable independent copy, maintain two links,
inspect them, and create/remove a temporary probe.

```sh
wtf new ../hello-demo -f @
wtf list --pr=false --checks=false
wtf hook run hello_world_list
```

Repeat listing or manual hook invocation freely. For another creation test use a
new destination. Set `WTF_HELLO_LABEL` to personalize the child-process greeting.
Files stay local and ignored; existing `kept-copy.txt` edits survive repeat runs.
No dependencies, services, remote calls, or global environment changes are needed.
