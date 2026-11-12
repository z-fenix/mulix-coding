#!/usr/bin/env bash
# Multi-session smoke in a disposable project. Optional argument: mulix binary.
set -euo pipefail
repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
bin=${1:-}
if [ -z "$bin" ]; then
  bin="$work/mulix"
  (cd "$repo" && go build -o "$bin" ./cmd/mulix)
fi
bin="$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")"
mkdir "$work/cli"
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) cli_name=mulix.exe ;; *) cli_name=mulix ;; esac
cp "$bin" "$work/cli/$cli_name"
export PATH="$work/cli:$PATH"
unset MULIX_SESSION_ID
mulix() { "$bin" "$@"; }
mkdir "$work/project"
cd "$work/project"
git init -q
mulix init >/dev/null
mulix context mode session >/dev/null
mulix new "Global context" --global >/dev/null
mulix new "Session A" --session-id session-A >/dev/null
mulix new "Session B" --session-id session-B >/dev/null
change() { mulix state show --session-id "$1" | sed -n 's/^change: *//p'; }
a=$(change session-A)
b=$(change session-B)
# --force here only constructs phase fixtures; guard behavior is tested in e2e.
for event in spec-complete clarify-complete; do mulix state transition "$event" --force --reason 'session phase fixture; not delivery qualification' --session-id session-A >/dev/null; done
for event in spec-complete clarify-complete design-approved tasks-complete; do mulix state transition "$event" --force --reason 'session phase fixture; not delivery qualification' --session-id session-B >/dev/null; done
expect_hook() {
  local id=$1 path=$2 expected=$3 out code
  if out=$(printf '{"session_id":"%s","tool_name":"Write","tool_input":{"file_path":"%s"}}' "$id" "$path" | mulix hook 2>"$work/hook-error"); then code=0; else code=$?; fi
  [ "$code" = "$expected" ] || { printf 'FAIL: %s on %s returned %s, wanted %s: %s\n' "$id" "$path" "$code" "$expected" "$out" >&2; exit 1; }
  case "$expected:$out" in 0:*'"permissionDecision":"allow"'*|2:*'"permissionDecision":"deny"'*) ;; *) printf 'FAIL: malformed hook response: %s\n' "$out" >&2; exit 1 ;; esac
}
expect_hook session-A src/code.go 2
expect_hook session-B src/code.go 0
expect_hook session-C src/code.go 2
mulix state select "$b" --global --session-id session-A >/dev/null
expect_hook session-A src/code.go 2
expect_hook session-C src/code.go 0
expect_hook session-A ".mulix/.runtime/$a/specs/design.md" 0
expect_hook session-B ".mulix/.runtime/$a/specs/design.md" 2
expect_hook session-B .mulix/context.json 2
expect_hook session-B .mulix/sessions/other.json 2
scratch_a=$(mulix context show --field scratch-dir --session-id session-A)
scratch_b=$(mulix context show --field scratch-dir --session-id session-B)
[ "$scratch_a" != "$scratch_b" ]
if command -v cygpath >/dev/null 2>&1; then
  scratch_a=$(cygpath -m "$scratch_a")
  scratch_b=$(cygpath -m "$scratch_b")
fi
expect_hook session-A "$scratch_a/brainstorm/session/content/a.html" 0
expect_hook session-B "$scratch_b/verification/tests.log" 0
expect_hook session-B "$scratch_a/verification/tests.log" 2
expect_hook session-A "$scratch_b/brainstorm/session/content/a.html" 2
if command -v sha256sum >/dev/null 2>&1; then
  digest=$(printf session-A | sha256sum)
else
  digest=$(printf session-A | shasum -a 256)
fi
digest=${digest%% *}
printf '{' > ".mulix/sessions/$digest.json"
expect_hook session-A src/code.go 2
if mulix context show --session-id session-A >/dev/null 2>&1; then printf 'FAIL: corrupt binding fell back\n' >&2; exit 1; fi
mulix context unbind --session-id session-A >/dev/null
expect_hook session-A src/code.go 0
for event in build-complete verify-pass archived; do mulix state transition "$event" --force --reason 'session phase fixture; not delivery qualification' --session-id session-B >/dev/null; done
test ! -f .mulix/active
expect_hook session-B src/code.go 2
expect_hook session-C src/code.go 0
expect_hook session-C .mulix/context.json 2
mulix context show --json --session-id session-C > "$work/empty-context.json"
grep -q '"runtime_dir":""' "$work/empty-context.json"
if runtime=$(mulix context show --field runtime-dir --session-id session-C 2>"$work/no-runtime-error"); then
  printf 'FAIL: missing-current query returned runtime %s\n' "$runtime" >&2; exit 1
fi
[ -z "$runtime" ]
scratch=$(mulix context show --field scratch-dir --session-id session-C)
[ -n "$scratch" ]
test ! -e "$scratch"
expect_hook session-C ".mulix/.runtime/$b/sdd/tasks/progress.md" 2
if mulix context show --field scratch-dir --session-id session-B > "$work/archived-scratch" 2>&1; then
  printf 'FAIL: archived binding produced scratch fallback\n' >&2; exit 1
fi
mulix context mode legacy >/dev/null
expect_hook session-B src/code.go 0
printf '\nPASS: session binding, dynamic fallback, invalid binding, unbind, archive, controls, legacy\n'
