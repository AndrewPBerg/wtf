# wtf cleanup

`wtf cleanup plan <workspace-id|name|path>` emits a read-only JSON artifact for a managed active workspace. The durable workspace UUID is the preferred selector; canonical workspace names and paths are accepted by the existing identity lookup conventions.

```bash
wtf cleanup plan 123e4567-e89b-12d3-a456-426614174000 --json > cleanup.json
wtf cleanup apply cleanup.json
```

Applying requires the unchanged returned artifact. WTF rechecks repository/workspace identity and VCS registration before stopping the recorded server, removing the workspace, releasing its port, and recording the identity tombstone. A changed or tampered artifact fails closed. This surface does not create bookmarks or define Agent Bridge/WorkUnit semantics. Existing `wtf rm` remains the compatibility-oriented direct removal command.
