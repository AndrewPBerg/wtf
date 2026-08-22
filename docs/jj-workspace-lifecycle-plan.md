# JJ-native workspace lifecycle plan

> **Status:** exploratory design for dogfooding; not a committed CLI contract.
>
> This document is intentionally broader than one command. It describes how WTF
> could make isolated jj workspaces feel cheap in a Zed-first development workflow,
> then provide safe higher-level operations for keeping or discarding their work.

## North star

Creating an isolated workspace should feel only one or two steps more expensive
than continuing in the default workspace. Isolation may consume more CPU, disk,
ports, and database resources, but it should not add meaningful mental overhead.

The normal decision should be:

```text
Is this separate work?
  no  -> continue in the default workspace
  yes -> create an isolated workspace
```

Afterward, its lifecycle should be:

```text
Create -> Work -> Gather or Discard -> Verify -> Clean up
```

WTF owns workspace and resource lifecycle. jj remains authoritative for changes.
Zed remains the primary editor. Git exists at the forge boundary and as a
read-only compatibility surface for Zed, not as the local workflow model.

## Design principles

1. **JJ-native, Git-compatible.** New lifecycle features may ship for jj first.
   Do not force misleading Git parity where Git has different semantics.
2. **Zed-first without a Zed fork.** Work around Zed's Git-first source-control
   model with managed metadata and ordinary directories.
3. **Zero-config remains useful.** A project manifest augments detection and
   defaults; it does not make simple repositories harder to use.
4. **One explicit isolation decision.** Avoid requiring users to separately
   reason about env files, ports, databases, install steps, and editor setup.
5. **Safe automation boundaries.** Planning, mutation, verification, and cleanup
   are distinct. Destructive cleanup is never implied by a successful rewrite.
6. **CLI is the durable interface.** Pi and other agents should call structured
   WTF operations rather than reconstructing sequences of `jj` commands.
7. **No secrets in project configuration or output.** Configuration may name
   env files and variables, but must not contain or print their values.
8. **Observed friction drives scope.** Dogfood each layer before adding the next.

## Scope

This plan covers:

- Zed source-control visibility for secondary jj workspaces
- declarative per-project workspace setup
- env-file symlink and copy policies
- stable allocation and cleanup of named ports
- isolated databases and other project-defined resources
- a higher-level `gather` operation for keeping workspace work
- safe agent use through Pi and Agent Bridge

It does not initially cover:

- a Zed fork or native Zed jj implementation
- replacing jj's operation log or conflict model
- hiding the actual jj graph from advanced users
- a general container orchestrator
- automatic publication, bookmark creation, or PR merging
- exact feature parity between jj and Git backends

## Existing foundation

WTF already has useful pieces of this design:

- jj workspaces behind the normal `new`, `sw`, `rm`, and `clean` commands
- workspace setup with env-file symlinking or `--copy-env`
- package-manager detection and installation
- stable per-workspace port allocation
- shared per-repository state under `.jj/repo/wtf/`
- an experimental Git-diff shadow for Git-aware editors such as Zed

The plan should extend these pieces rather than replacing them.

## Workspace modes

Start with two user-facing modes, not a large profile system.

### Default workspace

The long-lived workspace used for small or continuous work. It may share local
services and symlink env files. WTF should not require it to be provisioned again
for every task.

### Isolated workspace

A workspace for a separate feature, PR, review, experiment, or agent task. Creation
performs the project's declared setup as one operation:

- create the jj workspace
- initialize Zed Git-diff metadata when enabled
- apply env-file policy
- install dependencies when needed
- allocate named ports
- provision an isolated database when configured
- write non-secret workspace metadata
- optionally open the directory in Zed

A spike is an isolated workspace with disposable intent, not necessarily a third
provisioning mode. The decision to gather or discard it comes later.

Possible CLI shape, subject to dogfooding:

```bash
wtf new feat/auth --isolated
wtf new spike/cache --isolated --open
```

A project may eventually make isolation its default so the flag is unnecessary.

## Zed compatibility layer

Secondary jj workspaces are ordinary directories but normally lack `.git`, so Zed
cannot provide its normal source-control diff. WTF's experimental Git-diff shadow
is the preferred workaround:

- jj remains the only VCS allowed to mutate history
- the shadow Git repository borrows jj's object database
- its index represents the selected jj baseline
- it has no publishing remote
- WTF marks it so repository detection still chooses jj
- Zed's Git UI is treated as a read-only diff viewer

WTF should refresh this metadata automatically after graph mutations performed by
WTF, including a successful gather. It cannot intercept arbitrary `jj` commands,
so `wtf git-diff` remains the explicit repair/refresh operation. Status output may
later detect and explain a stale editor baseline.

Potential ergonomic additions:

```bash
wtf open feat/auth             # open the workspace using the configured editor
wtf git-diff                   # refresh Zed's baseline in the current workspace
```

Do not build deeper Zed integration until the shadow approach has been used on real
work long enough to identify a concrete missing capability.

## Project manifest

Introduce an optional, versioned project manifest after settling its minimum
requirements. The working name is `.wtf.toml`; the final name and schema remain open.

The manifest describes intent. It should not duplicate values WTF can reliably
detect, and it must never contain secrets.

Illustrative shape—not a final schema:

```toml
version = 1
default_mode = "isolated"

[vcs]
backend = "jj"

[editor]
command = "zed"
git_diff = true

[profiles.default]
env = "symlink"
install = false

[profiles.isolated]
env = "copy"
install = true

[ports.web]
base = 8000

[ports.frontend]
base = 5173

[database]
provider = "command"
create = "./scripts/wtf-db create"
drop = "./scripts/wtf-db drop"

[verify]
commands = ["task test"]
```

Questions to answer through dogfooding:

- Should the repository commit `.wtf.toml`, or should it support an ignored local
  override such as `.wtf.local.toml`?
- Which settings describe a team workflow versus one developer's machine?
- Should profiles stay internal, with only `default` and `isolated` exposed?
- How are project commands trusted and confirmed on first use?

## Environment model

Environment handling needs two separate concepts.

### Source files

Existing env files may be:

- **symlinked** for a trusted, shared local environment
- **copied** so an agent or isolated workspace cannot mutate the source file
- **omitted** when setup should not provide them

The manifest should eventually support explicit file rules and nested env files,
while auto-discovery remains the zero-config fallback.

### Workspace-generated values

Ports, database names, and similar allocations differ by workspace. They should not
be written back into a shared symlink. WTF needs a generated overlay or process env
contract for values such as:

```text
PORT
FRONTEND_PORT
DATABASE_URL or a project-specific database identifier
WTF_WORKSPACE
WTF_WORKSPACE_ID
```

The exact delivery mechanism needs a project spike. Candidates are:

1. a generated ignored env overlay loaded by the application;
2. environment injection through `wtf run` and WTF-managed dev servers;
3. a project hook that writes framework-specific local configuration.

Prefer a visible, inspectable mechanism over shell magic. Never log resolved secret
values while generating or verifying the overlay.

## Stateful resource allocation

Resources are leases owned by a workspace identity, not incidental setup output.
The registry should track at least:

- repository identity
- workspace name and stable workspace/change identity where available
- workspace path
- named port allocations
- provisioned database identifier
- creation time and lifecycle state
- cleanup status and last error

For jj repositories, state belongs under `.jj/repo/wtf/` so every workspace shares
one registry. Git repositories can continue using their common Git directory.

### Storage choice

Use the smallest reliable store:

1. keep JSON initially;
2. add file locking and atomic replacement before adding more writers;
3. move to SQLite only when concurrent updates, querying, or recovery requirements
   demonstrate that JSON is no longer sufficient.

The storage format is internal. The CLI's `--json` output is the automation contract.

### Ports

Extend the current allocator from one detected port to multiple named ports per
workspace. Allocation should be stable across restarts, collision-checked, shown by
`wtf sw`/`wtf status`, and released during successful cleanup.

```bash
wtf port                   # primary/default port
wtf port web
wtf port frontend
wtf resources              # all leases for the current workspace
```

### Databases

Start with project-defined commands rather than embedding Django, PostgreSQL, or
other framework-specific policy in WTF. WTF supplies safe identifiers and allocated
values; the project owns provisioning mechanics.

Required lifecycle:

```text
allocate name -> create -> expose to workspace -> verify -> use -> drop -> release
```

Database deletion must be explicit and must verify that the target identifier is
owned by the workspace being removed. A failed drop leaves a visible cleanup debt;
it must not silently release the registry entry and orphan the database.

## Gather: keep useful workspace work

`gather` is the proposed high-level operation for incorporating useful work from a
temporary workspace. It describes intent better than "merge workspaces," because a
jj implementation may squash, rebase, or preserve a stack rather than create a
merge commit.

Possible interactive shape:

```bash
wtf gather spike/cache --into default
```

Possible explicit/agent shape:

```bash
wtf gather spike/cache --into default --strategy squash --dry-run --json
wtf gather spike/cache --into default --strategy squash --yes --json
```

Initial strategies:

- **squash** — collect the source work into one focused change; likely default for
  a spike or small feature
- **preserve** — retain the source's logical change stack while integrating it with
  the destination line of work

Do not add a generic `merge` strategy until a real workflow needs a multi-parent jj
change and its Zed shadow limitation is understood.

### Gather planning

The first implementation should be read-only. It resolves workspace names to stable
jj change IDs and reports:

- source and destination workspace/change identities
- source change range or stack
- proposed graph transformation
- whether either workspace has additional descendants or conflicts
- whether either workspace appears active
- expected Zed Git-diff refresh
- verification commands that would run
- resources that would remain or become eligible for cleanup

The plan must be machine-readable and usable as the input to a later apply step.
Apply should fail if relevant identities or graph assumptions changed after planning.

### Gather apply

A mutating gather should:

1. resolve and validate the saved plan;
2. refuse or require confirmation when source/destination agents are active;
3. record the current jj operation ID and a WTF checkpoint;
4. perform one documented jj graph transformation;
5. surface conflicts without pretending the operation completed cleanly;
6. run configured verification;
7. refresh Zed Git-diff metadata for affected workspaces;
8. report the exact resulting change IDs and recovery command;
9. leave source-workspace cleanup as a separate decision.

A successful gather must not automatically remove the source workspace in the first
version:

```bash
wtf gather spike/cache --into default --strategy squash
wtf rm spike/cache
```

An optional combined cleanup flow can be considered after repeated dogfooding.

### Discard

Discarding change history and removing workspace resources are also separate actions.
WTF should make both easy but never conflate them:

```text
abandon source changes -> verify graph -> remove workspace -> drop resources
```

JJ's operation log provides recovery for graph mutations. It does not recover a
dropped database or deleted untracked workspace files, so cleanup needs a stronger
confirmation boundary than abandon/rewrite operations.

## Pi and Agent Bridge integration

WTF should remain independently useful from a human-operated terminal. AI support
sits above its structured CLI rather than inside its core semantics.

### Pi extension

A thin Pi extension can provide typed tools for:

- creating an isolated workspace
- inspecting workspace/resources status
- planning and applying gather
- running verification
- cleaning up after confirmation

The tool should call WTF and consume `--json`; it should not emit an improvised shell
sequence of low-level jj commands. Human and agent paths must share the same safety
checks.

### Agent Bridge

Before gather, discard, or cleanup, integrations can ask Agent Bridge whether another
agent is active in the source or destination workspace. Agent Bridge supplies:

- live coordination and ownership negotiation
- workspace/repository-scoped identity
- provenance and checkpoints
- collision handling for agents sharing a physical workspace

WTF remains responsible for filesystem, VCS, and resource correctness if Agent
Bridge is absent. Separate workspaces should remain the default agent isolation
model; shared physical workspaces are an intentional advanced mode.

## Git compatibility policy

Existing Git worktree workflows must continue to work. New generic setup and
resource features should work with both backends when their semantics align.

`gather` should be explicitly jj-only at first. A future Git implementation should
be based on an observed Git use case, not on forcing branches and commits to imitate
jj changes. Unsupported backend errors should explain the limitation clearly.

## Incremental dogfood plan

Change one concept at a time and use it during both 9–5 and personal work before
expanding scope.

### Phase 1: Zed shadow hardening

- use the experimental Git-diff shadow on real jj workspaces
- record stale-baseline and unsafe-Zed-action friction
- decide whether manual `wtf git-diff` is sufficient
- do not add a deeper Zed integration without repeated evidence

**Exit evidence:** ordinary review and editing in Zed no longer pushes the workflow
back to Git worktrees.

### Phase 2: workspace manifest and env policy

- document the smallest `.wtf.toml` needed by one Django and one frontend project
- preserve zero-config defaults
- separate source env-file policy from generated workspace values
- verify no secret values enter config, logs, or JSON output

**Exit evidence:** a new isolated workspace starts with correct dependencies and env
inputs without manual file repair.

### Phase 3: named resource registry

- harden the JSON store with locking and atomic writes
- support multiple named ports
- show leases and cleanup debt clearly
- avoid SQLite until a measured failure requires it

**Exit evidence:** concurrently active Django/frontend workspaces never require the
user to remember or manually negotiate ports.

### Phase 4: isolated database hook

- prototype one work project and one personal project
- make create/drop idempotent and ownership-checked
- retain failed cleanup state for repair
- measure startup, disk, and teardown cost

**Exit evidence:** choosing isolation provisions a usable database without additional
mental bookkeeping.

### Phase 5: gather planning

- implement only `--dry-run --json`
- test single change, stacked changes, conflicts, stale workspaces, and active agents
- compare the explanation with the low-level jj commands an expert would choose

**Exit evidence:** the plan is predictable enough that the user trusts it before
seeing the underlying commands.

### Phase 6: gather apply and verification

- apply one strategy, likely `squash`, before adding `preserve`
- make graph preconditions and recovery explicit
- refresh Zed metadata automatically
- keep cleanup separate

**Exit evidence:** useful spike work can be incorporated and verified without manual
JJ graph manipulation, while recovery remains straightforward.

### Phase 7: Pi/Agent Bridge tools

- expose already-proven WTF operations as typed tools
- add active-agent gates and durable checkpoints
- dogfood one agent per jj workspace first
- test shared-workspace collision behavior separately

**Exit evidence:** agents reduce keystrokes and coordination cost without bypassing
human-visible WTF plans or cleanup boundaries.

## Dogfood notes

Keep a short local journal rather than expanding this proposal after every isolated
incident. For each real workspace, record:

```text
project type:
workspace intent: feature / PR / review / spike / agent
creation steps and elapsed time:
manual repairs:
ports/database conflicts:
Zed friction:
gather/discard outcome:
cleanup debt:
candidate improvement:
```

Promote behavior into this plan only after it repeats or reveals a serious safety
problem. Keep employer-sensitive project details out of a public repository.

## Success criteria

The direction is working when:

- creating and entering an isolated workspace is one deliberate action
- Zed provides useful diffs without becoming the VCS authority
- env, ports, and database state require no memorized manual coordination
- `wtf resources` can explain what each workspace owns
- gather plans are understandable before mutation and reproducible through JSON
- cleanup either completes or leaves actionable, visible debt
- agents use the same high-level operations and safety boundaries as humans
- the default workspace remains simple for work that does not need isolation

## Open decisions

1. Is `.wtf.toml` the correct committed manifest name, and is a local override needed?
2. Should `wtf new` default to isolated mode per project, or should isolation always
   be explicit?
3. What is the least surprising generated-env mechanism for Django and frontend
   frameworks?
4. Should editor opening be part of `new`, a flag, or a separate `open` command?
5. Does the Git-diff shadow need stale-baseline detection before broader dogfooding?
6. What exact jj graph should `squash` produce when source or destination contains a
   stack rather than one change?
7. Should gather target a workspace, a change ID, or accept both while resolving to
   immutable identities during planning?
8. Which verification failures block gather completion versus merely block cleanup?
9. When do concurrent resource updates justify replacing locked JSON with SQLite?
10. Which Agent Bridge activity states should block, warn, or require confirmation?
