# Project hook modules

Hooks are explicit project code. TOML selects one entrypoint; its returned table
registers functions. A file named `new.lua` has no effect on its own.

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
native libraries or system Lua modules. `validate` compiles the entrypoint without
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

`wtf init` dispatches the `init` operation override. Its default registers an
existing workspace, preserving identity and ownership. `new` calls that operation
after native creation, using the created workspace's configuration and identity.
Preparation failures retain completed stages; retry through `init` in that workspace.
Repeated initialization preserves ownership and lets stage reuse checks decide
which work can be retained. With no `init` override, registration is enough.

```sh
wtf new feature -f @ -- --compose --test
wtf init -- --compose --test
wtf hook run inspect -- --anything ValueCase
```

Arguments after `--` reach `request.args` unchanged. The project interprets them;
they do not become framework flags. `ctx:hook(name, request)` composes named hooks
synchronously; omitting `request` forwards the current one. A failure stops the
sequence. Core creation/removal must still be delegated exactly once in this pilot.

## Useful recipes

| Recipe | Project policy | Go substrate |
|---|---|---|
| Preflight gate | Check tool availability or reject a workspace name | Bounded checks and typed blocking errors |
| Optional preparation | Decide whether args select setup, testing, or services | Forwarded arguments and named hook calls |
| Recoverable setup | Declare inputs and a live readiness check | Recorded stages, reuse, cancellation, workspace locking |
| Compose sandbox | Choose ports, project name, image, health check, teardown | Atomic claims and persistent ownership records |

## Try the Compose recipe

The checked-in registration supports `--compose` and `--test`. Default `new` only
runs the small Go check. The recipe uses a cached `redis:7-alpine` image, loopback
ports, UUID-named projects, ownership labels, and bounded container resources.
Docker provisioning and environment policy stay entirely in project Lua/YAML.

```sh
./bin/wtf-pilot new sandbox -f @ -- --compose --test
```

From the printed workspace path, using the pilot binary's absolute path:

```sh
wtf-pilot hook run prepare       # reusable local preparation
wtf-pilot hook run start         # start/recheck the owned Compose sandbox
wtf-pilot hook run test          # Redis ping, or focused Go tests without Compose
wtf-pilot hook run stop          # remove services while keeping source
```

Return to the parent pilot workspace before removal:

```sh
./bin/wtf-pilot remove sandbox
```

Teardown verifies Docker ownership labels before deleting the project. Changed
source blocks this recipe's removal before service teardown. Failed startup or
cleanup retains claims for inspection and repair; stopping services does not delete
source. The recipe never pulls images automatically. Cross-repository port claims
and arbitrary Compose-file safety are not enforced by the Go core.
