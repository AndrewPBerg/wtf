> Historical design or 0.x reference. This page is not the WTF 1.0.0 contract.
> Start with the [current documentation](README.md) and [migration guide](migration.md).

# wtf list / ls

Browse registered JJ workspaces and select one by name. `ls` is the only alias
of `list`.

```sh
wtf ls
wtf ls AgentCase
wtf ls --global
wtf ls --here --json
```

On a terminal, the command opens an inline workspace picker. Arrow keys move,
Enter selects, and Esc cancels. With the Bash navigation wrapper loaded, selection
changes the current shell's directory. JSON and redirected output never prompt.

Use `j`/`k` to move down/up, or a numeric prefix such as `4j` or `4k` to
move several rows. Row movement wraps through the list. `gg` selects the first
row of the current page; `G` selects its last row. Left/Right, `h`/`l` (also
`H`/`L`), and PageUp/PageDown move between pages. Other keys cancel a pending
numeric prefix or unfinished `gg` motion.

When the scope includes several repositories, each page contains workspaces from
one repository. The caption shows its name, workspace count, and position among
repositories. Large repositories get several pages. Left/Right moves through all
pages; Tab/Shift-Tab jumps to the next/previous repository. Row movement still
wraps through the entire scoped list, and metadata refreshes retain your selection.

```sh
install -Dm644 scripts/wtf.bash ~/.local/share/wtf/wtf.bash
source ~/.local/share/wtf/wtf.bash
wtf ls
```

| Flag | Meaning |
|---|---|
| `--global` | Include all registered repositories |
| `--here` | Show the current workspace |
| `--all`, `-a` | Include removed records |
| `--hook`, `-k` | Include recorded local stages |
| `--pr`, `-p` | Show PR links (enabled by default) |
| `--checks` | Show CI checks (enabled by default) |
| `--json`, `-j` | Emit one versioned JSON result |

Inside a repository, listing shows its workspaces. From a parent directory, it
shows registered repositories beneath that directory. Register and prepare a
workspace with `wtf new .`; create one with `wtf new PATH`.

See the [README](../README.md) for presentation and shell integration details.
