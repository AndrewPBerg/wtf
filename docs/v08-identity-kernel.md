# v0.8 identity kernel work unit

> WorkUnit tracking UUID: `6d77417a-6754-44a2-851c-739faed74cd5`
>
> Agent Bridge owns WorkUnits and orchestration. WTF owns only deterministic local
> repository/workspace identity and lifecycle state.

## Objective

Give every WTF-managed repository and workspace a canonical UUID, enforce globally
unique active workspace names on one machine, and expose stable identity through
existing structured WTF operations.

## Decisions

- Canonical IDs are lowercase RFC 4122 UUID strings.
- Repository and workspace IDs are allocated randomly, persisted, and never reused.
- Removed workspaces remain as tombstones.
- Active workspace names are globally unique across one local WTF installation.
- Canonical names use `<repository-slug>/<workspace-slug>`, for example
  `wtf/v08-identity`.
- JJ's native workspace name matches the canonical WTF name.
- A Git branch is not a workspace name. Git branches remain forge-origin refs.
- The store is versioned JSON protected by a cross-process lock and updated through
  same-directory atomic replacement.
- Existing state is adopted incrementally. New creation is strict immediately.
- There is no permanent migration command family in this slice.

## Boundaries

WTF stores repositories, physical workspaces, lifecycle state, and resource
ownership. It does not store WorkUnits, actors, participants, checkpoints,
integration policy, Linear data, or Agent Bridge state.

Agent Bridge may store:

```text
WorkUnit UUID -> WTF workspace UUIDs
```

WTF never calls Agent Bridge.

## Canonical data model

```text
repository {
  id
  locator
  lifecycle_state
  created_at
  updated_at
}

workspace {
  id
  repository_id
  name
  backend
  native_name
  path
  lifecycle_state
  created_at
  updated_at
  removed_at
}
```

Initial lifecycle values:

```text
pending
active
removed
cleanup_failed
```

Strict invariants:

- every ID parses as canonical lowercase UUID text;
- repository and workspace IDs are globally unique and never recycled;
- active names are globally unique;
- active canonical paths are globally unique;
- every workspace references an existing repository;
- rename or path movement preserves workspace ID;
- removal preserves a tombstone;
- reusing a removed name allocates a new workspace ID.

## Storage

Global authoritative state:

```text
~/.wtf/state.json
~/.wtf/state.lock
```

`WTF_HOME` continues to override `~/.wtf` for tests.

Repository marker:

```text
<backend state dir>/repository-id
```

For JJ this is beneath `.jj/repo/wtf/`; for Git it is beneath the common Git
metadata directory's WTF state directory.

A mutation must:

1. acquire the cross-process lock;
2. read and strictly validate the complete state;
3. apply exactly one lifecycle transition;
4. write a temporary file in the same directory;
5. flush and atomically replace the state file;
6. release the lock.

Corrupt or unsupported state fails closed and preserves the original bytes.
Concurrent attempts to claim one name must produce exactly one winner.

VCS/filesystem creation cannot be inside the JSON transaction. Creation therefore
uses `pending`, performs the physical operation, then transitions to `active`.
Failures either roll back safely or leave visible repairable state; they must not
silently leak an active name or resource.

## Naming

Canonical names are normalized lowercase strings with exactly one repository scope
prefix and one or more workspace path segments. Reject empty segments, `.`/`..`,
absolute paths, control characters, and forms that normalize ambiguously.

Examples:

```text
wtf/default
wtf/v08-identity
alita-core/feature/auth-refresh
```

Existing workspaces receive identity through adoption. Nonconforming or colliding
legacy names remain visible, but identity-dependent mutations return a deterministic
rename requirement instead of guessing or silently renaming JJ.

For JJ, applying an approved rename changes the native workspace name. For Git, WTF
assigns a canonical workspace name without renaming the Git branch.

## Compatibility

- Preserve existing human fields such as `branch`, `path`, `head`, and `change_id`
  during the transition.
- Add `repository_id`, `workspace_id`, `name`, and `native_name` to structured
  workspace results.
- Names remain human selectors only when unambiguous.
- Automation selects by UUID.
- Existing `repos.json` remains readable while repository identity is adopted.
- Existing port keys may be remapped only when an adopted workspace is explicitly
  renamed; the broader resource-store redesign is a later WorkUnit.

## Parallel implementation lanes

### Lane A: identity store

Own `internal/identity/**` and its tests:

- UUID generation/validation
- state schema and validation
- lock and atomic persistence
- repository/workspace lifecycle transitions
- name and path uniqueness
- tombstones and reuse behavior
- corruption and concurrency tests

### Lane B: workspace model

Own VCS/backend model changes and focused tests:

- separate canonical workspace name from Git branch/bookmark metadata
- represent repository/workspace IDs without deriving them from mutable paths
- JJ native-name behavior
- Git branch compatibility
- minimize call-site breakage and preserve current JSON fields

### Lane C: independent acceptance design

Build an implementation-independent test matrix and, where it can compile without
inventing production APIs, tests for:

- concurrent duplicate-name claims
- canonical UUID rejection
- rename/move stability
- tombstone/name reuse
- corrupt state
- JJ native naming and Git branch separation
- backward-compatible JSON fields

Do not weaken assertions to match implementation shortcuts.

A second wave integrates these lanes into `new`, `sw`, `rm`, global discovery, and
JSON output after the store/model contracts are reviewed together.

## Non-goals

- `.wtf.toml`
- general port/resource migration
- database hooks
- WorkUnit storage
- Agent Bridge orchestration
- JJ graph integration/gather
- Charm removal
- cross-machine name coordination

## Done criteria

- two concurrent creators cannot claim one active name;
- all persisted IDs are canonical and never reused;
- rename and path movement preserve workspace ID;
- removed IDs remain tombstoned;
- name reuse receives a new ID;
- JJ native names follow canonical WTF names;
- Git branches remain separate from workspace names;
- existing workspaces can be adopted without silent VCS mutation;
- existing JSON fields remain compatible and canonical IDs are added;
- corruption fails closed;
- full Go tests, lint, build, and focused race/concurrency tests pass.
