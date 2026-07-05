#!/bin/bash
# Downloads the latest csl-overnighter release for this machine and sets it
# up at ~/Desktop/CSL-Permit/csl-overnighter.
set -euo pipefail

REPO="nabec512/csl-overnighter"
INSTALL_DIR="$HOME/Desktop/CSL-Permit"
BINARY_NAME="csl-overnighter"

case "$(uname -s)" in
  Darwin) os="darwin" ;;
  Linux) os="linux" ;;
  *) echo "Error: unsupported OS $(uname -s). This installer supports macOS and Linux only." >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) echo "Error: unsupported CPU architecture $(uname -m)." >&2; exit 1 ;;
esac

asset="${BINARY_NAME}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/latest/download/${asset}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

echo "Downloading $asset..."
curl -fsSL "$url" -o "$tmp_dir/$asset"

echo "Extracting..."
tar -xzf "$tmp_dir/$asset" -C "$tmp_dir"

mkdir -p "$INSTALL_DIR"
mv -f "$tmp_dir/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"

# Downloaded via curl rather than a browser, so this normally isn't needed,
# but clear it defensively in case macOS tagged it anyway.
if [ "$os" = "darwin" ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$INSTALL_DIR/$BINARY_NAME" 2>/dev/null || true
fi

echo "Installed to $INSTALL_DIR/$BINARY_NAME"
