# WTF V1 hook pilot

An isolated, first-principles experiment in Go: JJ workspace lifecycle, Lua method
wrapping, and recorded command stages. The first real hook is intentionally small:
`wtf new` prints the created workspace's path and checks the Go toolchain.

```sh
GOMAXPROCS=1 go build -p 1 -o bin/wtf-pilot ./cmd/wtf
./bin/wtf-pilot init --json
./bin/wtf-pilot new agent-a --from @ --json
```

`init` registers the current JJ repository and workspace, then runs the optional
registered `init` hook. `new` creates a JJ workspace and uses that same registration
and preparation lifecycle. A first registration returns `code: initialized`;
repeating it returns `code: already_initialized` and preserves identity and
ownership. Reusable stages check whether completed preparation can be kept.
Preparation failures remain errors and retain the workspace and completed stages;
run `init` inside that workspace to retry. Project options follow `--` for both
commands, for example `wtf init -- --compose --test`.

The project selects [.wtf/hooks/init.lua](.wtf/hooks/init.lua) explicitly:

```toml
version = 1
lua = "hooks/init.lua"
```

```lua
return {
  hooks = { init = require("initialize"), prepare = require("prepare") },
  overrides = { new = require("new") },
}
```

The imported implementation wraps the operation:

```lua
new = function(ctx, request, default)
  ctx:hook("guard", request)
  return default(request) -- native creation, then shared initialization
end
```

The registered `init` hook calls `prepare` and any optional service/test stages.
`default` creates and records the workspace, then initializes it. `prepare` prints a message and runs a
synchronous `workspace-check` stage. Filenames have no automatic behavior; only
the entrypoint's returned table registers hooks and overrides. Run a hook again
from the child to inspect reuse:

```sh
cd ../agent-a--wtf-v1-hook-pilot--wtf
/path/to/wtf-pilot hook run prepare --json
/path/to/wtf-pilot doctor --here --json
/path/to/wtf-pilot run test --json
```

Every command accepts `--help` and `--json`, including JSON help and argument errors.
JSON stdout contains exactly one versioned result. Hook messages and subprocess
output go to stderr. Help and config validation never execute hooks.

Human-readable output is the default. `list`, `ls`, `switch`, `sw`, and `w` are
one command: on a terminal, they open an inline Bubble Tea workspace browser
below your prompt, with one horizontal row per workspace. Arrow keys move,
Enter selects, and Esc cancels. Pages resize with the terminal, capped at ten
rows; page counts and page-key hints appear only when multiple pages are needed.
JSON and piped output never prompt. An optional workspace name resolves its path
directly through any alias.

For selection to change your current Bash directory, source the local wrapper:

```sh
source ./scripts/wtf-pilot.bash
wtf-pilot ls
wtf-pilot sw -p
```

The binary itself returns a selected path; the wrapper performs `cd`. It keeps
help, JSON, redirected output, and cancellation from changing directories.
It does not replace your installed `wtf` or edit shell startup files.

`list` shows active workspaces;
`--all` (`-a`) includes removed records. A dot marks the current workspace.
Local lifecycle state remains in JSON and `doctor`. Nearby paths are relative to the current
directory (`.` for the workspace root); distant paths use `~` where possible.
`list --hook` (or `list -k`) adds local **Stages**:
successful recorded stages / total recorded stages, not GitHub Actions checks.
JSON always includes full stage records. Long paths shorten with an ellipsis; narrow
terminals use a stacked layout. Use `-j` for complete records. Colors come from
Omarchy's active `colors.toml` on each invocation; theme switches apply immediately.
Redirected output and `NO_COLOR` remain unstyled. Human help uses the same shared
Omarchy presentation, with rounded sections and wrapped command/flag descriptions;
JSON help remains structured and unstyled.

Command names, aliases, and flags ignore case through shared CLI middleware.
`--JsOn`, `--JSON`, `--json`, `-J`, and `-j` are equivalent. `-f`/`-F` aliases
`--from`. Paths, workspace names, revision values, and child argv keep their case:

```sh
./bin/wtf-pilot NeW agent-a -F @ --JsOn
./bin/wtf-pilot HOOK LIST -J --HELP
```

This is a repository invariant in [AGENTS.md](AGENTS.md), backed by a test that
registers a new nested command and verifies inherited behavior.

## Optional command features

```sh
./bin/wtf-pilot list -p          # GitHub PRs: draft/open/merged/closed
./bin/wtf-pilot list -p -k       # PRs plus local hook stages
./bin/wtf-pilot list -p --refresh
./bin/wtf-pilot list --pr=false  # override an inherited preference
```

The same registered flags can be configured through TOML:

```toml
version = 1

[commands.list]
pr = true
hook = true

[commands.new]
from = "trunk()"
```

Priority, highest to lowest, resolved separately for each flag:

```text
explicit CLI flags                 highest priority
/Work/repo/src/.wtf/config.toml     current leaf
/Work/repo/.wtf/config.toml         nearest parent
/Work/.wtf/config.toml             outer parent (continue up the directory tree)
~/.wtf-v1/config.toml               global (or $WTF_HOME/config.toml)
```

Only command defaults inherit; project hooks/tasks remain project-local. Reading
defaults never runs hooks. Command aliases and flag names ignore case in settings,
and future registered flags use the same resolver and their own value validation.
Help remains available even if configuration is invalid.

PR loading uses `gh` authentication and readonly JJ bookmark/revision inspection.
On a terminal it displays cached/pending results first, then refreshes in place.
Fresh cache lasts two minutes; refresh uses at most three Go workers and an
eight-second network budget. JSON emits one final result with URLs, observed-at
timestamps, association state, and revision relationships. Stale matches retain a
`~` marker on failed refresh. Unavailable is distinct from unassociated.

Lookup considers the latest 200 PRs per repository; **unassociated** means no match
in that result, not proof that a workspace never had a PR. Associations use current
or parent bookmarks and exact revision IDs, never workspace names. Unsnapshotted
filesystem changes and GitHub Actions CI are not inspected by this view.

## Hook API

```lua
-- Pre-creation checks use ctx.project.path; failed requirements block creation.
ctx:require(ctx:exists("go.mod"), "This project requires go.mod")
ctx:require(ctx:check({ "go", "version" }), "Go must be available")

-- After default creation, stages use ctx.workspace.path.
ctx:stage("prepare", { "uv", "sync", "--frozen" }, {
  inputs = { "pyproject.toml", "uv.lock" },
  check = { "./scripts/check-workspace-ready" }, -- optional live reuse/readiness check
  reuse = true,
  timeout_seconds = 120,
})
ctx:hook("prepare")
ctx:has_hook("start")
ctx:check({ "./scripts/check-service" })
ctx:claims()
ctx:claim("docker", "database", "project-specific-container-name")
ctx:release("docker", "database")
local port, error_code = ctx:claim_port("web", 3000)
ctx:listening(3000)
```

The checked-in module also registers `guard`, `start`, `test`, `stop`, and `remove`.
Its optional Compose recipe provisions an isolated Redis service:

```sh
./bin/wtf-pilot new sandbox -f @ -- --compose --test
```

Raw hook arguments after `--` reach `request.args`; project Lua decides their
meaning. [Module contract and Compose walkthrough](docs/hooks.md).

Claims record ownership; hooks choose port policy and provision/teardown Docker
resources through ordinary argv commands. Stages serialize within one workspace,
record failure/cancellation, and preserve completed setup. Reuse requires matching
inputs or a live check; test tasks rerun by default. This pilot has no background
job runner, durable process supervisor, or native Docker provider.

## Local state and boundaries

- Linux pilot; it does not replace the installed `wtf` binary or upgrade JJ.
- Separate shared repository state: `.jj/repo/wtf-v1-pilot/`.
- Separate global index: `~/.wtf-v1/`, or an explicit `WTF_HOME`.
- Lua 5.1 through GopherLua, with project operations exposed through `ctx` and
  explicit project-local `require` imports. Native/system Lua modules and escaping
  module paths are rejected; external scripts use stage argv commands.
- Hook execution is bounded to ten minutes and responds to cancellation. Individual
  stages default to 120 seconds, with an explicit maximum of 3600 seconds.
- `new` and `remove` overrides currently must delegate to the core once.
- Resource claim conflicts are checked within a repository; cross-repository
  leases and add-on loading are future work.
- Removal verifies native JJ membership and the workspace marker, refuses changed
  source or external claims, and protects current/adopted workspaces. A `remove`
  override must teardown external resources before calling `default`.
- Earlier command docs and Pi extension describe the old implementation, not this
  pilot. See [the direction draft](docs/roadmap.md) for the future design.

```sh
GOMAXPROCS=1 go test -p 1 ./internal/...
GOMAXPROCS=1 go vet -p 1 ./internal/...
```
