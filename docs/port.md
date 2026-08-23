# wtf port

Show the port allocated to the current workspace.

## Usage

```bash
wtf port
```

Port allocation is workspace-local lifecycle state. The v0.11 resource contract
also exposes named UUID-owned port intent, leases, observed state, and cleanup debt
through `wtf resources --json`.
