# State and safety

WTF stores repository-wide state under `.jj/repo/wtf-v1-pilot/`, retains the global
index under `~/.wtf-v1/`, and accepts `WTF_HOME` to choose an isolated global index.
These legacy path names remain unchanged in 1.0.0. The identity marker is
`.jj/wtf-workspace.json`; older `.wtf/workspace.json` markers are still readable.
Do not copy those identity/state files between workspaces.

`.wtf/` contains local project configuration, hooks, tasks, and optional artifacts.
Setup's `.gitignore` ignores everything there. Ignore rules do not untrack files
already committed. New copies only the configuration allowlist, never the whole
local tree. Preparing an existing workspace preserves configuration and identity.

Registered existing/native workspaces are adopted, not owned; registration does
not grant deletion permission. Removal protects the current workspace, adopted
workspaces, identity mismatches, unknown inspection results, and resource owners.
Force mode discards source edits but does not bypass these protections.

Lua filesystem APIs reject escapes, guard VCS/identity metadata, bound file/copy
sizes, and publish individual writes atomically. Lua command cwd is bounded;
commands themselves run with caller permissions and are not sandboxed. No
background service supervision is provided. Treat project Lua as trusted code.
Timeout/cancellation kills command process groups on Linux.

Failed preparation/cleanup remains visible and retryable; original hook errors
survive cleanup failures. Finally/failed phases have bounded cancellation-independent
cleanup contexts. Never infer successful delivery from a cached stage or old CI
observation. [Doctor](doctor.md), [removal](remove.md), and [Lua](lua-substrate.md)
provide the detailed contracts.
