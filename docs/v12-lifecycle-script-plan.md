# v0.12+ Lifecycle Script Proposal

> **Status:** deferred proposal. v0.11 shipped only typed file/port resources and
> explicitly excluded arbitrary lifecycle hooks. Promote this proposal only after
> repeated dogfood evidence justifies executable project hooks.

## Decision

Do **not** build a WTF task runner, task-file replacement, or generic `wtf task`
CLI. Existing tools already serve that purpose.

The value WTF adds is opt-in project scripts at safe workspace lifecycle points.
A project may declare create/remove scripts, but WTF runs them only when the
caller explicitly asks for lifecycle scripts on that invocation. A manifest alone
never authorizes command execution.

## Boundary

WTF validates declarations, supplies an inspectable workspace context, executes
declared lifecycle scripts in deterministic order, and returns structured/redacted
results. Projects own command contents, Docker/service behavior, database policy,
and package-manager policy.

Still out of scope:

- generic task execution/list/plan commands;
- Docker/service providers;
- database provisioning, stress testing, and destructive-drop policy;
- package-manager installation policy beyond existing setup behavior;
- shell strings, secret interpolation, implicit hooks, or Agent Bridge data.

Projects can call Docker, a database tool, or a task runner from their own explicit
lifecycle script. Repeated evidence—not convenience alone—would justify a native
provider later.

## Manifest extension

If promoted, retain manifest `version = 1` only when the change is strictly additive;
otherwise increment the manifest version. The proposed declarations are:

```toml
[[lifecycle.on_create]]
name = "bootstrap"
command = ["./scripts/workspace-bootstrap"]
cwd = "."

[[lifecycle.on_remove]]
name = "teardown"
command = ["./scripts/workspace-teardown"]
cwd = "."
```

`command` is an argv array, never a shell string. Complex shell logic belongs in a
project-owned script. `cwd` is a bounded relative path. Names are unique within an
entry point and entries execute in declaration order. The manifest carries no
environment values, secret interpolation, or executable discovery.

WTF injects only non-secret metadata into each child: `WTF_REPOSITORY_ID`,
`WTF_WORKSPACE_ID`, `WTF_WORKSPACE_PATH`, and named allocated port metadata. It
never emits resolved env-file contents or command environment values in JSON/log
output.

## Built-in and declared lifecycle

Existing env linking/copying and dependency installation are already native
on-create lifecycle work. They remain built-ins rather than becoming synthetic
scripts: their existing `--no-setup`, `--no-env`, and `--no-install` controls stay
compatible and they keep their current structured behavior.

The create pipeline is identity -> declared resources -> built-in setup -> optional
`on_create` scripts. The remove pipeline is optional `on_remove` scripts -> resource
cleanup -> VCS removal -> identity finalization. This makes generic project scripts
an extension of the lifecycle instead of a replacement for known WTF primitives.

## CLI contract

- `wtf new ... --run-lifecycle-scripts` runs `on_create` after identity, resource,
  and requested built-in setup success. A script failure leaves the workspace intact
  and reports a visible repair boundary.
- `wtf rm ... --run-lifecycle-scripts` runs `on_remove` before destructive resource
  and VCS cleanup. A script failure leaves the workspace/resources intact.
- The existing JSON result surfaces include redacted lifecycle-script status when
  scripts were explicitly requested. Doctor reports incomplete lifecycle cleanup
  as repairable debt.

Without `--run-lifecycle-scripts`, existing `new`/`rm` behavior is unchanged.

## Implementation sequence

1. Extend `internal/config` with strict lifecycle parsing and validation.
2. Add an isolated lifecycle executor: argv execution, bounded cwd, deterministic
   order, cancellation, output redaction, and structured results.
3. Wire opt-in create/remove execution at the stated lifecycle boundaries.
4. Extend JSON/doctor contracts and focused failure/ordering tests.
5. Dogfood scripts on two VCS projects; record whether service/database/package
   manager scripts reveal a concrete native-provider need.
6. Run the full quality gate and decide release readiness without tagging.

## Open evidence questions

- Whether any real project needs a shell wrapper; start argv-only.
- Whether the per-invocation flag is sufficient trust policy or first-use
  acknowledgement is needed.
- Which script results need durable retention versus terminal/JSON reporting only.
