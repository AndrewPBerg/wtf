#!/usr/bin/env bash
set -euo pipefail

REPO="AndrewPBerg/wtf"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS and architecture
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *)      echo "Unsupported OS: $OS (try the Windows zip from GitHub Releases)"; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64)  arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *)             echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

TARGET="${os}_${arch}"

# Get latest version if not specified
if [ -z "${VERSION:-}" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*"v\(.*\)".*/\1/')"
fi

echo "Installing wtf v${VERSION} (${TARGET})..."

TARBALL="wtf_${TARGET}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${TARBALL}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/v${VERSION}/SHA256SUMS"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

curl -fsSL "$URL" -o "${TMP}/${TARBALL}"
curl -fsSL "$CHECKSUM_URL" -o "${TMP}/SHA256SUMS"

# Verify checksum
cd "$TMP" && grep "$TARBALL" SHA256SUMS | sha256sum -c -

tar xzf "${TMP}/${TARBALL}" -C "$TMP"

if [ -w "$INSTALL_DIR" ]; then
  mv "${TMP}/wtf" "${INSTALL_DIR}/wtf"
else
  sudo mv "${TMP}/wtf" "${INSTALL_DIR}/wtf"
fi

echo "wtf v${VERSION} installed to ${INSTALL_DIR}/wtf"
