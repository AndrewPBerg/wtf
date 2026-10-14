# wtf change

Change a registered workspace's message without repeating initialization.

```sh
wtf change testing-msg -m "Picking this up tomorrow"
wtf change testing-msg -m "" # Clear the message
wtf change -m "Picking this up tomorrow" # Current workspace
```

The optional target is a workspace name or ID in the current repository. Names
and message text preserve their case. Omitting the target selects the current
workspace, including when invoked from a subdirectory. Missing, removed, or
unavailable workspaces cannot be changed.

`--message` / `-m` replaces the whole description; an explicitly empty string
clears it. Without a requested change, the command shows help and runs no hooks.
Normal mutations participate in configured invocation hooks but never run
workspace initialization or creation hooks.

The message also accepts inherited TOML defaults through the common resolver:

```toml
[commands.change]
message = "Picking this up tomorrow"
```

An explicit CLI value takes precedence, including `-m ""`. List and remove show
the saved message below the highlighted row. `--json` / `-j` emits one versioned
result whose `data` is the updated workspace; an empty message is omitted from
that record. Errors use the same versioned envelope. Human output confirms
whether the message was updated or cleared.
