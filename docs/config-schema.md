# TOML editor assistance

The pilot's `.wtf/config.toml` selects the generated schema locally:

```toml
#:schema ./schema.json

version = 1

[commands.list]
pr = true
hook = true
```

Tombi or Taplo can use this schema for key completion, types, hover descriptions,
and diagnostics. Zed offers the Tombi extension. No editor settings are changed by
WTF; install or enable the language server separately if needed. Relative schema
paths resolve from the TOML document, so a global or parent config can point to
this file with its own relative path or an absolute `file://` URL.

Regenerate after changing command flags or configuration types:

```sh
GOMAXPROCS=1 go generate ./internal/cli
```

The checked-in reference lives at `internal/cli/testdata/config-schema.json`.
Keeping it outside the ignored `.wtf/` directory lets clean checkouts run the
schema drift test. `wtf setup` generates the local schema from the installed CLI.
To refresh an existing local schema from source explicitly:

```sh
GOMAXPROCS=1 go run -p 1 ./cmd/wtf-schema -output .wtf/schema.json
```

The generator reads Go TOML types and their documentation tags, then walks the
Cobra command tree for flag types, help text, defaults, and aliases. Shared schema
references avoid duplicating each definition for aliases and mixed-case patterns.
New commands inherit schema support; unsupported flag bindings and undocumented
config fields fail generation. A drift test requires the checked-in schema to match.

The schema is editor assistance. Its `default` values document factory defaults;
they do not apply global/parent/leaf inheritance. Filesystem checks, hook execution,
cross-flag rules, and other runtime validation remain WTF's responsibility.
