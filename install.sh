#!/bin/sh
# Install beacon: a TUI for running Java Minecraft servers under tmux.
#
#   curl -fsSL https://raw.githubusercontent.com/sunyupei/beacon-tui/main/install.sh | bash
#
# Env overrides:
#   BEACON_VERSION   release tag to install (default: latest)
#   BEACON_INSTALL_DIR   where to put the binary (default: /usr/local/bin if writable, else ~/.local/bin)

set -eu

REPO="sunyupei/beacon-tui"
BINARY="beacon"
VERSION="${BEACON_VERSION:-latest}"

err() { echo "install.sh: $*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

os="$(uname -s)"
case "$os" in
  Darwin) os="darwin" ;;
  Linux)  os="linux" ;;
  *) err "unsupported OS: $os (beacon runs on macOS and Linux)" ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) err "unsupported architecture: $arch" ;;
esac

asset="beacon_${os}_${arch}"
if [ "$VERSION" = "latest" ]; then
  base="https://github.com/${REPO}/releases/latest/download"
else
  base="https://github.com/${REPO}/releases/download/${VERSION}"
fi

if have curl; then
  dl() { curl -fsSL "$1" -o "$2"; }
elif have wget; then
  dl() { wget -qO "$2" "$1"; }
else
  err "need curl or wget"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading ${asset} (${VERSION})..."
dl "${base}/${asset}" "${tmp}/${BINARY}" || err "download failed: ${base}/${asset}"

if dl "${base}/${asset}.sha256" "${tmp}/sum" 2>/dev/null; then
  want="$(awk '{print $1}' "${tmp}/sum")"
  if have sha256sum; then
    got="$(sha256sum "${tmp}/${BINARY}" | awk '{print $1}')"
  elif have shasum; then
    got="$(shasum -a 256 "${tmp}/${BINARY}" | awk '{print $1}')"
  else
    got=""
  fi
  if [ -n "$got" ] && [ "$got" != "$want" ]; then
    err "checksum mismatch: got $got want $want"
  fi
  [ -n "$got" ] && echo "Checksum OK."
else
  echo "Warning: no checksum published for this asset, skipping verification." >&2
fi

chmod +x "${tmp}/${BINARY}"

if [ -n "${BEACON_INSTALL_DIR:-}" ]; then
  dir="$BEACON_INSTALL_DIR"
elif [ -w /usr/local/bin ] || { [ ! -e /usr/local/bin ] && mkdir -p /usr/local/bin 2>/dev/null; }; then
  dir="/usr/local/bin"
else
  dir="${HOME}/.local/bin"
fi
mkdir -p "$dir" || err "cannot create $dir"

mv "${tmp}/${BINARY}" "${dir}/${BINARY}" || err "cannot write ${dir}/${BINARY}"
echo "Installed ${dir}/${BINARY}"

case ":${PATH}:" in
  *":${dir}:"*) ;;
  *) echo ""; echo "Note: ${dir} is not on your PATH. Add this to your shell profile:"; echo "  export PATH=\"${dir}:\$PATH\"" ;;
esac

if have tmux; then
  echo "Run: ${BINARY}"
else
  echo ""
  echo "beacon needs tmux on PATH. Install it with:"
  echo "  macOS:  brew install tmux"
  echo "  Debian: sudo apt-get install -y tmux"
fi
