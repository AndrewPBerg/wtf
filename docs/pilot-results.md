> Historical design or 0.x reference. This page is not the WTF 1.0.0 contract.
> Start with the [current documentation](README.md) and [migration guide](migration.md).

# First hook pilot — local evidence

The isolated `wtf/v1-hook-pilot` workspace replaces the previous Go runtime with a
fresh core. The original implementation remains in the parent JJ revision. This is
a Linux experiment, not a release or a migration of existing WTF state.

## First real hook

```text
wtf new hook-demo-a --from @ --json
  → default JJ workspace creation
  → Lua workspace hook prints the path on stderr
  → workspace-check runs go version
  → one JSON result reports the ready workspace and succeeded stage
```

Two real sibling workspaces were created. Re-running the named hook in the first
reused its matching completed stage. The second had independent identity and stage
state. Both were removed through the pilot, preserving the pilot workspace.

## Checks

- Focused Go tests: CLI help/JSON/alias contracts, subprocess-output isolation,
  stage reuse and invalidation, readiness loss, failure/retry, cancellation,
  concurrent resource claims, implicit overrides, and protected removal.
- A runaway Lua loop responds to cancellation; hook execution has a default bound.
- `go vet` passed for the fresh implementation.
- The claim-contention test passed with Go's race detector and one build worker.
- The local pilot binary was rebuilt after the final runtime changes.
- Native JJ workspace creation/removal and installed WTF editor-shadow refresh
  were exercised on the existing JJ installation.

The optional PR view was subsequently verified against a real draft PR using a
disposable repository and local bookmark, without publishing. A second lookup
reused cache; leaf TOML enabled the view, and explicit `--pr=false` disabled it.
Tests cover bounded concurrent refresh, stale cache fallback, fork-safe association,
global/parent/leaf defaults, generic future flag types, and terminal width limits.

Explicit Lua modules were subsequently exercised with a real Compose recipe.
Two workspace-owned Redis services used different host ports and separate data.
Removing A left B running. B's stale JJ working copy blocked teardown before
deleting services; `jj workspace update-stale` restored the disposable workspace,
and retry removed its containers, network, and claims. Both demos were cleaned up.
An additional start/stop/restart demo verified that a healthy service reuses its
completed startup stage, while lost readiness reruns startup. That demo was also
removed, including its container and network.
Pre-creation guard failure, unused-file non-registration, import cycles/escapes,
argument forwarding, and caught-error isolation have focused regression tests.

## October 11 — workspace browser and help

`list`, `ls`, `switch`, `sw`, and `w` now share a Bubble Tea browser. Removed
records are hidden unless `--all` is requested. Relative paths simplify the view;
the local status column is omitted. Optional PR and stage information remain available.
Human help uses the shared Omarchy palette, rounded panels, and wrapped text.
JSON and redirected output stay non-interactive.

Focused CLI tests passed for pagination, resizing, selection, cancellation,
unavailable paths, help width/theme, JSON, aliases, and removed-record filtering.
Real PTY runs verified selecting a sibling workspace through the Bash wrapper
changes directory, and cancelling preserves the original directory. The local
binary was rebuilt. No shell startup files or installed binaries were changed.

The command-surface pass also distinguishes first/repeated initialization with
`initialized` and `already_initialized` success codes while preserving identity.
Combined short JSON/help flags bypass shell navigation. All current pilot Go
packages passed serial tests and vet; the local binary and command help were
checked again. This is a local pilot checkpoint, not release certification.

Initialization and creation now share the `init` operation override, which delegates
registration to core and runs project preparation. The recipe overrides `init`
rather than `new`.
Focused regressions exercise existing/created workspaces, independent preparation,
argument forwarding, retained ownership, stage reuse, and retrying failed `new`
preparation through `init`. Terminal JSON was verified without startup control
queries using a real PTY.

Asynchronous job supervision,
cross-repository leases, hosted CI, and release compatibility were not
validated. Existing command documentation and Pi integration remain historical.
No global binary replacement, JJ upgrade, push, or deployment was performed.
