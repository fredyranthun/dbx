#!/usr/bin/env bash
set -euo pipefail

REPO="fredyranthun/dbx"
BIN="dbx"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64 | arm64) ARCH="arm64" ;;
  *)
    echo "Unsupported arch: $ARCH" >&2
    exit 1
    ;;
esac

if [[ "$OS" != "linux" && "$OS" != "darwin" ]]; then
  echo "Unsupported OS: $OS (this installer supports Linux/WSL and macOS)." >&2
  exit 1
fi

API_URL="https://api.github.com/repos/${REPO}/releases/latest"
json="$(curl -fsSL "$API_URL")"

tag="$(printf '%s' "$json" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
if [[ -z "${tag:-}" ]]; then
  echo "Could not determine latest tag from GitHub API." >&2
  exit 1
fi

base_url="https://github.com/${REPO}/releases/download/${tag}"
checksums_url="${base_url}/checksums.txt"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "Downloading ${checksums_url}"
curl -fsSL "$checksums_url" -o "$tmpdir/checksums.txt"

asset_line="$(grep -E "[[:space:]]${BIN}_.+_${OS}_${ARCH}\\.tar\\.gz$" "$tmpdir/checksums.txt" | head -n1 || true)"
if [[ -z "${asset_line:-}" ]]; then
  echo "No matching release asset found in checksums for repo=${REPO} tag=${tag} os=${OS} arch=${ARCH}." >&2
  exit 1
fi

expected="$(printf '%s\n' "$asset_line" | awk '{print $1}')"
asset="$(printf '%s\n' "$asset_line" | awk '{print $2}')"
asset_url="${base_url}/${asset}"

echo "Downloading ${asset_url}"
curl -fsSL "$asset_url" -o "$tmpdir/$asset"

if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmpdir/$asset" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$tmpdir/$asset" | awk '{print $1}')"
else
  echo "Neither sha256sum nor shasum is available for checksum verification." >&2
  exit 1
fi

if [[ "$expected" != "$actual" ]]; then
  echo "Checksum mismatch for ${asset}." >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
tar -xzf "$tmpdir/$asset" -C "$tmpdir"

if [[ -f "$tmpdir/$BIN" ]]; then
  cp "$tmpdir/$BIN" "$INSTALL_DIR/$BIN"
else
  found="$(find "$tmpdir" -maxdepth 3 -type f -name "$BIN" | head -n1 || true)"
  if [[ -z "$found" ]]; then
    echo "Binary '${BIN}' not found inside archive." >&2
    exit 1
  fi
  cp "$found" "$INSTALL_DIR/$BIN"
fi

chmod +x "$INSTALL_DIR/$BIN"

echo "Installed: $INSTALL_DIR/$BIN"
echo "Version:"
"$INSTALL_DIR/$BIN" --version || true

if ! command -v "$BIN" >/dev/null 2>&1; then
  echo
  echo "Add this to your shell profile:"
  echo "  export PATH=\$PATH:$INSTALL_DIR"
fi
