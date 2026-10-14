# Shell completions

`wtf completion SHELL` prints a completion script for Bash, Zsh or Fish.
The command appears under **Utilities** in `wtf --help`. It works without a
repository and does not edit shell configuration.

## Bash

Load completions in the current shell:

```bash
source <(wtf completion bash)
```

For future shells, generate a file and source it from your Bash configuration:

```bash
mkdir -p ~/.local/share/bash-completion/completions
wtf completion bash > ~/.local/share/bash-completion/completions/wtf
source ~/.local/share/bash-completion/completions/wtf
```

The script registers `wtf` and the common `f` alias. Define `alias f='wtf'`
separately if you use it. For another shell alias or navigation function:

```bash
complete -F _wtf_complete work
```

For keyboard cycling, add `"\t": menu-complete` to `~/.inputrc`. The default
Readline Tab binding may first expand the common prefix or list candidates.

The Bash script enables Readline's colored common-prefix highlight and displays
the full prefix instead of abbreviating it. Highlights use the terminal's native
palette (and Readline's existing `LS_COLORS` customization), so terminal theme
changes recolor the menu. File-type colors and Tab bindings are preserved.
These Readline display settings apply to the shell's completion menus generally.
`NO_COLOR` and `TERM=dumb` disable the added highlight. Candidate strings stay
plain: color codes never become part of a workspace name or task argument.
Zsh and Fish retain their own native completion menu colors and styling.

## Zsh

Initialize Zsh's completion system before sourcing the script:

```zsh
autoload -Uz compinit
compinit
source <(wtf completion zsh)
zstyle ':completion:*' menu select
```

For persistent setup, save the generated script and source it after `compinit`
in `.zshrc`. It registers `wtf` and `f`. Add another alias with
`compdef _wtf_complete work`.

## Fish

```fish
mkdir -p ~/.config/fish/completions
wtf completion fish > ~/.config/fish/completions/wtf.fish
source ~/.config/fish/completions/wtf.fish
```

The script registers `wtf` and `f`. Fish provides its usual completion menu and
fuzzy matching. For another alias, use
`complete -c work -f -a '(__wtf_complete)'`.

## Live candidates

All three scripts complete commands, command aliases, nested subcommands, long
and short flags, Boolean values and the generator's shell argument. Command and
flag matching ignores case. Workspace and task candidates preserve their case.

| Input | Candidates |
| --- | --- |
| `wtf l<Tab>` | `list`, `ls` |
| `f ls test<Tab>` | Available workspace names such as `testing-msg` |
| `f ls tmsg<Tab>` | Fuzzy subsequence matches such as `testing-msg` |
| `wtf rm test<Tab>` | Available workspace names |
| `wtf change test<Tab>` | Available workspace names |
| `wtf ls --global <Tab>` | Workspaces across registered repositories |
| `wtf run <Tab>` | Tasks declared in the current workspace's `.wtf/config.toml` |
| `wtf run tmix<Tab>` | Fuzzy task matches such as `TestMixed` |
| `wtf list --<Tab>` | Long flags |
| `wtf list --pr=f<Tab>` | `--pr=false` |
| `wtf new <Tab>` | Filesystem paths |

Workspace scope follows `list`: inside a repository, its registered workspaces;
from a parent directory, registered repositories beneath it; with `--global`,
all registered repositories. `--here` restricts candidates to the current
workspace. Removed and missing directories are excluded. Zsh and Fish can show
workspace messages and task commands alongside candidates.

Tasks use the workspace-root configuration, including when invoked from a
subdirectory. Files under `.wtf/tasks/` are not automatically runnable tasks;
declare their argv in `[tasks.NAME]` to expose them to `run` and completion.
Extra task arguments and arguments after `--` use filesystem completion.
Free-form messages and JJ revision expressions do not invent candidate values.

Lookups read local configuration and registries on each Tab request, with a
two-second context for external discovery. They do not reconcile or edit the
registry, snapshot JJ, run Lua modules/hooks/tasks, or fetch PR/CI metadata.
No project code runs while generating or requesting completions.

Set `WTF_BINARY` to use a specific build, such as a pilot binary. Otherwise the
script invokes `wtf` through the current shell, including the Bash navigation
wrapper. To regenerate after updating WTF, run the generation command again.

`wtf completion bash --json` returns one versioned result containing `shell` and
`script`. Human output is the sourceable script itself, without terminal styling.
