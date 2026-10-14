# Project hook modules

This page describes WTF 1.0.0. Files, commands, and environment helpers are
available in the [Lua substrate](lua-substrate.md); older roadmap proposals are
historical and do not define supported APIs.

Hooks are explicit project code. TOML selects the modules and their order.
Unlisted files have no effect on their own.

## Trigger phases

The `new`, `list`, `remove`, and `invocation` surfaces all support ordered
`before`, `after`, `failed`, and `finally` arrays. Each entry names a Lua module
under `.wtf/hooks`, returning a single `function(event)` callback.

```toml
[new]
before = ["check_prerequisites"]
after = ["prepare_dependencies"]
failed = ["report_failure"]
finally = ["finish_logging"]

[list]
before = ["begin_listing"]
after = ["inspect_listing"]
failed = ["report_failure"]
finally = ["finish_logging"]

[remove]
before = ["prepare_removal"]
after = ["report_removal"]
failed = ["report_failure"]
finally = ["finish_logging"]

[invocation]
before = ["begin_command"]
after = ["report_success"]
failed = ["report_failure"]
finally = ["finish_logging"]
```

`invocation.before` runs first, followed by the surface's `before`, the operation,
the surface's `after`, and `invocation.after`. Before and after sequences stop at
the first failure. A before failure prevents the operation. An after failure
fails the command but does not undo its completed operation.

On failure, the surface's `failed` and then `invocation.failed` run. The surface's
`finally` and then `invocation.finally` run on both success and failure. Every
module in failed/finally is attempted, even if another cleanup module fails.
Cleanup failures append diagnostics without replacing the primary error. A finally
failure also fails the command; it does not recursively dispatch failed hooks.
Each cleanup phase gets a fresh five-second deadline independent of caller
cancellation. Normal callback execution retains the existing ten-minute bound.

Events add `command` (canonical command path), `surface`, `trigger`, `result`,
and `error` (`code` and `message`). `result` is a Lua snapshot of the operation's
JSON data, not a mutable replacement for the CLI result. For list it contains
the listing or selected path; it can be nil when a picker is cancelled. Remove
phases for an explicit target see that workspace, including its removed status
after deletion. Workspace actions on removed identities fail explicitly. Bulk
removal has one command-level phase sequence and an aggregate result.

New phases apply to missing destinations, not preparation of existing JJ
workspaces. Invocation phases still wrap existing-workspace preparation. The
`hooks` shorthand runs before explicit after arrays; `[new].hooks` retains its
native post-creation behavior. Aliases dispatch the canonical surface once.

Only parsed execution commands enter this pipeline. Help, bare group help,
version, doctor, hook listing, setup teardown, and redirected/JSON removal previews
without a target never invoke callbacks. Invalid arguments/configuration fail
before the pipeline starts. Callback module declarations must be side-effect free.

The focused hello-world test matrix exercises the real CLI:

| Surface | before | after | failed | finally |
|---|---|---|---|---|
| new | Tested | Tested | Tested | Tested |
| list / ls | Tested | Tested | Tested | Tested |
| remove / rm | Tested | Tested | Tested | Tested |
| invocation | Tested | Tested | Tested | Tested |

See `internal/cli/triggers_test.go`: successful and failing commands, ordered
messages, JSON separation, before gates, after-failure retention, error-preserving
cleanup, cancellation, and callback-free inspection. Examples are temporary test
fixtures; setup continues to create empty hooks/tasks directories.

## Creation hooks in TOML

```toml
version = 1

[new]
hooks = ["hello_world"]
```

`.wtf/hooks/hello_world.lua` contains only the callback:

```lua
local wtf = require("wtf")

return function(event)
  wtf.notify("hello world", "success")
end
```

`wtf new ../example` creates and registers the workspace, then runs each listed
hook in array order. No registration entrypoint or default delegation is needed.
The event includes the new workspace, destination path, source project, and
case-preserved arguments after `--`. Ordinary creation hooks have no `event.default`.

Hooks resolve under `.wtf/hooks`; `lib.prepare` selects `hooks/lib/prepare.lua`.
Dependencies use the same package-local `require` loader. Explicitly listed
modules are also available through `wtf hook list` and `wtf hook run NAME` in a
workspace with that configuration. Module declarations should be side-effect free.
Config validation and doctor compile selected files without executing them;
callback return types are checked during loading.

Missing files, invalid syntax, and invalid callback exports prevent creation.
A callback failure stops the sequence and retains the new workspace and completed
stages. Preparing an existing destination never repeats creation hooks. Inspect
and repair the retained workspace after failure; `new` on that existing path does
not retry the creation sequence.

Creation hook lists are project-local and do not inherit command defaults.
Git-ignored `.wtf/` files are not copied into new JJ workspaces; the creating
workspace's configuration supplies this post-creation sequence.

```toml
[new]
hooks = ["hello_world", "prepare_dependencies", "start_services"]
```

Advanced `lua` entrypoints remain available for lifecycle interception and custom
registration. Declarative phase lists and `lua` cannot be combined.

## The WTF Lua package

The host provides `require("wtf")`. Its submodules can also be imported directly:
`wtf.fs`, `wtf.exec`, `wtf.policy`, `wtf.resources`, and `wtf.hooks`. Imports are
cached within one invocation; `wtf.env` provides environment reads and child settings.
The `wtf` namespace belongs to the host and cannot
be shadowed by project files. Project modules keep their existing bounded loader.

For advanced entrypoints, use a setup table and one event argument per callback:

```lua
local wtf = require("wtf")

return wtf.setup {
  hooks = {
    prepare = function(event)
      wtf.policy.require(wtf.fs.exists("go.mod"), "This project needs go.mod")
      wtf.exec.stage("dependencies", { "go", "mod", "download" }, {
        inputs = { "go.mod", "go.sum" },
        reuse = true,
      })
    end,
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

`wtf.setup` returns a registration table; returning it from the TOML-selected
entrypoint enables those callbacks. Calling setup alone does not register them.
Each event exposes `ctx`, `request`, `name`, `from`, `path`, and `args`. Its
`project` and `workspace` fields read the live context, including after delegation.
Overrides additionally receive `event.default()`; an optional replacement request
uses the existing identity and exactly-once checks. Ordinary hooks have no default.
Callback return values follow the existing invocation contract and are not CLI data.

| Module | API |
|---|---|
| `wtf` | `api_version`, `ctx`, `notify(message, level)`, `setup(options)` |
| `wtf.fs` | `exists`, `stat`, `read`, `write`, `copy`, `symlink`, `readlink`, `list`, `mkdir`, `remove`, `path` |
| `wtf.exec` | `run(argv, options)`, `check(argv, options)`, `stage(name, argv, options)` |
| `wtf.env` | `get`, `has`, `require`, `select`, `merge` |
| `wtf.policy` | `require(condition, message)`, `block(message)` |
| `wtf.resources` | `list()`, `claim(kind, name, value)`, `release(kind, name)`, `claim_port(name, port)`, `listening(port)` |
| `wtf.hooks` | `has(name)`, `run(name, request)` |

Package functions use dot calls and the active context. Stage options remain
`inputs`, `check`, `reuse`, `timeout_seconds`, `env`, `cwd`, and `inherit_env`; successful results expose
`status` and `reused`. Typed raised errors still expose `code` and `message` to
`pcall`; port claim conflicts retain their existing `nil, code` return convention.
`wtf.ctx` also exposes every existing colon-style context method below. Raw
registration tables and their `(ctx, request, default)` callbacks remain supported.
The [files, commands and environment reference](lua-substrate.md) documents path
roots, copy/link policy, captured results, component directories, bounds, and the
repeatable local hello-world examples. Provider reads remain planned.

### Theme-aware messages

```lua
wtf.notify("Preparing workspace")             -- info is the default
wtf.notify("Dependencies ready", "success")
wtf.notify("Optional tool missing", "warning")
wtf.notify("Service unavailable", "error")
```

Notifications use WTF's existing Lip Gloss styles and the active Omarchy palette.
They go to stderr alongside hook logs. JSON mode and redirected output remain
unstyled, including when FORCE_COLOR is set; NO_COLOR is respected. Message ANSI
sequences and terminal control characters are sanitized before rendering.
The level selects appearance only: use `wtf.policy.block` to stop a hook.
Generic Lua `print` remains available for plain logging.

Fresh `wtf setup` still creates an empty hooks directory and selects no entrypoint.
Add callback files and a `[new].hooks` list explicitly when you want creation hooks.

## Advanced entrypoint registration

```text
.wtf/
├── config.toml
└── hooks/
    ├── init.lua         registration
    ├── initialize.lua   shared init/new preparation
    ├── guard.lua        project preflight policy
    ├── prepare.lua      workspace preparation
    ├── start.lua        optional services
    ├── test.lua         project checks
    ├── stop.lua         service teardown
    ├── remove.lua       removal wrapper
    └── lib/
        ├── args.lua
        └── compose.lua
```

```toml
version = 1
lua = "hooks/init.lua"
```

```lua
-- .wtf/hooks/init.lua
return {
  hooks = {
    guard = require("guard"),
    prepare = require("prepare"),
  },
  overrides = {
    init = require("initialize"),
  },
}
```

An implementation may be inline or imported from any project module. `require`
resolves relative to the selected entrypoint's directory, caches each module within
one invocation, and rejects escaping paths and dependency cycles. It does not load
native libraries or system Lua modules. `doctor` compiles the entrypoint without
executing it; imported modules are checked when explicitly loaded. Unused files
remain unused.

## Override initialization

```lua
return function(ctx, request, default)
  ctx:require(ctx:exists("go.mod"), "This project needs go.mod")
  ctx:require(ctx:check({ "go", "version" }), "Install Go before initializing a workspace")

  local workspace = default(request) -- register or reuse the current identity
  ctx:hook("prepare", request)
  return workspace
end
```

Before first registration, `ctx.workspace` is nil and checks use `ctx.project.path`.
After delegation, checks and stages use the registered workspace. Existing and
newly created workspaces already have an identity in their context. A blocking
init override before `default` leaves an unregistered workspace unregistered;
when reached through `new`, it preserves the created workspace for retry.

`wtf new PATH` dispatches the `init` operation override for an existing workspace
root, preserving identity and ownership. A missing path runs the `new` override
before native creation, then the `init` override using the created workspace's
configuration and identity. The creation request includes `name` (the destination
basename), `from`, and `path` (the fixed absolute destination). The `new` override
may change the native name or revision before delegation, but cannot redirect
the destination. Initialization is internal to `new PATH`; the Lua `init`
operation remains the shared preparation hook.
Preparation failures retain completed stages; repeat `new PATH` to retry.
Repeated initialization preserves ownership and lets stage reuse checks decide
which work can be retained. With no `init` override, registration is enough.

```sh
wtf new ../feature -f @ -- --compose --test
wtf new . -- --compose --test
wtf hook run inspect -- --anything ValueCase
```

Arguments after `--` reach `request.args` unchanged. The project interprets them;
they do not become framework flags. `ctx:hook(name, request)` composes named hooks
synchronously; omitting `request` forwards the current one. A failure stops the
sequence. Core creation/removal must still be delegated exactly once.

## Useful recipes

| Recipe | Project policy | Go substrate |
|---|---|---|
| Preflight gate | Check tool availability or reject a workspace name | Bounded checks and typed blocking errors |
| Optional preparation | Decide whether args select setup, testing, or services | Forwarded arguments and named hook calls |
| Recoverable setup | Declare inputs and a live readiness check | Recorded stages, reuse, cancellation, workspace locking |
| Compose sandbox | Choose ports, project name, image, health check, teardown | Atomic claims and persistent ownership records |

## Try the tracked examples

Install the [hello-world callbacks](../examples/hello-world/README.md) or the
[copy callback](../examples/copy/README.md) explicitly. Fresh setup is empty.
The hello-world example exercises file writes/copies/links, child commands,
environment settings, repeated list callbacks, and themed messages. The copy
example transfers selected ignored mock inputs and preserves destination edits.

```sh
wtf setup
# Copy the selected example files/config according to its README.
wtf new .
wtf new ../sandbox -f @
wtf list --pr=false --checks=false
```

Creation inherits the callback package/config into the child automatically.
Project service recipes can compose the same stages/claims and explicit teardown,
but no Compose registration or service startup is provided by default.
