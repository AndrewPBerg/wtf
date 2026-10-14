# JSON automation contract

Every command/group/alias supports `-j`/`--json`, including help and argument
errors. Stdout contains exactly one JSON object with:

```json
{"version":1,"command":"wtf version","ok":true,"data":{"version":"1.0.0"}}
```

`version` is the envelope format, not the executable version. `command` is the
canonical command even when an alias/mixed-case spelling was used. `ok` is a
boolean; `data` contains command results. Failures include an `error` object with
`code` and `message`; partial lifecycle data can remain available. Exit status is
nonzero on failure. Data shapes are described by each command's reference/help.

```sh
wtf SW -j --help
wtf new feature-a -f @ -j
wtf remove -j
```

Logs, Lua notifications, and child output go to stderr without terminal styling
in JSON mode. JSON never prompts. Bare removal reports candidates without
deleting; setup teardown requires terminal confirmation and returns
`confirmation_required` when a configured directory exists. No implicit
confirmation flag is available. Help and inspection never invoke callbacks.
