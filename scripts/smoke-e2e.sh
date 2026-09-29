#!/usr/bin/env bash
# End-to-end smoke test: install mulix into a throwaway git repo and walk
# one change through all seven phases, exercising the hook, the guards,
# and the embedded superpowers scripts as the skills would call them.
#
# Usage: scripts/smoke-e2e.sh [path-to-mulix-binary]
set -euo pipefail
[ -n "${SMOKE_TRACE:-}" ] && set -x

repo=$(cd "$(dirname "$0")/.." && pwd)
bin=${1:-}
if [ -z "$bin" ]; then
  bin="$(mktemp -d)/mulix"
  (cd "$repo" && go build -o "$bin" ./cmd/mulix)
fi
mulix() { "$bin" "$@"; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cd "$work"
git init -q && git config user.email smoke@example.com && git config user.name smoke
git commit -q --allow-empty -m init

step() { printf '\n== %s\n' "$*"; }
expect_fail() { if "$@" >/dev/null 2>&1; then echo "FAIL: expected failure: $*" >&2; exit 1; fi; }

# hook_decision <tool> <path>: run the PreToolUse hook, print allow|deny.
hook_decision() {
  # The hook exits 2 on deny, by contract — capture, don't propagate.
  local out
  out=$(printf '{"hook_event_name":"PreToolUse","tool_name":"%s","tool_input":{"file_path":"%s"}}' "$1" "$2" \
    | mulix hook 2>/dev/null) || true
  printf '%s\n' "$out" | sed -n 's/.*"permissionDecision":"\([a-z]*\)".*/\1/p'
}
expect_hook() {
  local got
  got=$(hook_decision Write "$2")
  [ "$got" = "$1" ] || { echo "FAIL: hook on $2 in phase $(phase): got $got, want $1" >&2; exit 1; }
}
phase() { mulix state show | sed -n 's/^phase: *//p'; }

step init
mulix init >/dev/null
test -f .claude/skills/brainstorming/SKILL.md
test -f .claude/skills/using-mulix/SKILL.md
test -x .claude/skills/subagent-driven-development/scripts/sdd-workspace || [ "${OS:-}" = "Windows_NT" ]
test -f .mulix/.runtime/.gitignore

step new
mulix new "Add greeting" >/dev/null
c=001-add-greeting
test -f ".mulix/.runtime/$c/state.yaml"
[ "$(cat .mulix/active)" = "$c" ]

step specify
expect_hook deny "src/main.go"
expect_hook deny ".mulix/.runtime/$c/state.yaml"
expect_hook allow "docs/specs/$c/spec.md"
mkdir -p "docs/specs/$c"
printf '# Spec\n\n## Clarifications\n\n- Q: x → A: y\n' > "docs/specs/$c/spec.md"
mulix state set spec_path "docs/specs/$c/spec.md" >/dev/null
mulix state transition spec-complete >/dev/null

step clarify
mulix state transition clarify-complete >/dev/null
[ "$(phase)" = design ]

step design
expect_hook allow ".mulix/.runtime/$c/specs/2026-01-01-greeting-design.md"
expect_hook allow ".mulix/.runtime/$c/brainstorm/1-2/content/a.html"
expect_hook deny "docs/changes/$c/tasks.md"
expect_hook deny "docs/specs/$c/spec.md"
expect_fail mulix state transition design-approved
mulix state set design_track architectural >/dev/null
mkdir -p ".mulix/.runtime/$c/specs"
echo "# Design" > ".mulix/.runtime/$c/specs/2026-01-01-greeting-design.md"
mulix state set design_path ".mulix/.runtime/$c/specs/2026-01-01-greeting-design.md" >/dev/null
expect_fail mulix state transition design-approved   # not approved yet
mulix state set design_approved true >/dev/null
mulix state transition design-approved >/dev/null

# The brainstorm server's session dir resolves to the active change.
sp=.claude/skills/brainstorming/scripts/start-server.sh
grep -q 'RUNTIME_DIR="${PROJECT_DIR}/.mulix/.runtime/${MULIX_CHANGE:-_shared}"' "$sp"

step tasks
expect_hook allow "docs/changes/$c/tasks.md"
expect_hook deny ".mulix/.runtime/$c/specs/2026-01-01-greeting-design.md"
cat > "docs/changes/$c/tasks.md" <<'EOF'
# Greeting Implementation Plan

## Task 1: Greeter

- [ ] **Step 1: Write the failing test**

```markdown
## Task 9: an example heading inside a fence is not a task
```

## Task 2: CLI

- [ ] **Step 1: Write the failing test**
EOF
mulix state set tasks_path "docs/changes/$c/tasks.md" >/dev/null
mulix state transition tasks-complete >/dev/null

step build
expect_hook allow "src/main.go"
expect_hook allow ".mulix/.runtime/$c/sdd/tasks/task-1-report.md"
expect_hook deny ".mulix/.runtime/$c/state.yaml"
expect_hook deny ".mulix/.runtime/$c/specs/2026-01-01-greeting-design.md"
scripts=.claude/skills/subagent-driven-development/scripts
ws=$(bash "$scripts/sdd-workspace" "docs/changes/$c/tasks.md")
case "$ws" in */.mulix/.runtime/$c/sdd/tasks) ;; *) echo "FAIL: workspace $ws" >&2; exit 1 ;; esac
bash "$scripts/task-brief" "docs/changes/$c/tasks.md" 1 >/dev/null
grep -q "^## Task 1: Greeter" "$ws/task-1-brief.md"
grep -q "Task 9" "$ws/task-1-brief.md"   # fenced example stays inside Task 1's brief
expect_fail mulix state transition build-complete   # no execution method, no ledger
mulix state set execution_method inline >/dev/null
base=$(git rev-parse HEAD)
echo "package main" > main.go && git add main.go && git commit -qm "task 1"
bash .claude/skills/executing-plans/scripts/task-done "docs/changes/$c/tasks.md" 1 "$base" -- echo "ok  greet  0.01s" >/dev/null
expect_fail mulix state transition build-complete   # Task 2 not done, no reports
base=$(git rev-parse HEAD)
echo "// cli" >> main.go && git commit -qam "task 2"
bash .claude/skills/executing-plans/scripts/task-done "docs/changes/$c/tasks.md" 2 "$base" -- echo "ok  greet  0.01s" >/dev/null
for n in 1 2; do
  printf -- '- RED: go test ./... → FAIL: undefined (expected)\n- GREEN: go test ./... → ok\n' > "$ws/task-$n-report.md"
done
mulix state transition build-complete >/dev/null

step verify
expect_hook allow "docs/changes/$c/report.md"
expect_hook deny "src/main.go"
echo "all green" > "docs/changes/$c/report.md"
mulix state set report_path "docs/changes/$c/report.md" >/dev/null
mulix state set verify_result pass >/dev/null
mulix state transition verify-pass >/dev/null

step archive
expect_hook deny "docs/changes/$c/report.md"
expect_fail mulix state transition archived
mulix state set archive_confirmation confirmed >/dev/null
mulix state transition archived >/dev/null
test ! -f .mulix/active

step "git hygiene"
mkdir -p ".mulix/.runtime/$c/brainstorm/s1" && echo key > ".mulix/.runtime/$c/brainstorm/s1/.last-token"
git check-ignore -q ".mulix/.runtime/$c/brainstorm/s1/.last-token"
! git check-ignore -q ".mulix/.runtime/$c/state.yaml"

printf '\nsmoke-e2e: all phases passed\n'
