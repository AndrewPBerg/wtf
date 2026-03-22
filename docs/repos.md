# wtf repos

List all repos registered in the wtf registry (`~/.wtf/repos.json`).

## Usage

```bash
wtf repos [--json]
```

## Flags

| Flag     | Description            |
|----------|------------------------|
| `--json` | Output in JSON format  |

## Examples

```bash
# List all registered repos
$ wtf repos
→ my-app /home/user/code/my-app
  my-lib /home/user/code/my-lib

2 repo(s) registered

# JSON output
$ wtf repos --json
["/home/user/code/my-app", "/home/user/code/my-lib"]
```

## Behavior

1. Loads all paths from `~/.wtf/repos.json`
2. Auto-prunes stale entries (deleted directories, non-git repos)
3. Highlights the current repo with a `→` indicator
4. Shows the repo name (directory basename) and full path

## Auto-registration

Repos are automatically registered when you run any `wtf` command inside them. To manually register a repo, use `wtf register [path]`. To remove a repo, use `wtf unregister`.
