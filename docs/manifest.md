# `.wtf.toml` manifest

WTF optionally reads a strict, versioned `.wtf.toml` from a project root. If the
file is absent, the existing zero-config behavior is unchanged. This package
only parses resource intent; it does not create, copy, decrypt, or inspect
resources.

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

Unknown keys are rejected at every level. File paths are bounded relative
paths or globs: each slash-delimited segment may use Go `path.Match` syntax
(`*`, `?`, or bracket classes), while glob matches never cross a slash.
Absolute paths, `~` paths, backslashes, empty/`.`/`..` path segments, control
characters, malformed globs, and excessively deep or long values are rejected.
Port declarations are exposed in name order; file declarations retain declaration
order. `secret = true` is metadata only: WTF never reads or returns file contents.

The parser validates bounded glob syntax, but v1 lifecycle reconciliation currently
rejects glob declarations before creating files or resource state. Use literal paths
for managed resources until deterministic expansion semantics are added.
