# WTF simplification plan

> **Status:** preparation and audit. Remove one layer at a time and dogfood the
> replacement before deleting another feature.

## Product boundary

WTF should be a small, opinionated workspace-lifecycle wrapper with excellent
defaults. Its core job is:

```text
create -> enter -> configure -> inspect -> integrate -> publish -> remove
```

For the JJ-first workflow, that means workspaces, Zed-compatible diff metadata,
environment setup, resource leases, and a few safe graph operations. Pi integrations
should call the same CLI contract rather than own parallel behavior.

WTF should not become a general terminal UI framework, notification daemon, forge
dashboard, or hidden project orchestrator.

## Current dependency audit

The Charm Bracelet dependency surface is narrow:

- `internal/ui/picker.go` uses Bubble Tea and Lip Gloss for worktree selection.
- `internal/ui/repo_picker.go` uses them for repository registration/removal.
- their tests depend on Bubble Tea key messages.

The remaining `internal/ui` files provide small ANSI rerendering and output scrolling
and do not require Charm. Removing Bubble Tea and Lip Gloss also removes most of the
large indirect terminal dependency tree.

Other direct dependencies have narrower roles:

- Cobra: command parsing
- `fatih/color`: existing compact color output
- `go-isatty`: TTY detection
- Testify: tests only

## Proposed boring picker replacement

Preserve command behavior without preserving a full-screen TUI:

1. Render a numbered list to stderr.
2. Read one line from the terminal.
3. Accept one number for single selection.
4. Accept comma-separated numbers and ranges for multi-selection.
5. Treat an empty line, `q`, or EOF as cancellation.
6. Keep stdout reserved for the selected path so shell switching still works.
7. Keep `--json` and non-interactive behavior unchanged.

Example:

```text
Select a workspace:
  1  default *       /code/project
  2  feature/auth    /code/feature-auth--project
  3  spike/cache     /code/spike-cache--project
Choice [1-3, q]: 2
```

This retains discoverability and multi-select operations while making the behavior
ordinary line-oriented I/O that is easy to test. Shell completion and fuzzy command
arguments remain the faster path for experienced use.

Before implementation, confirm whether numbered selection is still valuable. The
smaller alternative is for no-argument commands to print a table and require an
argument for mutation or switching.

## Feature-surface audit

Classify commands by whether they serve the workspace lifecycle.

### Keep and strengthen

- `new` / `news`
- `sw`
- `rm` / `clean`
- `setup`
- `port`, expanding toward named resources
- JJ backend and Zed Git-diff shadow
- PR checkout needed by the review workflow
- machine-readable JSON output

Expected future lifecycle verbs are `sync`, `gather`, `publish`, `resources`, and
possibly `open`; each requires separate design and dogfooding.

### Re-evaluate after picker removal

- global repository registration and switching
- interactive register/unregister flows
- async PR table rerendering
- forge watching and desktop notifications
- self-update and uninstall commands
- GitLab support if it is not exercised by real work
- automatically starting development servers

Do not remove these together. For each candidate, record whether it was used during
the dogfood month, what simpler command or external tool replaces it, and whether
removal would break scripts or documentation.

PR checkout is not grouped with forge dashboards: creating a workspace from an
incoming PR is part of the core workflow even if passive monitoring is not.

## Pi extension boundary

The canonical Pi extension now belongs in:

```text
packages/pi-extension/
```

The repository install script copies the runtime adapter to Pi's global extension
directory. The extension should stay thin:

- enforce WTF-managed workspace creation
- expose typed wrappers only after the CLI operation is stable
- consume structured `--json` results
- avoid reimplementing JJ or workspace policy in TypeScript

Agent Bridge remains a separate product and coordination service. WTF may consult it
through adapters before integration or cleanup, but WTF's local safety must not
depend on the daemon being available.

## Removal order

1. Move the Pi extension source and tests into WTF. **Prepared.**
2. Replace Charm pickers without changing command semantics.
3. Run the normal Go quality suite and compare binary/dependency size.
4. Dogfood switching, registration, removal, and cancellation behavior.
5. Remove Bubble Tea and Lip Gloss with `go mod tidy`.
6. Audit passive monitoring and global registry features from usage evidence.
7. Remove one unused feature at a time, updating docs and tests with each removal.
8. Begin new lifecycle verbs only after the smaller boundary is stable.

## Measurements

Record before and after each simplification:

- `go list -m all | wc -l`
- built binary size
- cold `wtf sw` and `wtf new --no-setup` timing
- direct and indirect module count
- command/test count removed or replaced
- real workflow regressions during dogfooding

Dependency count is evidence, not the goal. The goal is a smaller behavioral and
maintenance surface without making workspace creation harder.

## Open decisions

1. Replace Charm with a numbered prompt, or remove interactive selection entirely?
2. Is global cross-repository switching part of the core daily workflow?
3. Is `wtf watch` still useful once editor and Agent Bridge notifications exist?
4. Should self-update/uninstall remain in a small Go binary distributed by an install
   script?
5. Which forge providers are exercised often enough to remain first-class?
6. Should development-server startup remain automatic or become an explicit command?
