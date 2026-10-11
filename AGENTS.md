# V1 CLI invariants

- All command names, aliases, long flags, and short flags are case-insensitive.
  This is shared CLI middleware, not a per-command feature. New commands inherit it.
- `-j` aliases `--json`; `-f` aliases `--from`. Mixed-case spellings behave identically.
- Preserve case in opaque values: workspace/task/hook names, paths, JJ revisions,
  environment strings, and child argv after `--`. Boolean flag literals ignore case.
- Every command, group, and alias has useful help and a versioned JSON contract,
  including argument failures. JSON stdout contains one result; logs use stderr.
- Human-readable output is the default. JSON requires `-j`/`--json`; never fall
  back to dumping raw records in human mode. Colors follow the active Omarchy
  palette, while JSON and non-color output stay free of terminal styling.
- Extend the command-tree contract tests when adding commands. Keep the dynamic
  future-command middleware test so extensions cannot silently bypass these rules.
- Commands start barebones. Optional flags may be enabled once on the CLI or
  inherited from `[commands.<name>]` in TOML. Per-flag priority is CLI > leaf >
  nearest parent > outer parents > global, including explicit `false` values.
  Use the common defaults resolver for new flags.
- PR links are enabled by default and prefer verified Linear reviews, falling
  back to GitHub. Keep their appearance identical; provider diagnostics belong
  in doctor and JSON. Explicit --pr=false disables the view.
- CI checks are enabled by default in a separate column. Explicit --checks=false
  or inherited TOML defaults can disable them.
- Bound metadata requests and timeouts; preserve GitHub observations on failure
  and distinguish unavailable from unassociated. Cache review links by repository
  and account configuration. Never cache transient failures as missing reviews.
- Use the existing Codex MCP OAuth connection and verify its workspace before
  review lookup. Start no model turn and emit no credential or child diagnostics.
- Hook registration is explicit: TOML selects a Lua entrypoint, and its returned
  hooks/overrides table registers functions. Never infer behavior from filenames.
  Imported modules stay within the selected package. Checks may block before
  mutation; hook arguments after `--` remain project-owned and preserve case.
