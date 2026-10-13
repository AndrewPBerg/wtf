# Copy local files on creation

The development checkout enables `.wtf/hooks/copy.lua` in `[new].after`.
Mock sources live in `.wtf/local/copy-example`. The hook reads that directory
from the project root and transfers selected files to the same relative path in
the new workspace. Everything stays ignored.

```toml
[new]
after = ["copy", "hello_world_new"]
```

The `rules` table in `copy.lua` selects filename prefixes (`.env`), suffixes
(`.local`), and explicit names (`copy me.md`). Matching regular files are copied
unless their names appear in `symlink`. This example copies `.env.local` and
`copy me.md`, and links `preferences.local` to the main workspace's mock file.
Subdirectories and source symlinks are skipped. Missing sources are skipped.

Copies are independent: editing one does not edit its source. Links share their
target: editing through `preferences.local` edits the main workspace's mock file.
Existing destinations are preserved; identical links are reused. Re-running
`wtf new` on an existing workspace does not repeat creation hooks.

```sh
wtf new ../copy-demo -f @
```

Inspect `../copy-demo/.wtf/local/copy-example`. Use a fresh destination to test
creation again. The callback and its configuration are inherited by new workspaces
automatically. Main-workspace manual invocations skip transfer.
No real env files are read, no values are logged, and no commands or services run.

For another checkout, run `wtf setup`, copy `copy.lua` into `.wtf/hooks`, copy
these mock `files/` into `.wtf/local/copy-example`, and add `copy` to your existing
creation hook array. Edit the local rules to choose actual project inputs later.
