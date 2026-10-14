# Installation

WTF 1.0.0 publishes Linux binaries for amd64, arm64, arm (GOARM=7), and 386.
Install JJ 0.46 or newer separately and configure its user name/email.
This release is validated with JJ 0.46, including native Git colocation and
missing-workspace reconciliation. Older JJ versions are not supported.
GitHub CLI, Node.js, Codex's configured Linear MCP connection, and Zed are optional
for their respective integrations. Omarchy is optional; plain terminals work.

```sh
curl -fsSL https://raw.githubusercontent.com/AndrewPBerg/wtf/v1.0.0/install.sh -o /tmp/wtf-install.sh
mkdir -p "$HOME/.local/bin"
VERSION=1.0.0 INSTALL_DIR="$HOME/.local/bin" bash /tmp/wtf-install.sh
wtf version
```

The installer downloads the selected archive and `SHA256SUMS`, verifies its SHA256,
and installs the executable. Ensure the destination exists and is on PATH. You can
instead download assets from [GitHub Releases](https://github.com/AndrewPBerg/wtf/releases/tag/v1.0.0),
verify the archive, extract it, and install `wtf` manually.

For Bash selection to change your shell directory, extract the release archive
and install its wrapper:

```sh
install -Dm644 scripts/wtf.bash "$HOME/.local/share/wtf/wtf.bash"
source "$HOME/.local/share/wtf/wtf.bash"
source <(wtf completion bash)
```

Add those source lines to your Bash startup file yourself. The wrapper defaults
to `$HOME/.local/bin/wtf`; set `WTF_BINARY` if installed elsewhere. The binary alone
returns selection results; it cannot change its parent shell's directory.
See [completion](completion.md) for Zsh/Fish and aliases.

Build from source with Go 1.26.1:

```sh
GOMAXPROCS=1 go build -p 1 -o bin/wtf ./cmd/wtf
./bin/wtf version
```

macOS, Windows, and BSD workspace mutation are not supported by this release.
The legacy PowerShell installer is retained for historical 0.x artifacts only.
