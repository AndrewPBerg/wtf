# Troubleshooting

Start with `wtf doctor -j`. It checks config, required JJ readiness, optional
GitHub/Linear integrations, and retained lifecycle state without executing hooks
or repairing state. Authentication probes can contact providers.

| Symptom | Next step |
|---|---|
| Not registered | Run `wtf new .` in the intended JJ workspace |
| JJ identity error | Configure JJ user name/email, then retry doctor |
| Invalid TOML/Lua | Repair the cited local file; unlisted modules never run |
| Missing hook | Check the explicit module list and relative module path |
| New uses wrong source revision | Use `-f @` or an explicit JJ revision |
| Existing workspace did not receive config | Expected: automatic inheritance applies only to creation |
| Selection does not change directory | Source the Bash wrapper; the executable cannot cd its parent |
| PR/check metadata unavailable | Check GitHub auth/doctor; use --pr=false --checks=false offline |
| Missing workspaces retained | Inspect --all; claims, locks, and access errors prevent pruning |
| Removal blocked | Inspect source changes/integration/ownership; preserve edits before cleanup |
| Setup teardown in JSON fails | Confirmation is terminal-only; no directory was deleted |
| macOS/Windows mutation fails | 1.0.0 supports Linux only |

Failed new/init operations keep workspace identity and completed stages. Repair
the underlying issue and rerun `wtf new PATH`. Configuration transfer errors
retain partial copies; they are not retried by overwriting an existing workspace.
Inspect and repair the selected files explicitly before retrying preparation.
Do not delete `.jj` identity/state to work around a safety guard.
