# WTF Pi extension

This directory is the canonical source for WTF's Pi extension.

The extension keeps agent workspace creation on WTF's managed path by blocking raw
`git worktree add` and `jj workspace add` tool calls. A deliberately exceptional
raw operation can opt out with `WTF_OK=1` or a `# wtf-ok` comment.

Install or refresh the local Pi copy from the repository root:

```bash
./scripts/install-pi-extension.sh
```

Then run `/reload` in existing Pi sessions.

Development checks:

```bash
cd packages/pi-extension
pnpm install
pnpm check
```
