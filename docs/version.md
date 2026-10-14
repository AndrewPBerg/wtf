# Version

```sh
wtf version
wtf --version
wtf -v
wtf version -j
```

Human output is `WTF 1.0.1`. JSON returns the versioned envelope with
`data.version = "1.0.1"`. Version/help never load project hooks or require a
registered workspace. Source carries the release version; tagged builds inject
the same version through the Go linker. Version does not update the binary.
