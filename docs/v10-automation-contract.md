# v0.10 Versioned Automation Contract

> WorkUnit: `ec920c96-e141-428c-95c5-ed969a7cf43e`

## Objective

Turn the v0.9 structured workspace substrate into a small, versioned CLI/JSON
contract that scripts, the Pi extension, and Agent Bridge can consume. WTF stays
independent: callers invoke WTF; WTF never invokes or stores Agent Bridge data.

## Scope

### 1. Versioned structured workspace results

`wtf workspace inspect --json` and `wtf workspace list --json` return a top-level
`version: 1` field. Every workspace result retains its existing `identity` object,
whose `id` and `repository_id` are the canonical WTF workspace and repository IDs.
Existing result fields remain compatible.

### 2. Git-diff shadow health

For JJ workspaces, `git_diff_shadow.status` distinguishes:

- `not_supported` — non-JJ workspace;
- `absent` — no private shadow metadata exists;
- `present` — a shadow exists and its Git HEAD matches the single JJ parent used as
  the editor-diff baseline;
- `stale` — a shadow exists but its HEAD is missing or differs from that JJ baseline;
- `unavailable` — shadow health could not be read; `error` explains why.

This is observation only. Inspection never refreshes or mutates a shadow.

### 3. Repairable cleanup debt

If planned cleanup removes the physical/VCS workspace but cannot complete local
lifecycle cleanup, the workspace is recorded as `cleanup_failed` rather than being
silently left active. Structured inspection exposes that repairable lifecycle state.
A retry through the existing UUID selector completes the normal cleanup path when
possible.

### 4. Pi extension contract coverage

The Pi extension remains a thin policy adapter. Its tests document that it directs
agents to `wtf new ... --copy-env` and does not call Agent Bridge or reproduce WTF
workspace lifecycle logic.

## Non-goals

- WorkUnits, actors, checkpoints, collisions, or Agent Bridge RPC inside WTF.
- JJ graph operations, bookmark creation, integration, publication, or verification policy.
- New resource/database hooks, `.wtf.toml`, picker simplification, or forge work.
- A general in-process Go API; the stable surface is CLI JSON.

## Acceptance criteria

1. Workspace inspect/list JSON envelopes include `version: 1` and tests lock it.
2. JSON inspection reports all shadow-health states without mutation.
3. Cleanup partial failure is visible as repairable `cleanup_failed` debt, never a
   falsely active or silently removed identity.
4. Existing ID selectors remain the automation path; names and paths remain human
   conveniences.
5. Targeted Go and Pi-extension tests pass.
6. README/roadmap describe the one-way adapter dependency and the stable contract.

## Hardening follow-up

> WorkUnit: `ff1c46e7-f557-448d-9400-735f68d64fe8`

The automation paths are explicit and fail closed:

- `new --ensure` is the only idempotent create path. Ordinary `new` retains its
  duplicate-name error. An exact active identity returns its existing UUID and
  performs no VCS or setup work; a conflicting identity is deterministic failure.
- `workspace inspect` remains read-only and accepts UUID, canonical name, or path.
- Cleanup plans are content-addressed. Replanning an unchanged target produces the
  same `plan_id`; changed identity/VCS observations invalidate the artifact.
- Applying a valid plan twice returns a versioned JSON success with `noop: true` on
  the second call. Altered or stale artifacts still fail closed.
- A `cleanup_failed` workspace with no checkout can be retried by UUID. A removed
  UUID is a versioned JSON no-op on repeat.

Focused CLI tests cover these five properties. Git-shadow status coverage, the thin
Pi-extension boundary, and JJ dogfooding remain separate release work.

## Work lanes

- **A — Shadow health:** `internal/cli/workspace.go` and focused tests.
- **B — Cleanup debt:** removal/identity lifecycle path and focused tests.
- **C — Contract docs/extension:** roadmap, command docs, Pi extension tests/README.

After the lanes land, run the narrow Go package suite and Pi-extension tests; do
not broaden v0.10 into an orchestration release.
