> Historical design or 0.x reference. This page is not the WTF 1.0.0 contract.
> Start with the [current documentation](README.md) and [migration guide](migration.md).

# wtf port

Show the port allocated to the current workspace.

## Usage

```bash
wtf port
```

Port allocation is workspace-local lifecycle state. The v0.11 resource contract
also exposes named UUID-owned port intent, leases, observed state, and cleanup debt
through `wtf resources --json`.
