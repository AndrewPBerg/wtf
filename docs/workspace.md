# Workspace inspection

`wtf workspace inspect <selector>` shows one managed workspace. The selector may
be its durable WTF workspace UUID, canonical name, or exact path. UUIDs also
resolve removed tombstones; names and paths resolve only claim-retaining records.

`wtf workspace list` lists all durable workspace records in stable UUID order.
`wtf workspace current` resolves the managed workspace containing the current
directory, including from a nested directory. All three commands accept the global
`--json` flag. JSON separates `identity` (the persistent WTF record) from
`physical` (what is currently present on disk), and includes JJ
change/commit/operation data and the JJ Git-diff shadow status when available.

`wtf capabilities --json` advertises the versioned JSON result schemas, supported
VCS backends, resource kinds, and doctor checks so automation does not infer them
from the installed binary version.

These commands are read-only. They do not snapshot a JJ working copy, create
bookmarks, or change graph state.
