# v0.8 identity kernel: acceptance test plan

WorkUnit: `6d77417a-6754-44a2-851c-739faed74cd5`
Lane: C (independent acceptance design)

This plan is intentionally implementation-independent. The current tree has no
`internal/identity` boundary or identity-bearing `vcs.Worktree`; therefore no
new production-facing tests are added in this lane. Tests below become
executable when the Lane A/B contracts land, without requiring speculative APIs.

## Contract and executable matrix

The authoritative fixture is a temporary `WTF_HOME`, with a fresh global
`state.json`, lock, repository markers, and two repositories/checkouts. Every
mutation is followed by reading and validating the complete persisted state and
checking that the original state bytes are unchanged on failure.

| Area | Acceptance assertions | Suggested executable shape |
| --- | --- | --- |
| Canonical IDs | Repository and workspace IDs parse as lowercase RFC 4122 text; malformed, uppercase, non-UUID, duplicate, missing, or recycled IDs are rejected. IDs survive reload. | Table-driven validator tests plus a state fixture containing each invalid form. |
| Canonical names | Names are lowercase `<repo-slug>/<workspace-slug...>`; reject empty segments, `.`/`..`, absolute paths, control characters, and ambiguous normalization. Active names and active canonical paths are unique across all repositories. | Table-driven name tests; create two repos and attempt the same name/path. |
| Adoption | Existing `repos.json` and physical default/v0.7 workspaces are readable and get persisted identities incrementally. Adoption does not rename JJ or Git resources. Nonconforming/colliding legacy entries remain listable but identity-dependent mutation returns a deterministic rename-required error. | Seed old registry and real git/JJ checkouts; run read/adopt; compare VCS listing and state bytes/resources. |
| Concurrency | Two processes claiming one active name produce exactly one success; one active record exists; loser does not overwrite winner or leave an active leak. Lock acquisition and atomic replacement work across processes. | `go test -race` subprocess test with a barrier and two independent store instances. Inspect final JSON and both results. |
| Lifecycle/tombstones | Physical creation uses `pending`; failed VCS creation cannot become active. Removal creates `removed` with `removed_at`; removed IDs remain forever. Reusing a removed name creates a new workspace ID and never revives the tombstone. | Transition table test, injected VCS failure test, then remove/create/reload assertions. |
| Rename/move | Approved rename changes the canonical name and (JJ only) native name; workspace ID is stable. Path movement also preserves ID. Git branch/bookmark metadata is unchanged. | Seed one record, apply rename/move, reload, compare IDs and backend observations. |
| Corrupt/unsupported state | Invalid JSON, unsupported schema, invalid records, duplicate IDs/names, dangling repository references, and impossible lifecycle combinations fail closed. Original bytes remain byte-for-byte intact; no partial mutation occurs. | Write each corrupt fixture, attempt a mutation, assert error, bytes, lock release, and no new active records. |
| JJ naming | New JJ native workspace name equals canonical WTF name, including slash segments; approved rename updates it. No bookmark is implicitly created. Read-only listing remains side-effect free (`--ignore-working-copy`). | Real `jj` repo integration test: inspect `jj workspace list`, bookmarks, and operation/log state before/after listing. |
| Git separation | Canonical WTF name is independent of Git branch. Adoption/rename never silently renames an existing branch; branch remains the forge-origin ref. | Real Git worktree test: create branch `origin/topic`, assign/rename WTF name, assert `git branch --show-current` and refs are unchanged. |
| JSON compatibility | Existing `branch`, `path`, `head`, `change_id`, `vcs`, and other human fields retain meaning and shape. Add `repository_id`, `workspace_id`, `name`, and `native_name`; old consumers can unmarshal and new consumers can distinguish UUIDs. Omit/retain optional fields consistently. | Golden JSON fixtures for old and new `ls`, `sw`, `new`, and global output; unmarshal old fixture with current types and assert required new fields in new output. |

## Migration hazards to test explicitly

- The v0.7/default JJ workspace is commonly named `default`; it is not a
  canonical scoped name. Adoption must preserve it and avoid silently issuing
  `jj workspace rename`. If it collides with another adopted name, list it but
  require an explicit rename before identity-dependent operations.
- Existing Git worktrees commonly use branch names such as `main`, `feature/x`,
  `pr-42`, or names derived from the old sibling path. These are forge refs,
  not canonical WTF names. Never turn a branch rename into identity migration.
- A repository marker may be absent, malformed, or located in a secondary JJ
  workspace with no `.git`; discovery must use the backend and converge on the
  shared marker/state location.
- Existing `~/.wtf/repos.json` may be v1 (bare path array) or v2 registry data
  without IDs. It must remain readable and be adopted without losing VCS
  preference entries or changing path ownership.
- Current ports are keyed by human branch/workspace strings in a repo-local
  `ports.json`. Renaming an adopted workspace can remap a key only as an
  explicit, atomic migration; never guess when the old key is ambiguous, and
  never silently merge two assignments. Unrelated resource-store redesign is
  out of scope for this slice.
- Existing JSON consumers may treat `branch` as the selector. Preserve it for
  compatibility, but acceptance must verify UUID selection is authoritative and
  human-name selection errors when ambiguous.

## Recommended integration coverage

1. A process-level lock race using two binaries/`go test` subprocesses and the
   same `WTF_HOME`; run it under `go test -race` where practical.
2. A real Git fixture covering adoption, branch separation, rename/path move,
   tombstone reuse, port-key behavior, and old/new JSON.
3. A real non-colocated JJ fixture plus a colocated JJ fixture, covering
   secondary-workspace discovery, canonical native names, no implicit bookmarks,
   and read-only listing.
4. Crash/failure injection around pending creation and atomic replacement,
   including preservation of corrupt/unsupported original bytes.
5. A compatibility fixture suite for v1 `repos.json`, v0.7/default workspaces,
   legacy port keys, and pre-v0.8 structured output.

Run the existing baseline with:

```sh
go test ./...
```

The Lane C baseline passes before identity changes. Do not push or abandon the
JJ change from this work unit.
