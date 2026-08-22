# wtf git-diff

Create or refresh the private Git metadata that lets Git-aware editors show
changes in a secondary jj workspace.

```bash
wtf git-diff
```

`wtf new` and `wtf news` create this metadata by default for jj workspaces.
Run `wtf git-diff` after a jj operation changes the workspace parent. The
command resets the editor-only Git index and never changes working-copy files.

Git staging and commits are presentation-only and do not update jj. Use jj for
all version-control mutations. To skip metadata when creating a workspace, use
`--no-git-diff` or set `WTF_JJ_GIT_DIFF=0`.
