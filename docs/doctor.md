# wtf doctor

Diagnose managed workspace state without repairing it.

## Usage

```bash
wtf doctor
wtf doctor --json                 # all managed workspaces
wtf doctor <workspace-id> --json  # one workspace
```

Findings use stable codes and severities and identify the affected repository and
workspace UUIDs. Checks cover identity and VCS registration, cleanup debt, JJ Git
shadows, managed files, and port leases. Repair commands are suggestions only;
`doctor` never changes workspace state.
