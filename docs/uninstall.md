# wtf uninstall

Remove the wtf binary from the system.

## Usage

```bash
wtf uninstall [--force]
```

## Flags

| Flag            | Description                        |
|-----------------|------------------------------------|
| `--force`, `-f` | Skip the confirmation prompt       |

## Examples

```bash
# Interactive (prompts for confirmation)
$ wtf uninstall
Remove /home/user/go/bin/wtf? [y/N] y
✔ Removed /home/user/go/bin/wtf

# Skip prompt
$ wtf uninstall --force
✔ Removed /home/user/go/bin/wtf
```

## Behavior

1. Locates the `wtf` binary via `$PATH`
2. Prompts for confirmation (unless `--force`)
3. Removes the binary file

This does not remove shell integration lines from your profile. To fully clean up, also remove the `eval "$(wtf init)"` line from your shell RC file.
