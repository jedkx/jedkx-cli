#!/bin/sh
# jedkx-cli installer for macOS / Linux — no sudo.
#   curl -fsSL https://raw.githubusercontent.com/jedkx/jedkx-cli/main/scripts/install.sh | sh
# Downloads the latest release, verifies its SHA-256, runs `jedkx-cli install`
# (→ ~/.local/bin/jedkx-cli) and deletes everything it downloaded.
set -eu

repo="jedkx/jedkx-cli"
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported CPU: $(uname -m)" >&2; exit 1 ;;
esac
asset="jedkx-cli_${os}_${arch}"
base="https://github.com/$repo/releases/latest/download"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "  jedkx-cli — downloading $asset…"
curl -fsSL "$base/$asset" -o "$tmp/jedkx-cli"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

want=$(awk -v a="$asset" '$2 == a { print $1 }' "$tmp/checksums.txt")
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/jedkx-cli" | awk '{print $1}')
else
  got=$(shasum -a 256 "$tmp/jedkx-cli" | awk '{print $1}')
fi
[ -n "$want" ] && [ "$want" = "$got" ] || { echo "checksum mismatch — nothing was installed" >&2; exit 1; }

chmod +x "$tmp/jedkx-cli"
"$tmp/jedkx-cli" install
