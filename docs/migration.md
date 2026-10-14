# Migration from 0.x

1.0.0 replaces the older Git/worktree command suite with a JJ-centered lifecycle
and explicit Lua project policy. Back up local configuration and describe/preserve
source changes in JJ before replacing your binary.

- Supported workspace mutations are Linux-only; JJ remains authoritative for
  revisions/bookmarks. Old --vcs Git flows and generated Git shadows are removed.
- `.wtf.toml` and `.wt-forge.toml` are not the 1.0 configuration format. Run
  `wtf setup` and express configuration in `.wtf/config.toml` version 1.
- `new PATH` creates or prepares; there is no public init command. Creation
  defaults to trunk(); use `-f @` for the current working-copy revision.
- `list`/`ls`/`sw`, `remove`/`rm`, and `doctor`/`doc` share canonical behavior.
  Old register/repos/unregister/watch/clean/news/update/uninstall/resource commands
  are absent. Use the [current command reference](cli.md).
- Tasks are explicit TOML argv definitions; project behavior belongs in Lua.
  No dependencies/services start automatically.
- Existing ignored configuration is copied to newly created workspaces, not
  retroactively into existing ones. Local runtime artifacts remain opt-in.
- Global/repository state paths keep their V1 names; canonical identity markers
  live in .jj. Legacy local workspace markers remain readable. This is not a
  general automatic migration of older 0.x resource manifests.

Review [setup](setup.md), [hooks](hooks.md), and [state](state.md). Keep historical
configs until their desired behavior has been reproduced and tested. Git ignore
rules do not remove tracked configuration from history.
