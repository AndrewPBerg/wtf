# wtf doctor

Check configuration, CLI prerequisites and workspace health in one command,
without repairing anything. Configuration validation is an internal check;
there is no separate `validate` CLI command.

## Usage

```bash
wtf doctor
wtf doc
wtf doctor --json
wtf doctor --here -j
```

`doc` is an alias for `doctor`. Both accept the same flags and report the same
canonical command in JSON.

Doctor checks:

- **Configuration:** strict TOML settings and the selected Lua entrypoint's syntax,
  without executing project code. Broken inherited command defaults are included
  in this check and do not prevent the other diagnostics from running.
  Missing `.wtf` directories and missing `.wtf/config.toml` files report distinct
  codes with `not_configured` status and "No configuration found for hooks".
  A valid TOML file without a Lua entrypoint reports that no Lua hooks are
  configured. Missing hook configuration is informational; malformed TOML or
  Lua fails. No case claims Lua syntax was checked unless an entrypoint exists.
- **JJ (required):** `jj` is available on PATH and executable, and effective
  `user.name` and `user.email` are nonempty in the current directory. Missing
  identity settings include `jj config set --user` suggestions.
- **GitHub CLI (optional):** `gh` is installed and
  `gh auth status --active --hostname github.com` succeeds. This enables the
  default GitHub PR and check metadata features. Missing installation or failed
  authentication checks include installation/login suggestions. Failure can
  also mean a network problem or an incompatible CLI; doctor does not claim
  every failure means the user is logged out.
- **Linear reviews (optional):** the embedded adapter checks Node.js, the selected
  Codex MCP connection, review tools and authenticated Linear workspace. The
  existing Alita profile is selected under `~/Desktop/Work`; other repositories
  use Cascading Labs. A verified account enables Linear-first review links.
  Unavailable connections or account mismatches keep GitHub links available.
- **Workspace health:** current registration and owned-workspace identity,
  retained stage results, claims and recorded failures.

Each area reports independently, so invalid configuration or missing prerequisites
do not hide the remaining checks. Doctor also works before registration: a healthy
JJ workspace reports `unregistered` with a `wtf new .` suggestion, without creating
registry state or failing solely because it is unregistered. Configuration checks
still run when JJ is unavailable, using the nearest workspace root on disk.

JJ and GitHub subprocess checks have a five-second timeout. The Linear probe has
an eight-second overall deadline. Checks run without interactive input and keep
child output private. Authentication may contact GitHub and Linear. Doctor never
installs tools, logs in, runs hooks, registers workspaces, snapshots JJ state or
starts services. Suggestions are for the
user to run separately. GitHub checks resolve an already installed Mise tool
when available, matching the GitHub integration's executable selection.

The version 1 JSON envelope includes `data.tools` with stable `code`, `status`
(`ready`, `missing`, `unavailable`), `required`, `feature`, `message`, and optional
`remedy` fields. The optional `review` record includes the selected profile,
connection, expected workspace and verified workspace identity. Tool diagnostics
are retained even if JJ is missing or workspace discovery/registration fails.
Tokens and raw MCP diagnostics are not included.

`data.configuration` and `data.workspace_check` include independent `code`,
`status`, `message` and optional `remedy` fields. Configuration reports `ready`,
`not_configured` or
`failed`; workspace checks report `ready`, `unregistered`, `unavailable` or `failed`.

Missing required JJ prerequisites return `unhealthy_prerequisites` and a nonzero
exit status. Optional GitHub or Linear problems do not make an otherwise healthy workspace
fail. Invalid configuration and existing workspace errors still return a nonzero
exit status. The JSON report retains every check on failure. The checks
confirm tool readiness, not permissions on every remote repository or CI success.

Doctor verifies connection readiness. A PR without a Linear review is normal and
falls back to GitHub; it does not indicate an unhealthy integration. Lookup errors
are reported separately in JSON and are retried rather than cached as missing reviews.
