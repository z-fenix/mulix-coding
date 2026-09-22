#!/usr/bin/env bash
# build.sh — build the mulix CLI locally with a stamped version.
#
# Usage:
#   ./build.sh            # build bin/mulix (native platform)
#   ./build.sh test       # build + go vet + go test ./...
#
# Version resolution:
#   MULIX_VERSION env var > v$(git describe --tags) > dev

set -euo pipefail
cd "$(dirname "$0")/.."

run_tests=0
if [ "${1:-}" = "test" ]; then
    run_tests=1
fi

if [ -n "${MULIX_VERSION:-}" ]; then
    version="$MULIX_VERSION"
else
    version="$(git describe --tags 2>/dev/null || echo dev)"
fi

ldflags="-X main.version=${version} -s -w"

echo "==> building mulix ${version}"
if [ "$(go env GOOS)" = "windows" ]; then
    go build -ldflags "$ldflags" -o bin/mulix.exe ./cmd/mulix
else
    go build -ldflags "$ldflags" -o bin/mulix ./cmd/mulix
fi

if [ "$run_tests" = 1 ]; then
    echo "==> vet"
    go vet ./...
    echo "==> test"
    go test ./...
fi

echo "==> done"
