# Resources and diagnostics

`wtf resources [workspace-id] --json` reports the v1 desired, observed, lease,
lifecycle, and cleanup-debt state for a managed workspace. With no selector, it
uses the workspace containing the current directory. It is read-only and never
allocates ports, creates registry files, or prints file contents.

`wtf doctor [workspace-id] --json` reports deterministic v1 findings for managed
workspaces: identity/VCS disagreement, cleanup debt, Git-shadow health, managed
file drift, and unavailable port leases. Findings contain a stable code, severity,
canonical repository/workspace UUIDs, a message, and a repair command only when a
safe UUID-based repair exists. Doctor never repairs automatically.

File resources use a strict optional `.wtf.toml` manifest. During workspace
creation, WTF records UUID-owned resource metadata, creates only absent targets,
and refuses to overwrite drifted or unowned targets. On removal it verifies that a
target is still owned before deleting it. A failed removal becomes visible,
repairable cleanup debt; retry removal by exact workspace UUID after repairing the
target.
