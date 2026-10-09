# WTF V1 — Direction

> Review draft · Proposed commands, schemas, and APIs below are illustrative.
> This is a new direction, not an implementation plan or a released contract.

WTF is a small Go workspace manager with a CLI and Charm/Bubble Tea navigation.
It connects repositories, workspaces, PRs, and owned resources. JJ owns local
history and publishes to GitHub through `jj git push`; WTF shows workspace-to-PR
relationships. UV, Mise, Bun, and existing project tools keep their jobs. WTF
provides ownership and execution primitives; hooks decide project behavior.

TOML names tasks, hooks, and operation overrides; scripts supply behavior. Opinionated
recipes are optional add-ons. Project tasks work in ordinary checkouts; workspace
creation uses JJ. Application environment policy stays in hooks. Toolchain
installation, standalone Git worktree management, and a required daemon are outside V1.

## One command surface, three views

```sh
wtf init                              # opt this repository into WTF
wtf list                              # alias: ls
wtf ports list
wtf new auth --from 'trunk()'          # automatically invokes a configured new override
wtf run test -- tests/auth
wtf hook list
wtf hook run prepare                  # manual invocation of a named hook
wtf validate                          # configuration and hook declarations
wtf doctor                            # observed registry and runtime health
wtf switch auth                       # aliases: sw, w; shell integration enters it
wtf remove auth                       # alias: rm; explicit workspace target
```

### Help and JSON are required

Every command, command group, and alias accepts `--help` and `--json`. Help describes
arguments, defaults, scope, side effects, and examples; it never runs hooks or
changes state. `--json --help` returns structured help through the same envelope.
Command names, aliases, and flags are case-insensitive through common middleware.
`-j` aliases `--json`; `-f` aliases `--from`. Values and child argv retain their case.
New commands inherit the rule, with a dynamic extension test guarding that boundary.

```sh
wtf new --help
wtf ports --json --help
wtf new auth --json
wtf hook run prepare --json
```

```json
{"version":1,"command":"new","ok":true,"data":{"workspace_id":"…","path":"/Work/auth--alita_core"},"error":null}
```

```json
{"version":1,"command":"new","ok":false,"data":null,"error":{"code":"workspace_exists","message":"Workspace auth already exists"}}
```

JSON mode is non-interactive: stdout contains one versioned result, while progress
and child-process output go to stderr. Errors use stable codes and nonzero exit
statuses, including argument errors. Partial failures report completed stages and
retained resources in `data`. Text-generating commands return their text in `data`;
aliases share the canonical command's schema. Core rendering owns this boundary,
even when a project overrides the operation.

```go
// Contract test traverses the complete registered command tree.
for _, command := range allCommands(root) {
    assertUsefulHelp(t, command)
    assertJSONHelp(t, command)
    assertJSONSuccess(t, command, fixtures)
    assertJSONFailure(t, command, fixtures)
    assertAliasParity(t, command)
}
```

Shared CLI infrastructure supplies flags and rendering. A new command ships only
with help and schema fixtures checked by this gate, including prompt-free behavior
and uncontaminated JSON stdout. Tests use temporary state and fake external tools.

```sh
cd /Work
wtf list                              # registered repos beneath /Work
wtf new auth --repo alita_core         # parent view needs a repository target

cd /Work/alita_core/src
wtf list                              # every workspace in alita_core
wtf ports list --here                  # only the current workspace
wtf list --global --json               # every registered repository
wtf list -p                           # GitHub PR state, loaded on demand
wtf list -p --refresh                  # refresh cached PR metadata
```

Commands stay barebones; registered flags can be enabled once or inherited as
defaults. Per-flag priority is **CLI → leaf → nearest parent → outer parents →
global**. Directory scopes use `.wtf/config.toml`; global uses
`$WTF_HOME/config.toml`. Explicit flags win, including `--pr=false`.

```toml
version = 1

[commands.list]
pr = true
hook = true
```

PR views use a two-minute cache, at most three concurrent Go workers, and a bounded
refresh. Terminals show cached/pending state first; JSON emits one final result.
Failed refresh retains stale data instead of claiming no PR exists.

Logical membership follows repository identity, regardless of workspace placement:

```text
/Work
├── alita_core
│   ├── default       #1652  merged       unpublished local work
│   └── auth          #1660  draft        matches published head
└── next_gen_ui
    ├── default       #1568  open         matches published head
    └── experiment    —      unassociated local only
```

PR rows show association, published-head agreement, and freshness. Unknown differs
from “no PR.” Explicit links and bookmark discovery support multiple associated PRs.

## Configuration names behavior

Project configuration and local runtime state:

```text
alita_core/
└── .wtf/
    ├── config.toml
    └── hooks.lua

~/.wtf/                               # local repository index and runtime state
```

```toml
version = 1

[tasks.test]
command = ["uv", "run", "pytest"]

[tasks.check]
command = ["task", "check"]

[hooks.prepare]
command = ["uv", "run", "python", "scripts/workspace_prepare.py"]

[hooks.start]
lua = "hooks.lua:start"

[overrides.new]
lua = "hooks.lua:new"
```

```text
wtf run test -- tests/auth
    → uv run pytest tests/auth

wtf run check
    → task check

wtf hook run prepare
    → project script owns its environment and configuration

wtf new auth
    → configured new override → default creation + project preparation
```

## Override an operation

A configured override replaces the default method. Calling `default` is the
equivalent of `super`: it bypasses override dispatch, so delegation cannot recurse.
Without an override, the normal operation runs unchanged.

```lua
return {
  new = function(ctx, request, default)
    -- Project policy can change inputs before delegating.
    request.from = request.from or "trunk()"

    local workspace = default(request) -- native creation + ownership record
    ctx.hooks:run("prepare", workspace)
    return workspace
  end,
}
```

```text
CLI / TUI / agent → app.new(request)
                       ├── override configured → project method
                       │                            └── default(request), if needed
                       └── otherwise           → default(request)
```

Overrides may wrap or replace behavior; ownership validation remains in core
primitives. A preparation failure preserves the created workspace and reports what
failed. External-script delegation needs a protocol; the Lua shape above is conceptual.

### Compose stages in project code

Named stage functions share a workspace context. Calls finish before the next
stage starts; conditions and ordering stay in the override, without a separate
workflow language.

```lua
local stages = {}

function stages.prepare(ctx, workspace)
  ctx.hooks:run("prepare", workspace)
end

function stages.start(ctx, workspace)
  if not ctx.hooks:exists("start") then return end
  local service = ctx.hooks:run("start", workspace)
  service:wait_ready({ timeout_seconds = 30 })
end

return {
  new = function(ctx, request, default)
    local workspace = default(request)
    stages.prepare(ctx, workspace)
    stages.start(ctx, workspace)
    if ctx.hooks:exists("verify") then
      ctx.hooks:run("verify", workspace)
    end
    return workspace
  end,
}
```

Hook completion and service readiness are separate: a start hook returns a tracked
service handle, and the project supplies its readiness check. Failure stops the
remaining stages and reports completed work. Sequential stages order one invocation;
concurrent invocations still require atomic claims and resource-specific locking.

## Hooks choose; the registry tracks

Illustrative Lua API; embedding versus a CLI-backed module remains open:

```lua
local wtf = require("wtf")

return {
  start = function(ctx)
    -- Project policy, reusable as an add-on.
    local port = 3000
    while true do
      if not wtf.net.listening(port) then
        local claim, err = ctx.ports:claim("frontend", "127.0.0.1", port)
        if claim then break end
        if err.kind ~= "conflict" then error(err.message) end
      end
      port = port + 1
      if port > 65535 then error("No port available") end
    end

    return ctx.processes:start("frontend", {
      command = { "bun", "run", "dev", "--port", tostring(port) },
      ready = { command = { "./scripts/check-frontend-ready", tostring(port) } },
    })
  end,
}
```

External scripts use the same primitives:

```sh
wtf ports list --here --json
wtf ports claim frontend --address 127.0.0.1 --port 3000
wtf process start frontend -- bun run dev --port 3000
wtf process list --here --json
```

Registry claims are atomic, not socket reservations. Hooks choose retries and
cleanup; failed launches stay visible. Process environments are opaque inputs.

## Go owns the mechanics

```text
cmd/wtf/                 entry point
internal/
├── app/                 operations shared by CLI, TUI, and hooks
├── scope/               current workspace, repository, parent, global
├── registry/            repository identity and owned resource records
├── jj/                  native workspace lifecycle and revision inspection
├── github/              GitHub PR links, published revisions, freshness
├── process/             launch, logs, cancellation, verified ownership
├── hook/                context and script execution
├── config/              TOML declarations
├── cli/                 commands and versioned JSON results
└── tui/                 Bubble Tea views over the same operations
```

```text
JJ         → actual workspace membership and revision state
WTF        → opted-in repositories, management state, claims, processes
GitHub     → PR state and published revisions, with observed-at timestamps
Project    → setup, environment, startup, readiness, teardown policy
```

Inspection is read-only. Removal verifies identity and preserves failed cleanup
state. Process ownership uses more than a bare PID. Existing JJ workspaces can be
adopted; shared services remain distinct from workspace-owned services.

## Delivery direction

### First usable slice — hooks in real WTF workspaces

Dogfood the override/stage lifecycle before expanding navigation or PR views.
The goal is to preserve setup and testing work when agents operate asynchronously
across several workspaces. Agents own fan-out; WTF owns each workspace's observable
lifecycle. Hooks provision isolated services and Docker components using project tools.

```text
agent A → new → prepare → isolated service ready → test → teardown
agent B → new → prepare → isolated service ready → test → teardown
                        ↑ separate ownership and service state ↑
```

```sh
wtf new agent-a --json           # override delegates creation, then runs stages
wtf new agent-b --json           # independent workspace; caller controls concurrency
wtf hook run test --json         # project tests against this workspace's services
wtf doctor --here --json         # completed/failed stages and owned resources
wtf remove agent-a --json        # teardown override, then default removal
```

Pilot acceptance:

- Run real create/prepare/test/teardown hooks through the same CLI used by agents.
- Hook-defined service names, ports, and Docker resources belong to one workspace;
  removing it leaves another workspace's services intact.
- Stage results are recorded. Retry reuses completed work only when its inputs and
  readiness still match; hooks supply reuse checks and idempotent repair behavior.
- Failure and cancellation preserve completed work, identify the failed stage,
  and expose remaining cleanup rather than silently recreating everything.
- Exercise concurrent ownership claims with lightweight fixtures; expensive setup
  and validation stay bounded, with one worker by default.
- Every pilot command passes the help/JSON contract gate. No daemon or agent
  orchestration engine is required for the pilot.

```text
Dogfood   → hook stages, isolated services, recorded progress and safe retry
Orient    → scope views, PR relationships, registry diagnostics
Extend    → reusable tasks and hook add-ons from observed friction
Simplify  → verified native colocation, state migration, legacy removal
```

Review: hook runtime and delegation protocol; state migration; minimal add-on API.
No JJ upgrade or implementation is part of this draft.
