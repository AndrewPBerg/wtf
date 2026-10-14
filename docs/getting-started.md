# Getting started

Run inside an existing JJ repository. For a new repository, initialize it with
`jj git init` and set JJ's user identity first.

```sh
wtf setup
wtf new .
wtf doctor
wtf new feature-a -f @
wtf ls --pr=false --checks=false
```

Setup creates local ignored config, schema, and empty hooks/tasks. `new .`
registers the current workspace without taking deletion ownership. `new feature-a`
creates a sibling of the registered main repository. `-f @` selects your current
JJ revision; omitted `--from` selects `trunk()`. Explicit paths such as
`./experiments/feature-a` resolve from your current directory.

New workspaces inherit the invoking workspace's config, schema, hook tree, task
tree, and local ignore rules before initialization/creation callbacks. Runtime
state and `.wtf/local/` are excluded. Files are independent copies, so child
customizations can be inherited by further children. Existing destinations keep
their config and do not repeat creation triggers.

To add your first callback, follow the [hook example](hooks.md#trigger-phases).
For real file setup, install the [copy example](../examples/copy/README.md).
Tasks are [declared in TOML](tasks.md), not discovered from task filenames.

Use `wtf change feature-a -m "Working on the API"` to label your work. Use the
[Bash wrapper](installation.md) to make `wtf sw` change your current directory.
JJ still owns revisions: describe changes, create new changes, move bookmarks,
fetch, and push with JJ. WTF does not commit or publish source changes.

For cleanup, inspect `wtf remove -j` before opening the interactive removal
picker. Current and adopted workspaces remain protected. Read [removal](remove.md)
before choosing force mode, which discards remaining source changes.
