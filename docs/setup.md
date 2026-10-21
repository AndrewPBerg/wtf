# wtf setup

Create local WTF configuration in the current directory:

```sh
wtf setup
wtf setup --json
wtf setup teardown
```

No repository or workspace registration is required. Run this at the project root
where you want the configuration to live.

```text
.wtf/
├── .gitignore
├── AGENTS.md
├── .agents-template.sha256
├── schema.json
├── config.toml
├── hooks/
└── tasks/
```

The generated `.wtf/.gitignore` contains only:

```gitignore
*
```

Everything inside `.wtf`, including its ignore file, stays local and untracked.
Git ignore rules do not untrack files already committed to a repository.
Existing ignore rules, config, and schema files are preserved when setup is rerun.

`.wtf/AGENTS.md` is a self-contained agent reference covering workspace
isolation, configuration, CLI/JSON, hook phases and overrides, all public Lua
namespaces, tasks/stages, port claims, cleanup, and recovery. Point agents at it
from your project instructions. It is generated from the
[embedded guide](../internal/core/scaffold/AGENTS.md), refreshed safely on reruns, and copied
independently into newly created workspaces before callbacks. Preparing an
existing workspace preserves its guide. The guide is local and ignored along
with the rest of `.wtf/`; point agents there explicitly when working on configuration.

## Updating the agent guide

After updating the WTF executable, rerun `wtf setup` in the desired workspace.
The installed binary supplies the template; setup makes no network request.
The `.agents-template.sha256` file records the last installed template's SHA-256.

| Existing guide | Result |
| --- | --- |
| Missing | Create latest installed template and checksum |
| Matches installed template | Preserve; enroll if checksum was missing |
| Matches previously installed checksum | Replace with installed template; update checksum |
| Locally edited or unknown older template | Preserve; report update available |

Matching the last installed checksum proves the generated guide was not edited.
Checking only against the latest template would not provide that distinction.
Checksum records copy into new workspaces alongside the guide; setup refreshes
each existing workspace individually. Preparing an existing workspace with `new`
does not update the guide. Keep project-specific instructions in the project's
root AGENTS.md so the generated reference can update cleanly.

Unknown legacy guides without a checksum are adopted only if they exactly match
the installed template. Otherwise they remain preserved. To deliberately replace
a customized/unknown guide, save its edits elsewhere, remove only `.wtf/AGENTS.md`,
and rerun setup. No force flag or automatic merge is provided.

Invalid checksum contents fail with `invalid_setup_state` and preserve the guide.
Restore the checksum record or remove it to treat the guide as untracked. Guide
and checksum publish as separate atomic writes; rerunning with the same binary
recovers an interruption between them. Refreshes serialize through the local
`.setup.lock`, which is not inherited. Neither setup nor the checksum tracks
live edits made by external editors as a transaction.

## Initial configuration

The initial configuration selects the local generated schema:

```toml
#:schema ./schema.json

version = 1
```

`version` identifies the configuration format. The generated schema supplies
editor completion, descriptions, and validation for the installed command tree;
it is not a replacement for runtime validation. See [config-schema.md](config-schema.md).

Both `hooks/` and `tasks/` start completely empty. Setup creates no Lua files and
selects no entrypoint. Setup introduces no task-file loader or new task execution
contract.

Setup creates missing files and directories and reports existing ones as preserved.
It never overwrites project edits, executes Lua, installs dependencies, or registers
a workspace. Conflicting files, directories, or symlinks produce an error. If a run
fails, completed files remain available and rerunning setup fills in missing files.

Human output lists created, preserved, updated, and available-update paths. JSON returns one versioned envelope
with `data.path`, `data.created`, `data.existing`, `data.updated`, and
`data.update_available`; the lists use paths relative
to the current directory. A preserved guide with an available update is a success,
not an error. Help and argument errors follow the shared CLI contract.

After editing your hooks, `wtf new .` registers and prepares a JJ workspace.
See [hooks.md](hooks.md) for the current hook contract.

`wtf setup teardown` removes the entire `.wtf/` directory from the current
directory after a yes/no confirmation. No is selected by default. Use arrow keys
or y/n to select, then Enter to confirm; Esc cancels. All custom files inside
`.wtf/` are included in removal. Workspace registration and shared state remain.
Cleanup works even if the local configuration is invalid and runs no hooks.
Symlink targets are never removed. The agent guide is removed with `.wtf/`.

JSON and redirected output return a `confirmation_required` error without
deleting anything. A missing `.wtf/` succeeds without prompting. Teardown JSON
includes `data.path` and `data.status` (`confirmation_required`, `cancelled`,
`not_configured`, or `removed`).
