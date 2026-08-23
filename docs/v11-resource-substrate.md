# v0.11 Resource Substrate

> **Status:** released. The typed file/port substrate and one-project dogfood gate
> shipped in v0.11. A second VCS-backed project remains post-release confidence
> work; lifecycle scripts are deferred to the v0.12+ proposal.

## Goal

Make a WTF workspace self-describing and repairable for both people and thin
automation adapters, without adding Agent Bridge coupling, arbitrary lifecycle
hooks, or secret-value handling.

## Command surfaces

### `wtf workspace current --json`

Return the v1 structured report for the workspace containing the current
working directory. It is read-only and includes canonical repository/workspace
IDs, backend, physical state, lifecycle state, and Git-shadow health.

### `wtf capabilities --json`

Advertise the CLI contract rather than requiring callers to infer it from the
installed binary version. Initial fields include result schema versions,
supported VCS backends, supported resource kinds, and supported doctor checks.

### `wtf doctor --json`

Read-only diagnostics for a current workspace or all managed workspaces:

- identity/path/VCS registration disagreement;
- `cleanup_failed` debt;
- missing or stale Git shadows;
- missing, broken, or unexpectedly replaced managed file resources;
- unavailable port/resource leases.

Each finding has a stable code, severity, affected repository/workspace ID, and
an optional explicit repair-plan command. Doctor never repairs automatically.

## Declarative `.wtf.toml`

A versioned, committed project manifest defines only typed resource intent. It
must be small enough to validate strictly and evolve additively.

```toml
version = 1

[workspace]
default_workspace = "warn" # allow | warn | deny

[resources.ports.web]
preferred = 3000

[[resources.files]]
name = "root-env"
source = ".env"
target = ".env"
mode = "symlink" # symlink | copy
secret = true

[[resources.files]]
name = "agent-notes"
source = "docs/agent-context.md"
target = "AGENT_CONTEXT.md"
mode = "symlink"
secret = false
```

File resources are composable declarations, not hard-coded framework behavior:
source and target support a documented, bounded relative-path/glob grammar.
They cover root or nested dotenv files, SOPS/SecretSpec-adjacent encrypted files,
Markdown, configuration files, and other project files without revealing file
contents. `secret = true` affects diagnostics and output: WTF reports metadata
and paths only, never values.

The initial resource kinds are `ports` and `files`. Database support begins only
as a typed, plan/apply provider after a concrete repeated conflict; it does not
begin as a generic shell-hook escape hatch.

## Resource lifecycle

`wtf resources --json` lists resources keyed by canonical workspace UUID:

- desired declaration;
- observed state;
- ownership/lease state;
- safe apply/remove/repair actions;
- visible cleanup debt.

Creation and removal reconcile declared resources through deterministic
plan/apply operations. Failed removal becomes repairable debt. Existing
zero-config env discovery remains compatible; a manifest augments or explicitly
replaces it per resource, never silently changes it.

## Non-goals

- Arbitrary `on_create`/`on_remove` shell hooks.
- Secret parsing, decryption, logging, copying into command output, or secret
  storage in WTF state.
- Agent Bridge WorkUnits, actors, policy, or orchestration.
- JJ graph mutations, bookmark creation, integration, or publication.

## Done criteria

- `workspace current`, `capabilities`, `doctor`, and `resources` have versioned
  JSON contracts and focused tests.
- Doctor findings and repair plans are deterministic and read-only by default.
- File and port resources are UUID-owned, safely reconciled, and visible after
  partial failure.
- `.wtf.toml` supports composable file source/target/mode declarations,
  including non-secret Markdown symlinks and metadata-only secret files.
- One release-gate project dogfoods nested env/config paths, Markdown links,
  named ports, and at least one repairable resource failure; a second project is
  tracked as post-release confidence work.
