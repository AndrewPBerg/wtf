# WTF workspace and Lua guide for agents

This is a self-contained reference for configuring and using WTF 1.x in this
project. Read this file before changing `.wtf/` or preparing a separate workspace.
The configuration and JSON envelope formats are version `1`; those numbers are
independent of the executable release. Check `wtf version` and command help if the
installed binary differs from this guide. The generated `.wtf/schema.json` describes
that binary's command flags and configuration. After updating the executable,
rerun `wtf setup` to refresh this guide when its generated contents are unchanged.
Locally edited guides are preserved and can describe an older installation.

Project and user instructions still apply. This guide explains WTF capabilities;
it does not authorize deleting work, contacting people, deploying, or changing
shared services. Read the selected workspace's configuration before invoking
execution commands: its hooks are trusted project code and may have side effects.

## Contents

1. Agent workflow and workspace isolation
2. Files, identity, setup, and inheritance
3. CLI and JSON reference
4. TOML configuration and defaults
5. Declarative lifecycle hooks
6. Advanced Lua registration and overrides
7. Lua runtime and event contract
8. Filesystem API
9. Command execution and environment API
10. Tasks and reusable stages
11. Resource and port claims
12. Preparation and teardown recipes
13. Recovery, diagnostics, and verification
14. Unsupported features and common mistakes

## 1. Agent workflow and workspace isolation

WTF manages JJ workspace lifecycle and project preparation. JJ remains the
source of truth for revisions, bookmarks, rebases, fetches, and pushes. Creating a
workspace does not create a bookmark or PR. Editor Git metadata is not delivery
proof. Run commands from the intended workspace and preserve other agents' edits.

For a registered source repository, a typical isolated workflow is:

```sh
wtf version
wtf doctor --here -j
wtf hook list -j
wtf new AgentCase -f @ -m "Investigate the assigned issue" -j
cd ../AgentCase
wtf doctor --here -j
```

The `cd` example assumes the source is the main repository directory. Always use
the returned workspace path if that assumption does not hold. `@` selects the
invoking workspace's current revision; choose an explicit JJ revision when the
assignment requires a particular base. The default for a missing destination is
`trunk()`, so do not omit `-f` when the base matters.

Before doing work:

- Inspect the task scope, current JJ status, and `.wtf/config.toml`.
- Use one workspace per independent assignment; do not share a mutable checkout.
- Confirm preparation succeeded. A created directory alone is not proof of it.
- Keep application ports, Compose project names, temporary paths, and service
  ownership distinct where the project requires isolation.
- Run focused checks with one build/test worker by default. Run one expensive
  build or validation command at a time. Do not repeat passing checks without a
  change or unresolved concern that justifies it.
- For Python projects, prefer the project's existing environment and `uv run`;
  use `uv run --no-sync` when synchronized. Do not invent per-task caches or venvs.

When delivering work, distinguish focused local tests, full local suites, hosted
CI, real service/browser/device tests, and deployed behavior. If pushing a PR,
verify its remote head and current base, then watch applicable hosted CI through
completion unless the user explicitly requests otherwise. Fix in-scope failures
without weakening tests or bypassing checks. Preserve workspaces needed for
review or recovery; use WTF's removal safeguards rather than deleting them by hand.

## 2. Files, identity, setup, and inheritance

Run `wtf setup` at the intended project/workspace root. It needs no repository
registration, creates missing scaffold entries, and never overwrites existing
regular files. It runs no project callbacks and installs no dependencies.

The scaffold contains:

```text
.wtf/
  .gitignore            initially: *
  AGENTS.md             this self-contained agent guide
  .agents-template.sha256  checksum of the last installed guide template
  schema.json           generated editor schema
  config.toml           initially: version = 1, plus schema directive
  hooks/                initially empty; explicitly selected Lua modules
  tasks/                initially empty; project-owned support files
```

`.wtf/.gitignore` ignores everything inside `.wtf/`, including this guide; it does
not untrack files already committed. Setup does not change root ignore rules.
Point agents at `.wtf/AGENTS.md` explicitly from your project instructions when
they need the reference from outside this directory. Agent tools differ in how
they discover nested instructions; WTF does not arrange automatic agent loading.

`wtf setup` preserves existing configuration, schema, and guide edits. Rerunning
fills in missing entries and refreshes an unchanged generated guide using the
installed binary's embedded template. It does not upgrade preserved schema/config.
Conflicting directories or symlinks at file paths fail. A partial setup can leave
completed entries; fix the conflict and rerun. Setup's JSON `created`, `existing`,
`updated`, and `update_available` arrays contain paths relative to the invocation
directory. Update availability is informational, not command failure.

### Updating this reference

Update the WTF executable, then run `wtf setup` in each existing workspace where
you want the new reference. No network lookup occurs during guide refresh; the
installed binary supplies the template, which can be older than another binary.

Setup records SHA-256 of the template in `.wtf/.agents-template.sha256`:

- Missing guide: create the installed template and record its checksum.
- Guide already matches the installed template: preserve it; create/repair a
  missing or older valid checksum record to identify that template.
- Guide matches the last installed checksum but differs from the new template:
  atomically replace it, then update the checksum record.
- Guide differs from both, or differs from the template with no checksum record:
  preserve it and report `Update available` / `data.update_available`.

The checksum tracks a shipped template, never your customized file. Do not set it
to a local edit's hash: doing that would tell setup the edit is safe to overwrite.
Keep project-specific policy in your project's own root AGENTS.md and point agents
here for framework reference, so this generated guide can refresh without conflicts.
To intentionally replace a customized guide, save its edits elsewhere, remove
only `.wtf/AGENTS.md`, and rerun setup. Review the resulting guide before using it.

An invalid checksum record fails with `invalid_setup_state` without modifying the
guide. Restore the record or remove it; an unmatched guide then remains untracked
and preserved. A guide without provenance is adopted only when it exactly matches
the installed template. Guide/checksum updates are separate atomic writes: if a
run stops after publishing the guide, a retry with the same binary heals its record.
Concurrent setup refreshes serialize through `.wtf/.setup.lock`. The lock is local
runtime metadata and is not copied to new workspaces.

`wtf new .` registers/prepares the current JJ workspace. For a missing destination,
`wtf new PATH` creates a native workspace and prepares it. An existing ordinary
directory or file is rejected; an existing JJ workspace root is prepared in place.
A directory nested inside a workspace is not a workspace root and is rejected.
Existing native workspaces are adopted: registration does not grant deletion rights.

Bare names create siblings of the registered main repository, even when invoked
from a child workspace or subdirectory. Explicit relative paths (`./x`, `../x`, or
paths containing `/`) resolve from the invoking directory. Absolute paths work.
The destination basename supplies a native name: 1–80 letters, digits, dots,
underscores, or dashes, starting with a letter or digit. It must be repository-unique.
Paths and names preserve case. Parent paths can contain spaces.

Creation copies the invoking workspace's `.wtf/AGENTS.md` and template checksum,
config, schema, ignore rules,
hooks tree, and tasks tree before preparation callbacks. An explicitly selected
legacy Lua entrypoint outside `hooks/` is also copied. These are independent copies;
child edits do not modify the parent. Source configuration wins over matching
files checked out from the selected JJ revision. Existing-workspace preparation
never transfers configuration or overwrites customizations.

Unselected top-level `.wtf` files, `.wtf/local/`, runtime artifacts, and identity
markers are not inherited. Hook/task trees include nested and empty directories;
links inside them preserve their raw target spelling. Source structural files and
directories, including the guide, must not be symlinks. Transfers are bounded to
8 MiB per file, 32 MiB total, and 1024 entries. Individual files publish atomically;
the whole transfer is not one transaction. Failures retain the workspace for retry.

Repository-wide state lives under the shared JJ repository's
`.jj/repo/wtf-v1-pilot/`. The global repository index is normally `~/.wtf-v1/`;
`WTF_HOME` selects another index directory. The current identity marker is
`.jj/wtf-workspace.json`; older `.wtf/workspace.json` markers remain readable.
Never copy or edit these markers or registry state to fabricate an identity.

`wtf setup teardown` removes the entire current `.wtf/` after terminal confirmation
(default No). It removes this guide too, runs no hooks, and does not unregister
the workspace. Invalid local configuration does not block teardown.
JSON/redirected output cannot confirm deletion; a missing `.wtf/` succeeds. It does
not follow a symlinked `.wtf/`. This command is configuration deletion, not service
shutdown or workspace removal.

## 3. CLI and JSON reference

Every command, group, and alias has help and supports `-j`/`--json`, including help
and argument errors. Command names, aliases, long/short flags, and boolean literals
ignore case. Opaque values preserve case: workspace/task/hook names, paths,
revisions, environment strings, messages, and forwarded child arguments.

| Command | Purpose / important options |
| --- | --- |
| `wtf` | Root help; no implicit preparation |
| `wtf new PATH` | Create/prepare; `-f`/`--from`, `-m`/`--message`, `-- HOOK_ARGS...` |
| `wtf list`, `ls`, `sw` | Browse/list/select; optional NAME, `--global`, `--here`, `--all`, `--select`, `--refresh` |
| `wtf change [NAME] -m TEXT` | Set a description; absent message requests no mutation |
| `wtf remove`, `rm` | Terminal removal picker; optional NAME for direct removal |
| `wtf setup` | Create missing scaffold; refresh unchanged generated agent guide |
| `wtf setup teardown` | Confirm deletion of local `.wtf/` |
| `wtf hook`, `wtf hook list` | Inspect registered hooks/overrides without calling them |
| `wtf hook run NAME -- ARGS...` | Invoke a registered hook under workspace lock |
| `wtf run TASK -- ARGS...` | Execute an explicitly declared task |
| `wtf ports`, `wtf ports list` | Inspect the current workspace's port claims |
| `wtf ports claim NAME --port PORT` | Claim a chosen loopback TCP port |
| `wtf ports release NAME` | Release the current workspace's named port claim |
| `wtf doctor`, `doc` | Configuration, prerequisites, registration, stage/claim health; `--here` |
| `wtf completion bash`, `zsh`, `fish` | Generate shell completion |
| `wtf version` | Installed release; root `--version`/`-v` also works |

List flags: `-a` = `--all`, `-k` = `--hook` (stage column), `-p` = `--pr`,
`-c` = `--checks`. PR links and checks are enabled by default. Stage column,
global scope, all records, and selection are disabled by default. `--global` and
`--here` cannot be combined. Use `--pr=false --checks=false` to disable metadata
lookups where appropriate. A supplied NAME returns its path; a process cannot
change its parent shell directory without a shell wrapper.

PR links prefer verified Linear reviews, falling back to GitHub. Provider
failures are unavailable observations, not proof that no PR/review exists.
Metadata is bounded and cached; cached CI or review observations are not proof of
current delivery. Doctor can check optional GitHub/Linear readiness and account
identity, but it does not install tools, log in, or establish project permissions.
Do not print credentials or run model turns to perform review lookup.

JSON stdout contains exactly one result, for example:

```json
{"version":1,"command":"wtf version","ok":true,"data":{"version":"1.0.0"}}
```

The envelope `version` is the contract version. `command` uses canonical spelling.
Failures have `ok: false`, an `error` with `code` and `message`, and nonzero exit
status. Partial lifecycle `data` can still be useful. Lua logs, notifications, and
child output go to stderr; JSON and redirected logs have no terminal styling.
Do not parse human tables or treat a `data` field alone as success. Check both
exit status and `ok`. JSON never opens interactive prompts.

Bare `remove -j` or redirected removal previews candidates without deleting.
Terminal list/removal pickers use arrows or j/k, counts, gg/G, and h/l/page keys.
Removal uses Space to select, `f` for force mode, `a` for clean verified work, then
Enter to review and Enter to confirm. Esc goes back/cancels and Ctrl+C cancels.
Force permits discarding changes but does not bypass current/adopted-workspace,
identity, inspection, resource, or project lifecycle protections.

### Command recipes and exact boundaries

`new` accepts one destination, optional `--from`/`-f` (default `trunk()`),
`--message`/`-m`, JSON, and project hook args after `--`. An existing root ignores
the creation revision. A repeat call preserves the message unless a new message
is supplied. Empty message clears it. Native initialization is internal; there
is no separate public init command. New-source registration is required before
creating another workspace, so prepare the source with `wtf new .` first.

```sh
wtf new .
wtf new AgentCase -f @ -m "Scoped experiment" -- --test
wtf new ./local-experiment -f @ -j
wtf new /absolute/path/to/existing-workspace -j
```

`list`/`ls`/`sw` share behavior. Inside a JJ repository their normal scope is
that repository; from a parent directory, registered repositories beneath it.
`--global` covers all registered repositories. `--here` restricts to the current
managed workspace. `--all` includes unavailable/removed records. A named lookup
returns path/ID and fails if the workspace is unavailable. `--select` makes the
selected path suitable for shell navigation; redirected human selection without
a NAME requires a terminal. `--refresh` requires PR or checks to be enabled.
The hook column reports local stages, while checks report published PR-head CI.

```sh
wtf ls --here --pr=false --checks=false -j
wtf ls --global --all --pr=false --checks=false -j
wtf list AgentCase --select
```

`change [NAME]` targets the selected workspace, or the current one when NAME is
omitted. `-m` replaces the description, including empty-string clearing. Without
a requested message mutation it does not invoke callbacks. Messages are metadata,
not JJ commit descriptions, bookmarks, or PR bodies. Terminal rendering sanitizes
controls and may truncate display; JSON preserves the message's original text.

```sh
wtf change -m "Ready for review" -j
wtf change AgentCase -m "" -j
```

`remove NAME`/`rm NAME` performs explicit-target cleanup with safety checks.
An untargeted terminal call opens the multi-select picker. Untargeted JSON or
redirected output reports candidate assessments and never deletes. The current
workspace and adopted native workspaces are protected. Remaining changes outside
main/master prevent direct removal. Force is a picker mode, not a general
permission-bypass flag. Remote bookmarks use last-fetched state; inspect/fetch
through JJ when freshness matters. Each confirmed deletion rechecks identity,
claims, and source changes; removal hooks can affect those checks. Multi-removal
can partially succeed, so inspect every result rather than assuming all-or-nothing.

```sh
wtf remove -j
wtf remove AgentCase -j
```

`setup` needs no JJ repository and operates on the invocation directory, so run
it at the intended root. Both hook/task directories start empty. It generates
this reference without executing examples and refreshes an unchanged generated
copy on reruns. Customized guides remain preserved with an available-update report.
`setup teardown` deletes all local
scaffold content only after terminal confirmation; no JSON confirmation flag is
provided. It does not stop application services or clean up claims.

`hook` defaults to inspection, as does `hook list`. Registration code is loaded
but callback bodies are not called. `hook run NAME` invokes an explicitly
registered hook, forwards only project args after `--`, locks the workspace,
records its stages, and stops on error. `run TASK` executes TOML task argv with
literal appended arguments. Arbitrary Lua files are not command entrypoints.

```sh
wtf hook list -j
wtf hook run prepare -j -- --test ValueCase
wtf run test -j -- -run TestExample
```

`ports` defaults to the current workspace's port-claim list. `ports list` filters
recorded claims by port kind and owner; it does not enumerate every repository
owner or check socket availability. `ports claim NAME --port N` requires a managed
workspace and integer port 1–65535. `ports release NAME` releases that owner's key.
It does not stop a process. The registry is shared within a repository, even though
the CLI listing is owner-scoped. Section 11 explains conflict/retry semantics.

```sh
wtf ports list -j
wtf ports claim web --port 3000 -j
wtf ports release web -j
```

`doctor`/`doc` accepts `--here` and JSON. It observes tool/config/workspace health
without repairing it or executing hooks. Required JJ checks include effective
user.name and user.email; optional provider checks may contact GitHub/Linear.
JJ/GitHub subprocess probes have five-second bounds and the Linear probe has an
eight-second overall bound. An unregistered healthy workspace is informational.
Malformed configuration or unhealthy required prerequisites fail. Sections 4
and 13 explain syntax-check and readiness limits.

`completion` requires a supported shell subcommand: bash, zsh, or fish. Inspect
the generated output and follow project shell-installation policy; generation
alone does not install a script or enable parent-shell directory changes. `version`
and root `--version`/`-v` report the installed release. JSON help is useful for
automation and available for all command groups and aliases too.

```sh
wtf completion bash
wtf completion zsh
wtf completion fish
wtf version -j
wtf --version -j
wtf SW --help -j
wtf ports claim --help -j
```

## 4. TOML configuration and defaults

Lifecycle/task config loads from the selected JJ workspace root's
`.wtf/config.toml`. Unknown fields and invalid types fail validation. Start with:

```toml
#:schema ./schema.json
version = 1

[commands.list]
pr = false
checks = false
hook = true

[new]
after = ["prepare"]

[tasks.test]
command = ["go", "test", "-p", "1", "./internal/..."]
timeout_seconds = 120
```

The task above is a Go-project example; use this project's actual focused command.
The format version is mandatory when a file exists. An absent config behaves as
empty configuration. Command defaults and lifecycle registrations are different:

- Command defaults load from `$WTF_HOME/config.toml`, then `.wtf/config.toml` files
  from the filesystem root down to the invocation directory.
- Per flag, precedence is explicit CLI > leaf > nearest parent > outer parents >
  global > factory default. Explicit false and empty values remain meaningful.
- Command/flag keys ignore case and aliases normalize to their canonical names.
  Use the generated schema for valid keys/types; unknown entries fail.
- Hook phase arrays, task definitions, and Lua entrypoints belong to the selected
  workspace. Their arrays do not merge through command-default inheritance.
- Configure declarative phases OR a Lua registration entrypoint. Do not combine
  `lua = "..."` with nonempty new/list/remove/invocation hook arrays. Tasks and
  command defaults may coexist with either registration mode.

`wtf doctor` checks configuration without invoking callbacks. It compiles selected
Lua syntax without evaluating project registration or importing every dependency;
it is not proof that a callback or imported module works. `wtf hook list` loads
registration for inspection, so module declarations must be side-effect free.

## 5. Declarative lifecycle hooks

Select ordered modules under `.wtf/hooks/`:

```toml
version = 1

[new]
before = ["preflight"]
after = ["prepare"]
failed = ["report_failure"]
finally = ["finish"]

[remove]
before = ["stop_services"]
```

Each selected module returns one callback; filenames alone never register it.
For `.wtf/hooks/preflight.lua`:

```lua
local wtf = require("wtf")
return function(event)
  wtf.policy.require(wtf.fs.exists("go.mod"), "This example requires go.mod")
end
```

Put host operations inside the callback, not at module scope. Imports and function
construction at module scope are fine. Unlisted files do not run automatically.
Nested module names use package imports such as `require("lib.helpers")`.

`new`, `list`, `remove`, and `invocation` support `before`, `after`, `failed`,
and `finally`. `[new].hooks` is the compatible post-creation shorthand; use
explicit phases for new declarations. On other surfaces, `hooks` runs before
explicit `after`. Arrays preserve declaration order.

Execution order:

1. `invocation.before`
2. Surface `before`
3. Operation (including native preparation and compatible hooks)
4. Surface `after`
5. `invocation.after`
6. On failure: surface `failed`, then `invocation.failed`
7. On both outcomes: surface `finally`, then `invocation.finally`

Before/after sequences stop on their first error. A before error blocks mutation;
an after error fails the command but does not undo the completed operation. All
failed/finally modules are attempted. Cleanup errors add diagnostics without
replacing the primary error. Finally failure fails the command but does not
recursively run failed hooks. Each cleanup phase has a fresh five-second deadline
independent of caller cancellation. Normal callback execution has a ten-minute bound.

New phases apply only when creating a missing destination. `wtf new .` and other
existing-workspace preparation do not run `[new]` phases. Invocation phases still
wrap execution. Use an `init` override when preparation must apply to both modes.
Aliases dispatch the canonical surface once. Bulk removal has one command-level
phase sequence with aggregate results; design cleanup for that scope explicitly.

Help, bare group help, version, doctor, hook listing, setup teardown, and untargeted
JSON/redirected removal previews never invoke callbacks. Invalid arguments/config
fail before callbacks. Other execution commands can be wrapped by invocation
hooks; keep those hooks narrow, since they can wrap tasks and port commands too.

## 6. Advanced Lua registration and overrides

Use this mode for shared preparation, named composition, or operation overrides:

```toml
version = 1
lua = "hooks/init.lua"
```

```lua
-- .wtf/hooks/init.lua
local wtf = require("wtf")
return wtf.setup {
  hooks = {
    prepare = require("prepare"),
  },
  overrides = {
    init = function(event)
      local workspace = event.default()
      wtf.hooks.run("prepare")
      return workspace
    end,
  },
}
```

```lua
-- .wtf/hooks/prepare.lua
local wtf = require("wtf")
return function(event)
  wtf.fs.mkdir(".wtf/local")
  wtf.fs.write(".wtf/local/prepared.txt", "ready\n", { replace = true })
end
```

`wtf.setup` returns a registration table; calling it without returning the table
from the selected entrypoint does not register callbacks. It exposes the `hooks`
and `overrides` maps. Supported operation overrides are `new`, `init`, and `remove`.
Ordinary hooks have no default delegate. Delegate lifecycle operations exactly
once via `event.default()`, and return the resulting workspace.

A missing destination dispatches the source `new` override before native creation,
then initializes using the created workspace's configuration/identity. `init` also
runs for an existing root. Before delegation, a new override has no workspace;
a first-time init can also have no registered workspace. After delegation,
`event.workspace` and host APIs reflect the live bound workspace.

A new override can change allowed request name/revision fields before delegation,
but cannot redirect the fixed destination path. Replacement init/remove requests
cannot change workspace identity. Do not call default twice, fabricate a workspace
record, or implement core creation/removal yourself. A pre-delegation policy error
can block mutation. Once creation has happened, later failures retain the workspace.

`wtf.hooks.run(name, request)` synchronously invokes a registered named hook;
omitting request forwards the current request. `wtf.hooks.has(name)` checks it.
Composition stops on failure. Use host composition rather than launching another
`wtf hook run`/`wtf new` process inside a locked callback: reentry can wait on the
same workspace lock. Imported function helpers can also be called directly.

Legacy raw registration tables with `(ctx, request, default)` callbacks remain
supported. Their context methods use colon syntax, for example
`ctx:claim_port("web", 3000)` and `ctx:stage(...)`. Prefer the `wtf` package and
one-event callbacks for new code. Raw tables and `wtf.setup` registrations have
different callback signatures; do not mix the assumptions.

## 7. Lua runtime and event contract

The host embeds GopherLua with base, table, string, and math facilities plus a
bounded project module loader. There is no `io`, `os`, native/system module loader,
`dofile`, `loadfile`, or `collectgarbage`. Use host filesystem/exec/environment
helpers. `require` is relative to the selected package directory (normally
`.wtf/hooks`), caches modules within one invocation, and rejects escape paths and
cyclic imports. Separate CLI invocations do not share Lua globals or module state.
The `wtf` namespace belongs to the host and cannot be shadowed by project files.

Import `require("wtf")` or the supported `wtf.fs`, `wtf.exec`, `wtf.env`,
`wtf.policy`, `wtf.resources`, and `wtf.hooks` namespaces. Package functions use
DOT calls, such as `wtf.fs.read(path)`. `wtf.ctx` exposes legacy COLON methods.
`wtf.api_version` is currently `1`.

Event fields:

| Field | Meaning |
| --- | --- |
| `ctx` | Live host context; identical to `wtf.ctx` |
| `request` | Current request table |
| `name`, `from`, `path`, `args` | Request fields; path can be absent when not applicable |
| `project` | Project context with path and repository ID when available |
| `workspace` | Live workspace context or nil before registration/creation |
| `command`, `surface`, `trigger` | Canonical command and declarative phase metadata when applicable |
| `result` | Snapshot of operation JSON data; may be nil; not a mutable replacement |
| `error` | Failure code/message when applicable |
| `default` | Lifecycle delegate, present only for overrides |

Workspace fields include `id`, `name`, `path`, `status`, and `message`. Request
arguments after `--` arrive unchanged; the project interprets them. Field presence
depends on the callback's operation/phase. Check nil explicitly, especially before
creation, in failed phases, on picker cancellation, and for aggregate removal.

```lua
local wtf = require("wtf")
return function(event)
  local selected = false
  for _, arg in ipairs(event.args or {}) do
    if arg == "--test" then selected = true end
  end
  if selected then wtf.hooks.run("test") end
end
```

Use `wtf.policy.require(condition, message)` or `wtf.policy.block(message)` to fail.
Host failures raised to Lua expose typed `code` and `message` through `pcall`:

```lua
local wtf = require("wtf")
local ok, err = pcall(function()
  wtf.env.require("PROJECT_REQUIRED_SETTING")
end)
if not ok then
  -- This operation raises host errors; other Lua errors can be strings.
  if type(err) == "userdata" and err.code == "environment_missing" then
    wtf.policy.block("Provide PROJECT_REQUIRED_SETTING")
  end
  error(err)
end
```

Do not swallow cancellation, environment, path, or registry failures as success.
`claim_port` has a distinct `nil, code` failure convention described below.

`wtf.notify(message, level)` supports info (default), success, warning, and error.
Level controls appearance, not success/failure. Notifications and Lua `print` use
logs/stderr. Terminal colors follow the active palette; controls are sanitized;
JSON/redirected output is unstyled and NO_COLOR is respected.

## 8. Filesystem API

Default paths are relative to the active workspace, or the project when no
workspace exists. Options accept `root = "project"` or `root = "workspace"`.
Copy/link select source and destination with `from` and `to` independently.
Unknown options or invalid types fail. Absolute paths and escaping `..` are rejected.

| Function | Behavior / options |
| --- | --- |
| `fs.exists(path, opts)` | Boolean; broken links count by default; `follow = true` tests target |
| `fs.stat(path, opts)` | Record: name/type/size/mode, optional raw target; nil if missing; `follow` |
| `fs.read(path, opts)` | String contents, including binary bytes |
| `fs.write(path, contents, opts)` | Creates parents; preserves existing file unless `replace = true` |
| `fs.copy(src, dst, opts)` | `from`, `to`, `replace`, `recursive`, `symlinks` |
| `fs.symlink(src, dst, opts)` | `from`, `to`, `replace`; source may be missing if parent exists |
| `fs.readlink(path, opts)` | Stored target spelling, including broken links |
| `fs.list(dir, opts)` | Name-sorted records with name/type/size/mode |
| `fs.mkdir(path, opts)` | Creates parents; created/reused outcome |
| `fs.remove(path, opts)` | Removed/missing outcome; nonempty directories need `recursive = true` |
| `fs.path(path, opts)` | Absolute spelling inside selected root for explicit command arguments |

Mutation outcomes include relative `path` and `status`, such as written, copied,
linked, reused, preserved, created, removed, or missing. Check status when replacement
matters. Default writes/copies preserve existing destinations; requesting a write
is not proof that an existing file changed. Replacement swaps an entry, never writes
through its destination symlink. An identical symlink request reuses the link.

```lua
local fs = require("wtf.fs")
fs.mkdir(".wtf/local/example")
fs.write(".wtf/local/example/message.txt", "ready\n", { replace = true })
local message = fs.read(".wtf/local/example/message.txt")
fs.copy("settings.json", ".wtf/local/example/settings.json", {
  from = "project", to = "workspace",
})
```

For a directory copy, set `recursive = true`. Copying a link requires
`symlinks = "preserve"` (raw target) or `"follow"` (target contents). Cycles and
copies into the source tree fail. Nonempty destination directories are preserved
or rejected, never recursively erased to make replacement work. Removing a link
removes only the link, including directory links.

Reads/following links stay within the project and current workspace roots.
Cross-links between those roots are supported; external host links cannot be
followed. Preserved links may intentionally have inaccessible or missing targets.
VCS metadata, identity markers, root deletion, and recursive deletion of managed
workspaces are protected. Use WTF lifecycle commands for workspace removal.

Limits: 8 MiB per read/write/copied file, 32 MiB per directory copy, 1024 entries
per listing/copy. Failed writes/copies do not publish partial files. Directory
copy publication requires Linux. Host path guards are not a sandbox for commands;
a subprocess still has the user's ordinary filesystem permissions.

## 9. Command execution and environment API

```lua
local exec = require("wtf.exec")
local result = exec.run({ "sh", "-c", 'printf "%s" "$GREETING"' }, {
  cwd = ".",
  root = "workspace",
  env = { GREETING = "hello world", OPTIONAL = false },
  inherit_env = false,
  stdin = "",
  timeout_seconds = 5,
})
assert(result.ok and result.stdout == "hello world")
```

Commands receive argv directly. Shell expansion happens only if you explicitly
choose a shell, such as `sh -c`. Pass dynamic values in argv or environment maps;
do not compose shell source from untrusted text. `cwd` is a relative directory
inside the selected root; it defaults to that root. Command helpers have caller
permissions and are not a filesystem/network sandbox.

`exec.run(argv, options)` returns `ok`, `started`, `code`, `stdout`, `stderr`,
`stdout_truncated`, `stderr_truncated`, `timed_out`, and `cancelled`. Captured streams
are limited to 1 MiB each; excess bytes are drained and flagged. Stdin is capped at
8 MiB. Captured output is returned to Lua, not automatically printed or recorded.

Nonzero exits/timeouts raise by default. `check = false` lets you inspect those
results. Invalid settings, start failures, and caller cancellation still raise.
`exec.check(argv, options)` returns false for nonzero exits/timeouts; invalid
settings/start failures still raise. Timeout is an integer 1–3600 seconds, default
120. On Linux cancellation kills the command process group. Command helpers also
clean up subprocess groups; shell backgrounding is not a service supervision API.

| Environment function | Behavior |
| --- | --- |
| `env.get(name, default)` | String, optional default, or nil |
| `env.has(name)` | Distinguishes missing from present empty string |
| `env.require(name)` | String; raises environment_missing when absent |
| `env.select(names)` | Reads only the selected names into a map |
| `env.merge(tables...)` | New map; later tables win; inputs unchanged |

Command `env` maps accept strings and false (unset). Empty strings remain empty.
Inheritance defaults true; `inherit_env = false` starts from an empty environment.
Names/values preserve case. Empty names, names with equals/NUL, or values with NUL
are rejected. Executable lookup uses invocation PATH; child overrides apply after
lookup. These helpers neither mutate the parent shell environment nor write env
files. If a service needs an env file, create it explicitly and arrange for the
launch script to read it. Never print secrets or place them in notifications.

## 10. Tasks and reusable stages

Tasks must be explicitly declared; placing files in `.wtf/tasks/` does not make
executable tasks. A Go project example:

```toml
version = 1
[tasks.test]
command = ["go", "test", "-p", "1", "./internal/..."]
inputs = ["go.mod", "go.sum", "internal"]
timeout_seconds = 120
```

```sh
wtf run test -j
wtf run test -- -run TestExample
```

Tasks run in the current registered workspace under its execution lock. Extra
arguments append literally to argv. Inputs record the stage fingerprint, but tasks
rerun by default. Omitted/zero task timeout is 120 seconds; max 3600. Task execution
has no TOML shell/cwd/env/reuse options beyond the schema: use Lua for those needs.

Lua `exec.stage(name, argv, options)` records/reuses named work. Options are
`inputs`, `check` (argv), `reuse`, `timeout_seconds`, `env`, `cwd`, and `inherit_env`.
Stage cwd and input paths are workspace-relative, even when cwd selects a component.
A reusable stage needs at least inputs or a live check. Name uses the same bounded
identifier rules as other named resources.

```lua
local exec = require("wtf.exec")
local stage = exec.stage("dependencies", { "go", "mod", "download" }, {
  inputs = { "go.mod", "go.sum" },
  reuse = true,
  timeout_seconds = 120,
})
assert(stage.status == "succeeded")
```

Reuse needs a prior succeeded stage and matching fingerprint. The fingerprint
includes argv, declared inputs' contents, check argv, timeout, cwd, explicit env,
unsets, and inheritance policy. Input paths must exist, stay inside the workspace,
and contain regular files rather than symlinks. VCS metadata directories are skipped.
The full inherited environment is not fingerprinted: pass important changing
settings explicitly or add readiness checks.

If a live check is supplied, it must pass for reuse. A failed readiness check reruns
the main command. After successful execution, the check runs again and must pass.
A failure stops subsequent calls and retains failed stage details. Repeated
preparation can retain completed stages without hiding failed work. Stage results
include status/reused and recorded timing/fingerprint/error fields. Stage logs go
to stderr; captured command output and env values are not stored in stage records.

Readiness must prove what matters to your project. A marker file proves a marker;
an open port proves a listener, not service identity. A reused local stage does
not prove hosted CI or deployment success. Failure persistence uses a bounded
cancellation-independent context.

## 11. Resource and port claims

`resources.list()` returns only the current workspace's claims, as records with
`kind`, `name`, and `value`. It is not a machine-wide enumeration.

```lua
local resources = require("wtf.resources")
resources.claim("compose", "sandbox", "project-specific-unique-value")
resources.release("compose", "sandbox")
local port, code = resources.claim_port("web", 3000)
```

Generic `claim(kind, name, value)` returns the claimed string or raises a typed
failure. Kind/name follow identifier rules; value must be nonempty. Generic
claims coordinate strings, not an external system's actual allocation. Use the
port helper for integer/range validation and canonical loopback spelling.

Port claims are REPOSITORY-wide, not machine-global. All workspaces sharing one
JJ repository share its registry. A file lock and atomic state publication prevent
competing processes from both claiming a kind/value for different owners. Independent
repositories can claim the same port. The global index does not add cross-repository
leases. IPv4 loopback `127.0.0.1:PORT` is the supported port claim value.

Ownership rules:

- A claim key is workspace ID + kind + name.
- Repeating that key with the same value is idempotent.
- Changing its value requires release first.
- Another workspace cannot claim the same kind/value under any name.
- One owner can claim the same value under multiple names; releasing one leaves
  the others intact. Release every applicable name when relinquishing ownership.
- Release affects only the caller's named claim and succeeds if already absent.
- Removed or missing workspace identities cannot claim resources.
- Claims persist after hook/service exit and have no TTL or process-health tracking.

`claim_port(name, port)` accepts integers 1–65535. Success returns the number;
registry errors return nil and an error code (such as claim_conflict). Invalid
arguments raise invalid_port. Retry conflicts only, and propagate other errors.
`resources.listening(port)` attempts a TCP connection to IPv4 loopback with a
200 ms timeout. False means connection failure, not guaranteed bind availability.

A bounded allocator, usable inside a registered workspace callback:

```lua
local wtf = require("wtf")
local resources = wtf.resources
local function choose_web_port()
  for _, claim in ipairs(resources.list()) do
    if claim.kind == "port" and claim.name == "web" then
      local port = tonumber(claim.value:match("^127%.0%.0%.1:(%d+)$"))
      wtf.policy.require(port ~= nil, "Invalid existing web port claim")
      return port
    end
  end
  for candidate = 3000, 3099 do
    if not resources.listening(candidate) then
      local port, code = resources.claim_port("web", candidate)
      if port then return port end
      wtf.policy.require(code == "claim_conflict",
        "Cannot claim web port: " .. tostring(code))
    end
  end
  wtf.policy.block("No available web port in 3000-3099")
end
return function(event)
  local port = choose_web_port()
  wtf.fs.write(".wtf/web.env", "PORT=" .. tostring(port) .. "\n", {
    replace = true,
  })
  wtf.notify("Web port: " .. tostring(port))
end
```

Register that module in `new.after` for creation-only allocation, or call it from
an advanced `init` preparation hook for new AND existing workspaces. A workspace
must exist before claiming; `new.before` has no new destination identity to claim
against. Choose the candidate range as project policy. On preparation retry,
reusing an existing assignment avoids unnecessary churn.

The launch script must read the generated env file and pass PORT to the service.
WTF does not bind a socket, select a port automatically, launch a service, or stop
one on release. An unrelated service can take a port between availability check
and actual startup. On a confirmed bind conflict, project code must release its
assignment, select another candidate, rewrite configuration, and retry with a
bound. An open listener alone does not prove it is your service.

Successful workspace removal clears its port claims. Non-port external claims
must be explicitly released by teardown before removal. Stop owned services first;
release after confirmed shutdown. If shutdown fails, retain claims so another
workspace does not receive misleading availability. A stale claim is cleanup work,
not permission to kill an arbitrary process listening on its port.

Local focused tests cover concurrent owners, separate processes, persistence,
Lua retry/release, and repository scope. These prove registry behavior locally,
not service startup/teardown, a long soak, or machine-global allocation safety.

## 12. Preparation and teardown recipes

Choose one registration mode per workspace. Start with optional, idempotent steps;
keep project-specific service commands and dependencies explicit.

For a shared init preparation hook, the advanced registration in section 6 runs
`prepare` after `event.default()`. Within prepare, inspect `event.args` to select
optional checks. Keep argv/environment construction separate from shell source.
Call named hooks directly through `wtf.hooks.run`, and use named stages for work
whose success should survive retries. Do not mark fast-changing tests reusable
merely to make subsequent preparation look successful.

For a service supervisor, use its actual management API via `exec.run`/`stage`,
with a workspace-specific name or label. Confirm the service identity and health.
WTF supplies command execution and claims; it does not supply a supervisor,
Compose configuration, health endpoint, background process registry, or restart loop.
Do not use `sh -c 'server &'` as a durable launch mechanism.

A remove override delegates only after teardown:

```lua
-- Selected advanced entrypoint; stop_services must be project-owned code.
local wtf = require("wtf")
return wtf.setup {
  hooks = { stop_services = require("stop_services") },
  overrides = {
    remove = function(event)
      wtf.hooks.run("stop_services")
      wtf.resources.release("port", "web")
      wtf.resources.release("compose", "sandbox")
      return event.default()
    end,
  },
}
```

This sketch assumes those are the project's actual claim names. Implement
`stop_services.lua` to check and stop ONLY the supervisor resources owned by this
workspace, tolerate already-stopped services, and raise if shutdown fails. Do not
release first and then attempt shutdown. A teardown error preserves the workspace
and primary failure for recovery. Declarative `remove.before` also supports
explicit-target teardown; account for command-level aggregate behavior in bulk mode.

For failed preparation, release newly acquired claims only when their associated
service is confirmed stopped and the claim should not survive retry. Do not put
unconditional release into finally: finally also runs on success. Failed/finally
cleanup has a short bound and should be selective and safe after partial progress.

## 13. Recovery, diagnostics, and verification

```sh
wtf doctor --here -j
wtf hook list -j
wtf ports list -j
wtf list --all --pr=false --checks=false -j
```

Doctor checks configuration, JJ executable and identity, optional gh/Linear
readiness, registration, stages, claims, and recorded errors. Required JJ failures
fail health; missing optional metadata tools do not fail an otherwise healthy
workspace. Doctor runs no hooks, installs nothing, starts no services, repairs
nothing, and does not fetch or snapshot JJ state. Its successful syntax check is
not callback execution proof.

Retry failed preparation with the SAME destination path:

```sh
wtf new ../AgentCase -- --test
```

Registration/identity/ownership stay stable and reusable stages determine what
can be retained. Completed stages remain inspectable. Repeated new without a
message preserves its description; `-m TEXT` updates it and `-m ""` clears it.
First registration/creation reports initialized; repeats report already_initialized.
Do not infer preparation success from that code without checking the command result.

For a stale JJ workspace, inspect JJ's own diagnostics and repair its state through
JJ as appropriate before retrying. WTF does not automatically fetch or update-stale.
Integration checks use available, last-fetched main/master bookmarks and bounded
provider observations. A matching branch name, closed PR, old CI result, or stage
success does not prove a change is integrated or safe to delete.

Listing can reconcile confirmed missing, idle registrations, removing only stale
WTF/JJ/Git administrative metadata. It does not delete source directories. Locks,
claims, broken links, in-flight work, and access errors prevent pruning. Unavailable
records are normally hidden; use `--all` to inspect them. Even listing is not a
promise of zero metadata mutation.

Before accepting a `.wtf/` change:

1. Run doctor for TOML and selected entrypoint syntax.
2. Inspect registrations; verify declarations have no top-level side effects.
3. Execute the relevant callback/task in a disposable isolated workspace.
4. Repeat preparation to check idempotence, file preservation, and stage reuse.
5. Exercise expected failure/cancellation and ensure later steps stop, primary
   errors survive, and retries retain useful completed work.
6. If ports/services are involved, test competing workspaces and actual startup,
   address conflicts, service identity, shutdown, and claim cleanup.
7. Verify the child has the intended inherited config/guide, independent edits,
   and none of the parent's private local artifacts or identity state.
8. Report exactly what ran and what remains untested. Keep checks proportional to
   the change and workstation resources.

Do not run cleanup, remove workspaces, or release another assignment's resources
just to tidy a report. Keep failure evidence available for the owning agent/user.

## 14. Unsupported features and common mistakes

- There is no public `wtf init`, `validate`, `resources`, `port`, `watch`, `update`,
  or `uninstall` command in the 1.0.0 contract. Use new/doctor/ports and current help.
- A task Lua file is not auto-discovered; declare tasks in TOML.
- A hook file is not auto-run; register its module or callback explicitly.
- Declarative new phases do not run for existing-workspace preparation.
- Setup does not register a workspace or run preparation; new does.
- Setup preserves config/schema and custom guide edits; only unchanged generated
  guides refresh automatically.
- Command defaults are not merged hook declarations.
- `.wtf/local` and arbitrary top-level artifacts are not copied into new workspaces.
- A port claim is not a socket reservation or a global lease.
- A listener is not proof of this workspace's service identity or health.
- Release does not stop a service; removal does not supervise background processes.
- Env helpers cannot change the caller shell environment.
- Filesystem writes preserve existing destinations unless replacement is explicit.
- Lua has no system `os`/`io`; use the host APIs and trusted project commands.
- Lua host path checks do not sandbox subprocesses.
- Shell backgrounding is not supported service supervision.
- Help/doctor/inspection success is not execution or delivery proof.
- Cached/reused success is not fresh hosted CI or deployment evidence.
- Do not copy registry/identity state between workspaces or bypass removal guards.

When a needed behavior is absent, implement bounded project policy with the actual
host APIs or propose a scoped framework change. Do not invent commands, options,
module APIs, or machine-global guarantees based on historical designs.
