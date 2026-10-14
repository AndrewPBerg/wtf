# List and switch workspaces

`wtf list`, `wtf ls`, and `wtf sw` are the same command. On a terminal they open
an inline browser; piped/JSON output lists records without prompting. A supplied
workspace name returns its path directly. Source the [Bash wrapper](installation.md)
for interactive selection to change your shell directory.

```sh
wtf sw
wtf list feature-a --select
wtf ls --pr=false --checks=false
wtf list --global -j
wtf list --all -j
```

Up/Down or j/k wrap through rows; counts such as 4j/4k repeat the motion. gg/G
select the top/bottom of the current page. Left/Right, h/l, and PageUp/PageDown
change pages. Enter selects; Esc cancels. Terminal hyperlinks open workspace
folders through Zed's URL handler. Messages appear under the selected row.

Press `/` to fuzzy-search workspace names and messages as you type (ignoring
case). Matching characters use the active theme's accent color and an underline.
Enter applies the filter and returns to navigation; press Enter again to select.
`/` edits the query, Backspace deletes a character, Ctrl+U clears the input, and
Esc clears the search before closing the picker. Repository pages include only
matching workspaces; metadata refreshes preserve the filter.

By default, unavailable/removed paths are hidden. `--all` includes retained
records; `--here` restricts to the current workspace; `--global` covers registered
repositories. `--global` and `--here` cannot be combined. The stage column uses
`--hook`; GitHub CI counts use `--checks`. PR links prefer verified Linear reviews
and fall back to GitHub. See [metadata](metadata.md).

Listing reconciles confirmed missing idle registrations, forgetting only stale
WTF/JJ/Git administrative metadata. It does not remove source directories. Locks,
claims, broken symlinks, and access failures prevent pruning. See [removal](remove.md).
