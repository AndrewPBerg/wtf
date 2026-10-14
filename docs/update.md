> Historical design or 0.x reference. This page is not the WTF 1.0.0 contract.
> Start with the [current documentation](README.md) and [migration guide](migration.md).

# wtf update

Update wtf to the latest version by downloading and running the install script.

## Usage

```bash
wtf update
```

## Examples

```bash
$ wtf update
⟳ Updating wtf...
✔ Updated successfully.
```

## Behavior

1. Downloads the install script from the main branch on GitHub
2. Runs it via `sh` to install the latest binary
3. Reports success or failure

Requires `curl` to be available on the system.
