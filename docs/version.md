# wtf version

Print the current version of wtf.

## Usage

```bash
wtf version
```

## Examples

```bash
$ wtf version
wtf version 0.2.0
```

## Build-time Version

The version string is set at build time via Go linker flags (`-ldflags`). When installed from source without flags, the version displays as `dev`.
