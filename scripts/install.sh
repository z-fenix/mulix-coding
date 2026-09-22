#!/usr/bin/env bash
# install.sh — install mulix from source (local compile, no remote binaries).
#
# Usage:
#   curl -fsSL <...>/install.sh | bash            # install from main
#   ./scripts/install.sh --version v0.1.0          # install a specific tag
#   ./scripts/install.sh --prefix /usr/local       # install to a custom dir
#   ./scripts/install.sh --source <local-dir>      # build from a local checkout
#
# Defaults: clone into a temp dir under $HOME, install to ~/.local/bin.

set -euo pipefail

REPO="https://github.com/mulix-dev/mulix-coding"
VERSION=""
PREFIX="${HOME}/.local/bin"
SOURCE=""
TMPDIR_INSTALL=""

usage() {
    sed -n '2,10p' "$0"
    exit 0
}

while [ $# -gt 0 ]; do
    case "$1" in
        --version) VERSION="$2"; shift 2 ;;
        --prefix) PREFIX="$2"; shift 2 ;;
        --source) SOURCE="$2"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "unknown option: $1" >&2; usage ;;
    esac
done

command -v git >/dev/null 2>&1 || { echo "install.sh: git is required" >&2; exit 1; }
command -v go  >/dev/null 2>&1 || { echo "install.sh: go is required (see https://go.dev/dl)" >&2; exit 1; }

# --- acquire source ---
if [ -n "$SOURCE" ]; then
    SRC="$SOURCE"
else
    TMPDIR_INSTALL="$(mktemp -d)"
    trap 'rm -rf "$TMPDIR_INSTALL"' EXIT
    REF="${VERSION:-main}"
    echo "==> cloning ${REPO} @ ${REF}"
    git clone --depth 1 --branch "$REF" "$REPO" "$TMPDIR_INSTALL/mulix"
    SRC="$TMPDIR_INSTALL/mulix"
fi

# --- build ---
cd "$SRC"
STAMP="$(git -C "$SRC" describe --tags 2>/dev/null || echo dev)"
ldflags="-X main.version=${STAMP} -s -w"
echo "==> building mulix ${STAMP}"
if [ "$(go env GOOS)" = "windows" ]; then
    BINARY="$PREFIX/mulix.exe"
else
    BINARY="$PREFIX/mulix"
fi

# --- install ---
mkdir -p "$PREFIX"
go build -ldflags "$ldflags" -o "$BINARY" ./cmd/mulix
echo "==> installed $BINARY"

# --- PATH hint ---
case ":$PATH:" in
    *":$PREFIX:"*) ;;
    *)
        echo
        echo "$PREFIX is not in your PATH. Add it, e.g.:"
        echo "  echo 'export PATH=\"$PREFIX:\$PATH\"' >> ~/.bashrc"
        ;;
esac

"$BINARY" version
