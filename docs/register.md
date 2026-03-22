# wtf register

Manually register one or more local repos in the wtf registry (`~/.wtf/repos.json`).

## Usage

```bash
wtf register [path...] [flags]
```

## Flags

| Flag          | Description                              |
|---------------|------------------------------------------|
| `-l, --list`  | List all registered repos after registering |
| `--json`      | Output in JSON format                    |

## Examples

```bash
# Register the current repo
$ wtf register
✔ Registered /home/user/code/my-app

# Register a repo by relative path
$ wtf register ../my-lib
✔ Registered /home/user/code/my-lib

# Register multiple repos at once
$ wtf register ../my-lib ../my-api /home/user/code/my-ui
✔ Registered /home/user/code/my-lib
✔ Registered /home/user/code/my-api
✔ Registered /home/user/code/my-ui

# Register and list all repos
$ wtf register -l
✔ Registered /home/user/code/my-app

→ my-app /home/user/code/my-app
  my-lib /home/user/code/my-lib

2 repo(s) registered

# JSON output
$ wtf register ../my-lib --json
{"registered": ["/home/user/code/my-lib"]}
```

## Behavior

1. If no path is given, registers the current git repo
2. Resolves relative paths to absolute paths
3. Validates that each path is a git repository (has a `.git` directory)
4. Skips repos that are already registered (no duplicates)
5. With `--list`, prints the full registry after registering

## Notes

- Repos are also auto-registered when you run any `wtf` command inside them
- Use `wtf unregister` to remove a repo from the registry
- Use `wtf repos` to list all registered repos
