#!/usr/bin/env bash
set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
source_dir="$root/packages/pi-extension"
target_dir="${PI_HOME:-$HOME/.pi}/agent/extensions/wtf-worktrees"

mkdir -p "$target_dir"
install -m 0644 "$source_dir/index.ts" "$target_dir/index.ts"
install -m 0644 "$source_dir/README.md" "$target_dir/README.md"

printf 'Installed WTF Pi extension to %s\n' "$target_dir"
