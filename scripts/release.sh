#!/usr/bin/env bash
# release.sh — cross-compile mulix for all supported platforms into dist/.
#
# Usage:
#   ./scripts/release.sh            # cross-compile + checksums
#   ./scripts/release.sh --dry-run  # cross-compile only, skip checksums
#   MULIX_VERSION=v0.1.0 ./scripts/release.sh
#
# Targets: linux/darwin/windows x x86_64/arm64 (windows -> .exe).
# Output layout:
#   dist/mulix_<version>_<os>_<arch>/mulix[.exe]
#   dist/mulix_<version>_checksums.txt   (sha256, when not --dry-run)

set -euo pipefail
cd "$(dirname "$0")/.."

DRY_RUN=0
if [ "${1:-}" = "--dry-run" ]; then
    DRY_RUN=1
fi

if [ -n "${MULIX_VERSION:-}" ]; then
    version="$MULIX_VERSION"
else
    version="$(git describe --tags 2>/dev/null || echo dev)"
fi

ldflags="-X main.version=${version} -s -w"

OS_ARCH="linux_amd64 linux_arm64 darwin_amd64 darwin_arm64 windows_amd64 windows_arm64"

rm -rf dist
mkdir -p dist

echo "==> cross-compiling mulix ${version}"
for target in $OS_ARCH; do
    GOOS="${target%%_*}"
    GOARCH="${target##*_}"
    ext=""
    [ "$GOOS" = "windows" ] && ext=".exe"
    outdir="dist/mulix_${version}_${target}"
    mkdir -p "$outdir"
    echo "    ${target}"
    CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
        go build -ldflags "$ldflags" -o "$outdir/mulix${ext}" ./cmd/mulix
done

# note: cross-compiled binaries cannot be executed on this host, so version
# stamp correctness relies on the -ldflags above; verify on a target machine
# via `mulix version` once installed.

if [ "$DRY_RUN" = 0 ]; then
    echo "==> writing checksums"
    ( cd dist
      if command -v sha256sum >/dev/null 2>&1; then
          find . -name 'mulix' -o -name 'mulix.exe' | sort | xargs sha256sum > "mulix_${version}_checksums.txt"
      else
          # macOS / Windows fallback
          for f in $(find . -name 'mulix' -o -name 'mulix.exe' | sort); do
              shasum -a 256 "$f"
          done > "mulix_${version}_checksums.txt"
      fi
    )
else
    echo "==> dry run: skipping checksums"
fi

echo "==> done: dist/"
find dist -type f | sort
