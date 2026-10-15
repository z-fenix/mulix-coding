#!/usr/bin/env bash
# Real CLI resource-update smoke in private throwaway projects.
# Usage: bash scripts/smoke-update.sh [path-to-mulix-binary]
set -euo pipefail
[ -n "${SMOKE_TRACE:-}" ] && set -x
repo=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
bin=${1:-}
if [ -z "$bin" ]; then
  bin="$work/mulix"
  (cd "$repo" && go build -o "$bin" ./cmd/mulix)
fi
mulix() { "$bin" "$@"; }
update() { (cd "$1" && mulix update "${@:2}"); }
expect_update_fail() {
  local project=$1
  shift
  if update "$project" "$@" > "$work/failure.log" 2>&1; then
    printf 'FAIL: update unexpectedly succeeded in %s\n' "$project" >&2
    exit 1
  fi
}

printf '\n== DSH-only: three updates keep local edits and pure baseline\n'
project="$work/dsh"
mulix init --dir "$project" --host dsh >/dev/null
rel=.mulix/templates/spec-template.md
cp "$project/$rel" "$work/vendor.md"
printf '\nSMOKE_LOCAL_CUSTOMIZATION\n' >> "$project/$rel"
for n in 1 2 3; do
  update "$project" >/dev/null
  grep -q SMOKE_LOCAL_CUSTOMIZATION "$project/$rel"
  cmp "$work/vendor.md" "$project/.mulix/.installed/$rel"
  test ! -e "$project/.claude"
done
test -f "$project/.dsh/skills/mulix-using/SKILL.md"
# The deferred preset issue remains explicit: resolve uses embedded core,
# not the locally edited installed core. This is not claimed as a fix.
(cd "$project" && mulix preset resolve spec-template) > "$work/resolved.md"
! grep -q SMOKE_LOCAL_CUSTOMIZATION "$work/resolved.md"

printf '\n== Known entry aliases migrate and retry without deleting auxiliary files\n'
for alias in using-mulix mulix-using-mulix; do
  host=dsh
  [ "$alias" = mulix-using-mulix ] && host=claude
  legacy="$work/$alias"
  mulix init --dir "$legacy" --host "$host" >/dev/null
  old=".$host/skills/$alias/SKILL.md"
  mkdir -p "$(dirname "$legacy/$old")"
  python -I - "$repo/internal/scaffold/testdata/legacy/$alias.md" "$legacy/$old" ".$host/skills" <<'PY'
import pathlib, sys
source, target, skills = sys.argv[1:]
pathlib.Path(target).write_bytes(pathlib.Path(source).read_bytes().replace(b'{{SKILLS_DIR}}', skills.encode()))
PY
  printf 'KEEP AUXILIARY\n' > "$(dirname "$legacy/$old")/user-notes.md"
  update "$legacy" > "$work/migration.log"
  grep -q migrated "$work/migration.log"
  grep -q 'backup:' "$work/migration.log"
  test ! -e "$legacy/$old"
  grep -q 'KEEP AUXILIARY' "$(dirname "$legacy/$old")/user-notes.md"
  grep -q '^name: mulix-using' "$legacy/.$host/skills/mulix-using/SKILL.md"
  update "$legacy" >/dev/null
  if [ "$host" = dsh ]; then test ! -e "$legacy/.claude"; fi
done

printf '\n== Unknown old source stays blocked across retries\n'
unknown="$work/unknown"
mulix init --dir "$unknown" --host dsh >/dev/null
old=.dsh/skills/using-mulix/SKILL.md
mkdir -p "$(dirname "$unknown/$old")"
printf 'UNKNOWN USER ENTRY\n' > "$unknown/$old"
cp "$unknown/$old" "$work/unknown-original.md"
for n in 1 2; do
  expect_update_fail "$unknown"
  grep -q blocked "$work/failure.log"
  cmp "$work/unknown-original.md" "$unknown/$old"
  test ! -e "$unknown/.claude"
done

printf '\n== Conflict is sidecar-only; explicit acceptance keeps resolution\n'
conflict="$work/conflict"
mulix init --dir "$conflict" --host dsh >/dev/null
# Model a previously recorded vendor version using fixture metadata; current
# N remains the real embedded template. Never modify/rebuild repository assets.
python -I - "$conflict" "$rel" <<'PY'
import hashlib, json, pathlib, sys
root, rel = pathlib.Path(sys.argv[1]), sys.argv[2]
local = root / rel
vendor = local.read_bytes()
rest = vendor.split(b'\n', 1)[1]
base = b'# Old vendor header\n' + rest
(root / '.mulix/.installed' / rel).write_bytes(base)
local.write_bytes(b'# Local custom header\n' + rest)
meta = root / '.mulix/install-state.json'
st = json.loads(meta.read_text())
st['files'][rel]['vendor_sha256'] = hashlib.sha256(base).hexdigest()
meta.write_text(json.dumps(st), encoding='utf-8')
PY
cp "$conflict/$rel" "$work/local-before.md"
cp "$conflict/.mulix/.installed/$rel" "$work/base-before.md"
for n in 1 2; do
  expect_update_fail "$conflict"
  grep -q conflicted "$work/failure.log"
  cmp "$work/local-before.md" "$conflict/$rel"
  cmp "$work/base-before.md" "$conflict/.mulix/.installed/$rel"
  ! grep -q '^<<<<<<< incoming (mulix)' "$conflict/$rel"
done
python -I - "$conflict" "$rel" <<'PY'
import json, pathlib, sys
root, rel = pathlib.Path(sys.argv[1]), sys.argv[2]
st = json.loads((root / '.mulix/install-state.json').read_text())
pending = st['pending'][rel]
candidate = root / pending['candidate']
assert candidate.is_file()
assert b'<<<<<<< incoming (mulix)' in candidate.read_bytes()
assert (candidate.parent / 'incoming.md').is_file()
local = root / rel
local.write_bytes(b'# Resolved by human\n' + local.read_bytes().split(b'\n', 1)[1])
PY
cp "$conflict/$rel" "$work/accepted.md"
update "$conflict" --accept "$rel" > "$work/accept.log"
grep -q accepted "$work/accept.log"
grep -q 'backup:' "$work/accept.log"
cmp "$work/accepted.md" "$conflict/$rel"
cmp "$work/vendor.md" "$conflict/.mulix/.installed/$rel"
update "$conflict" >/dev/null
cmp "$work/accepted.md" "$conflict/$rel"
test ! -e "$conflict/.claude"
python -I - "$conflict/.mulix/install-state.json" <<'PY'
import json, pathlib, sys
assert not json.loads(pathlib.Path(sys.argv[1]).read_text()).get('pending')
PY
printf '\nsmoke-update: repeated update, aliases, blocked legacy, conflict/accept passed\n'
