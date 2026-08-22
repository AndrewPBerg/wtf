# wtf cleanup

Cleanup is part of WTF's **Versioned Automation Contract**. `wtf cleanup plan
<workspace-id|name|path> --json` emits a read-only JSON artifact for a managed active
workspace. JSON workspace results use a `version: 1` envelope and expose canonical
WTF repository/workspace IDs. The durable workspace UUID is the preferred selector;
canonical names and paths remain human conveniences.

```bash
wtf cleanup plan 123e4567-e89b-12d3-a456-426614174000 --json > cleanup.json
wtf cleanup apply cleanup.json
```

Applying requires the unchanged returned artifact. WTF rechecks repository/workspace
identity and VCS registration before stopping the recorded server, removing the
workspace, releasing its port, and recording the identity tombstone. A changed or
tampered artifact fails closed.

If physical/VCS removal succeeds but local lifecycle cleanup cannot finish, WTF records
`cleanup_failed` debt instead of silently leaving the workspace active. Structured
inspection exposes that debt, and retrying with the existing UUID selector
completes cleanup when possible. The Git-diff shadow is presentation-only; its health
is observed by workspace inspection and is never refreshed or mutated by inspection.

This surface does not create bookmarks or define Agent Bridge/WorkUnit semantics.
Agent Bridge may call WTF, but WTF never calls or stores Agent Bridge data. Existing
`wtf rm` remains the compatibility-oriented direct removal command.
