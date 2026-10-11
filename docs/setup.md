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
Existing ignore rules and other scaffold files are preserved when setup is rerun.

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

Human output lists created and preserved paths. JSON returns one versioned envelope
with `data.path`, `data.created`, and `data.existing`; the lists use paths relative
to the current directory. Help and argument errors follow the shared CLI contract.

After editing your hooks, `wtf new .` registers and prepares a JJ workspace.
See [hooks.md](hooks.md) for the current hook contract.

`wtf setup teardown` removes the entire `.wtf/` directory from the current
directory after a yes/no confirmation. No is selected by default. Use arrow keys
or y/n to select, then Enter to confirm; Esc cancels. All custom files inside
`.wtf/` are included in removal. Workspace registration and shared state remain.
Cleanup works even if the local configuration is invalid and runs no hooks.
Symlink targets are never removed.

JSON and redirected output return a `confirmation_required` error without
deleting anything. A missing `.wtf/` succeeds without prompting. Teardown JSON
includes `data.path` and `data.status` (`confirmation_required`, `cancelled`,
`not_configured`, or `removed`).
