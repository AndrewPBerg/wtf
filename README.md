# WTF V1 hook pilot

An isolated, first-principles experiment in Go: JJ workspace lifecycle, Lua method
wrapping, and recorded command stages. The first real hook is intentionally small:
`wtf new` prints the created workspace's path and checks the Go toolchain.

```sh
GOMAXPROCS=1 go build -p 1 -o bin/wtf-pilot ./cmd/wtf
./bin/wtf-pilot new . --json
./bin/wtf-pilot new agent-a --from @ --json
```

`new NAME` creates a sibling of the registered main repository, using exactly the
supplied name without a repository prefix or suffix. Creation from another
workspace or a subdirectory keeps the same flat layout. Explicit relative paths
such as `./agent-a` or `../agent-a` resolve from the current directory; absolute
paths also work. `new .` registers and prepares the current JJ workspace. An existing
workspace root is prepared in place; a missing path creates a workspace at that
exact location, named after its basename. `--from` applies only to creation.
Bare `wtf` shows help. `new PATH` is the public entry point for workspace creation
and preparation; initialization is an internal lifecycle phase.

`wtf setup` creates a local `.wtf/config.toml`, generated editor schema, and
completely empty `hooks/` and `tasks/` directories in the current directory.
Its `.gitignore` contains only `*`, so everything inside stays
local and untracked. Existing files are preserved. Setup runs no hooks or
dependency installs and does not register a workspace. See [setup](docs/setup.md).

Both modes run the configured Lua `init` override, which delegates registration to
`default(request)` and then runs project preparation. Only creation runs the Lua
`new` override first; its `request.path` is the fixed absolute destination.
A first registration or creation returns `code: initialized`;
repeating it returns `code: already_initialized` and preserves identity and
ownership. Reusable stages check whether completed preparation can be kept.
Preparation failures remain errors and retain the workspace and completed stages;
repeat `new PATH` to retry. Existing native workspaces retain their names and
ownership, so registration never makes them removable. Existing files, ordinary
directories, and directories inside a workspace are rejected. Project options
follow `--`, for example `wtf new . -- --compose --test`.

Creation hooks are declared directly in `.wtf/config.toml`:

```toml
version = 1

[new]
after = ["hello_world"]
```

```lua
-- .wtf/hooks/hello_world.lua
local wtf = require("wtf")

return function(event)
  wtf.notify("hello world", "success")
end
```

WTF creates and registers the workspace, then runs the listed callbacks in order.
No `init.lua` or imperative creation wrapper is required. A failure stops the
sequence and retains the workspace and completed stages. Existing destinations
do not rerun creation hooks. Unlisted files never run. Git-ignored `.wtf` files
stay local; this sequence uses the creating workspace's configuration.
Advanced Lua entrypoints and lifecycle overrides remain available separately.
See [hooks](docs/hooks.md) for the callback event and current package API.
See [files, commands and environment](docs/lua-substrate.md) for substrate APIs
and the [repeatable hello-world examples](examples/hello-world/README.md).

`new`, `list`, `remove`, and `invocation` all support ordered `before`, `after`,
`failed`, and `finally` arrays. Before hooks can block the operation; failed hooks
inspect the original error; finally hooks run with bounded cleanup time even after
cancellation. The [hello-world matrix](docs/hooks.md#trigger-phases) verifies all
16 combinations. `[new].hooks` remains compatible as a post-creation shorthand.

Every command accepts `--help` and `--json`, including JSON help and argument errors.
`wtf doctor` combines configuration validation, tool readiness and workspace health.
It reports each area independently and checks configuration before registration,
without executing hooks or changing state. There is no separate `validate` command.
JSON stdout contains exactly one versioned result. Hook messages and subprocess
output go to stderr. Help and config validation never execute hooks.

Human-readable output is the default. `list` and its aliases, `ls` and `sw`, open
an inline Bubble Tea workspace browser on a terminal
below your prompt, with one horizontal row per workspace. Arrow keys move,
Enter selects, and Esc cancels. Pages resize with the terminal, capped at ten
rows; page counts and page-key hints appear only when multiple pages are needed.
The workspace count and page indicator share help's caption in the top border.
Up/Down and k/j wrap between the first and last workspace.
Numeric prefixes repeat j/k motions: `4j` moves down four rows and `4k` moves
up four rows, wrapping through the list. `gg` goes to the top of the current
page and `G` to its bottom, including a partial final page. Left/Right and
PageUp/PageDown keep their existing page navigation.
Workspace names are terminal hyperlinks to Zed in list and remove: hover to see the absolute
destination and Ctrl-click (or your terminal's link gesture) to open that workspace
folder. This requires Zed's `zed://` URL handler. Missing and removed paths have
no link; redirected output stays plain.
JSON and piped output never prompt. An optional workspace name resolves its path
directly through any alias.

With JJ 0.46+, `new` inherits JJ's native Git worktree colocation when its source
workspace is colocated and `git.colocate = true`. Zed reads that workspace's real
Git metadata. Existing non-colocated workspaces are not converted by opening a
link; inspect them with `jj git colocation status` from the workspace folder.

For selection to change your current Bash directory, source the local wrapper:

```sh
source ./scripts/wtf-pilot.bash
wtf-pilot ls
wtf-pilot sw -p
```

The binary itself returns a selected path; the wrapper performs `cd`. It keeps
help, JSON, redirected output, and cancellation from changing directories.
The development wrapper does not replace your installed `wtf` or edit shell startup files.

For the installed binary, install and source the Bash navigation wrapper:

```sh
install -Dm644 scripts/wtf.bash ~/.local/share/wtf/wtf.bash
source ~/.local/share/wtf/wtf.bash
wtf ls
```

Source that installed file from your personal Bash configuration so new terminals
also enter the selected workspace. Existing aliases such as `f='wtf'` use the same
function. `command wtf` invokes the underlying CLI directly.

Enable Bash completions for `wtf` and `f` in the current shell:

```bash
source <(wtf completion bash)
```

Completions include commands, aliases, flags, live workspace names, and configured
tasks. Workspace and task names support fuzzy matches: `f ls tmsg<Tab>` can
complete `testing-msg`. Generate Zsh or Fish scripts with `wtf completion zsh`
or `wtf completion fish`; see [shell completion setup](docs/completion.md).

`list` shows active workspaces whose directories are available;
`--all` (`-a`) includes retained unavailable and removed records. Listing removes
confirmed missing, idle registrations from WTF, JJ, and native Git metadata so
their names can be reused. Resource owners and locked workspaces are retained.
A dot marks the current workspace.
Local lifecycle state and full workspace paths remain in JSON and `doctor`.
The workspace name carries the editor link, so there is no separate PATH column.
`list --hook` (or `list -k`) adds local **Stages**:
successful recorded stages / total recorded stages, not GitHub Actions checks.
JSON always includes full stage records. Long names shorten with an ellipsis; narrow
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

List and remove show an `AGE` column. Age is time since
first WTF registration, using whole seconds (`S`), minutes (`M`), hours (`H`), days
(`D`), or weeks (`W`). Reinitializing a workspace preserves that time. Older records
without a timestamp show `?`; no migration or inferred age is applied. JSON includes
the UTC `created_at` timestamp when known.

`wtf new hello_world -m "hello this me testing"` saves an optional workspace
description. List and remove display it below the highlighted row. A repeated
`new PATH` preserves it; `new PATH -m TEXT` updates it, and `-m ""` clears it.
JSON exposes the description as `message` when nonempty.

Use `wtf change hello_world -m "Picking this up tomorrow"` to replace the
message later without preparing the workspace again.
`wtf change hello_world -m ""` clears it;
`wtf change -m "New message"` targets the current workspace.
See [wtf change](docs/change.md) for details.

```sh
./bin/wtf-pilot list -p          # GitHub PRs: draft/open/merged/closed
./bin/wtf-pilot list -p -k       # PRs plus local hook stages
./bin/wtf-pilot list -p -c       # PRs plus GitHub CI check counts
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
PRs and Linear-first review links are enabled by default; use
`--pr=false` to disable them. On a terminal, matched PRs show a Nerd Font pull
request glyph and number as an OSC 8 hyperlink. Ghostty handles opening the link
through its normal link interaction. Redirected output stays plain; JSON keeps
the GitHub URL and resolved review destination in each PR record. The visible
glyph, number, state, and colors stay the same for both destinations.

The embedded review adapter uses Node.js and the existing Codex MCP OAuth
connection. It verifies the repository's expected Linear workspace before
resolving the PR. Default connections use Cascading Labs; `~/Desktop/Work/**`
uses the configured `alita` profile and `linear-alita` connection. A found review
opens its canonical Linear URL; no matching review or an unavailable connection
falls back to GitHub. No token is copied into WTF, and no model turn is started.
Account routing follows the current Codex wrapper and explicit project MCP
configuration. Only the selected Linear connection is started.

`wtf doctor` reports the optional Linear connection, verified workspace, and
lookup failures alongside JJ/GitHub readiness. Missing Node/Codex, unavailable
authentication, missing review tools, and wrong-workspace routing include repair
guidance. These optional warnings do not block normal GitHub-backed operation.
No review for a particular PR is a normal fallback, not a failed diagnostic.

Verified review destinations are cached for five minutes and confirmed missing
reviews for one minute, scoped to repository/account configuration. Transient
lookup failures are retried on the next refresh. `ls --refresh` rechecks the
destination. PR state and check counts still refresh independently.
Draft, open, closed, and merged PRs use distinct Octicon Nerd Font glyphs and
the theme's muted, success, error, and accent colors respectively.
CI checks are enabled by default in a separate column. `✗ 1` means one failed
GitHub check; `↻ 2` means two pending checks;
`✓ 3` means three passing checks. Mixed failures and pending checks show both
counts. Cancelled/skipped checks use `⊘`, and unknown or incomplete observations
use `?`. No PR or no checks leaves a blank cell. Clicking a CI count opens the
PR's Checks tab. Counts describe GitHub's rollup for the published PR head;
local hook stages remain separate. JSON includes the head and all category counts.
Use `--checks=false` to hide CI. `--pr=false --checks=false` disables remote
metadata views. Both flags support inherited `[commands.list]` TOML defaults.
Every list invocation with PRs enabled checks GitHub. The interactive browser
displays cached/pending results immediately, then lazily updates the frame with
the live response. It rechecks automatically every 15 seconds while open.
A `↻` caption indicates an active lookup.
Refreshes run in the background, stay bounded, and are cancelled when the picker
closes. JSON and redirected output wait for the live lookup and emit one result.
Cached data is retained as a marked fallback when GitHub is unavailable.
Refresh uses at most three Go workers and an
eight-second network budget. JSON emits one final result with URLs, observed-at
timestamps, association state, and revision relationships. Stale matches retain a
`~` marker on failed refresh. Unavailable is distinct from unassociated.

Lookup considers the latest 200 PRs per repository; a blank PR cell means no match
in that result, not proof that a workspace never had a PR. Associations use current
or parent bookmarks and exact revision IDs, never workspace names. Unsnapshotted
filesystem changes are not inspected by this view.

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
./bin/wtf-pilot new ../sandbox -f @ -- --compose --test
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
- `init`, `new`, and `remove` overrides currently must delegate to the core once.
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
