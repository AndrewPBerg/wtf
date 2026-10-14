# WTF 1.0.0 documentation

This is the supported 1.0.0 documentation. The CLI registry and generated schema
are authoritative for commands and options. Older architecture pages remain for
history and have a historical banner; they do not describe this release.

## Start here

1. [Install and verify](installation.md).
2. [Create your first workspaces](getting-started.md).
3. [Configure defaults and tasks](configuration.md).
4. [Write hooks](hooks.md) and use the [Lua substrate](lua-substrate.md).
5. [Navigate](list.md), [clean up](remove.md), and [diagnose](doctor.md).

## Reference

- [Complete command reference](cli.md)
- [Setup and teardown](setup.md)
- [New and configuration inheritance](new.md)
- [Workspace messages](change.md)
- [Shell completion](completion.md)
- [Review and CI metadata](metadata.md)
- [TOML editor schema](config-schema.md)
- [Tasks](tasks.md) and [port claims](ports.md)
- [JSON contract](json.md), [state and safety](state.md), [troubleshooting](troubleshooting.md)
- [Migration from 0.x](migration.md), [version](version.md)
- [1.0.0 release notes](releases/1.0.0.md), [release procedure](releasing.md)

## Working examples

[Hello-world creation/list callbacks](../examples/hello-world/README.md) exercise
files, commands, environment settings, and themed messages repeatedly.
[Copy local files on creation](../examples/copy/README.md) demonstrates independent
copies and explicit symlinks. All sample values are mock data. Install examples
explicitly; a fresh setup never registers code automatically.

WTF 1.0.0 supports Linux. It does not provide a daemon, process supervisor,
native Docker provider, remote execution, or automatic dependency installation.
