# Command reference

Every command/group/alias accepts `--help` and `-j`/`--json`. JSON help and
argument failures use the same [versioned result envelope](json.md). Commands,
aliases, and flags ignore case; opaque values and child arguments preserve it.
Boolean literals ignore case. Human output is the default.

| Command | Arguments and important options | Details |
|---|---|---|
| `new` | `PATH [-f REVISION] [-m TEXT] [-- HOOK_ARGS...]` | [Creation/preparation](new.md); default revision `trunk()` |
| `list`, `ls`, `sw` | `[NAME] [--global \| --here] [-a] [--select] [-k] [-p] [-c] [--refresh]` | [Browser/selection](list.md) |
| `change` | `[NAME] -m TEXT` | [Messages](change.md); no mutation without requested message |
| `remove`, `rm` | `[NAME]` | [Safe cleanup](remove.md); bare terminal command opens picker |
| `setup` | no arguments | [Local scaffold](setup.md) |
| `setup teardown` | no arguments | Yes/no terminal confirmation, default No |
| `hook`, `hook list` | no arguments | Registered callbacks/overrides; does not invoke callbacks |
| `hook run` | `NAME [-- HOOK_ARGS...]` | Invoke a registered hook under workspace lock |
| `run` | `TASK [-- ARGS...]` | [Recorded task execution](tasks.md) |
| `ports`, `ports list` | no arguments | [Current repository claims](ports.md) |
| `ports claim` | `NAME --port PORT` | Caller-selected port, 1–65535 |
| `ports release` | `NAME` | Release claim; does not stop service |
| `doctor`, `doc` | `[--here]` | [Configuration/tool/state checks](doctor.md) |
| `completion` | `bash`, `zsh`, or `fish` | [Shell scripts](completion.md) |
| `version` | no arguments | [Installed version](version.md) |

Root `--version`/`-v` also prints version. Bare `wtf` shows help.
List defaults: PR links/checks enabled; stage column, global, all, selection disabled.
`-k` means `--hook`, `-p` means `--pr`, `-c` means `--checks`, `-a` means `--all`.
Explicit `--pr=false` and `--checks=false` disable metadata columns.
Use `wtf COMMAND --help` for the complete option set.

[TOML defaults](configuration.md) inherit per flag: CLI > leaf > nearest parent >
outer parents > global. Explicit false values override true values.
No `init`, `validate`, `resources`, `watch`, `update`, or `uninstall` command exists
in 1.0.0. See [migration](migration.md) for the 0.x command transition.
