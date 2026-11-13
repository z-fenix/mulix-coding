#!/usr/bin/env bash
# Build a new six-target bundle; existing dist/output content is preserved.
# --dry-run prints a plan only. MULIX_VERSION and --version stamp the binary.
set -euo pipefail
script_dir="${BASH_SOURCE[0]%/*}"
if [ "$script_dir" = "${BASH_SOURCE[0]}" ]; then script_dir=.; fi
script_dir="$(cd -- "$script_dir" && pwd -P)"
python_bin="${PYTHON:-}"
if [ -z "$python_bin" ]; then
    python_bin="$(command -v python3 || command -v python || true)"
fi
if [ -z "$python_bin" ] || ! "$python_bin" -I -B -c 'import sys; sys.exit(sys.version_info < (3, 10))'; then
    printf '%s\n' 'release: Python 3.10+ is required (or set PYTHON)' >&2
    exit 1
fi
exec "$python_bin" -I -B "$script_dir/delivery.py" build "$@"
