# Repeatable Lua hello world

The local development `.wtf/` contains this example already. The canonical copies
here are tracked so tests and other checkouts can reproduce it.

`hello_world_new.lua` runs after creation; `hello_world_list.lua` runs after list.
Both use `lib/hello_world.lua` to exercise files, links, command capture, cwd,
environment reads/merges/unsets, inspection, and idempotent removal.

Generated files live only under `.wtf/local/hello-world/{new,list}`. The greeting
and report are atomically refreshed; `kept-copy.txt` preserves edits. Link requests
are reused, and the probe is deleted each run. No dependencies or services start.

```sh
wtf new ../hello-demo -f @
wtf list --pr=false --checks=false
wtf hook run hello_world_list
```

Optionally set `WTF_HELLO_LABEL` to customize the child greeting. The example
passes only its explicit settings to the child and changes no shell environment.
Repeat listing/manual hooks as often as desired. Use a fresh destination for each
creation test; preparing an existing workspace does not repeat creation hooks.

To use the example in another repository, run `wtf setup`, copy the two callback
files and `lib/` under `.wtf/hooks`, then add the `[new]` and `[list]` arrays from
`config.toml` to that repository's config. Keep existing project settings.

See [the substrate reference](../../docs/lua-substrate.md) for the complete API.
