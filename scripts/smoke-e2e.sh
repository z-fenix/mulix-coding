#!/usr/bin/env bash
# End-to-end smoke test: install mulix into a throwaway git repo and walk
# one change through all seven phases, exercising the hook, the guards,
# and the embedded superpowers scripts as the skills would call them.
#
# Usage: scripts/smoke-e2e.sh [path-to-mulix-binary]
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
bin="$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")"
mkdir "$work/cli"
case "$(uname -s)" in MINGW*|MSYS*|CYGWIN*) cli_name=mulix.exe ;; *) cli_name=mulix ;; esac
cp "$bin" "$work/cli/$cli_name"
export PATH="$work/cli:$PATH"
unset MULIX_SESSION_ID
mulix() { "$bin" "$@"; }

mkdir "$work/project"
cd "$work/project"
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
test -f .claude/skills/mulix-using/SKILL.md
test -x .claude/skills/subagent-driven-development/scripts/sdd-workspace || [ "${OS:-}" = "Windows_NT" ]
test -f .mulix/.runtime/.gitignore

step new
mulix new "Add greeting" >/dev/null
IFS= read -r c < .mulix/active
[[ "$c" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}-add-greeting$ ]]
for directory in "docs/specs/$c" "docs/changes/$c" ".mulix/.runtime/$c"; do test -d "$directory"; done
test -f ".mulix/.runtime/$c/state.yaml"
mulix new "Add greeting" >/dev/null
IFS= read -r second < .mulix/active
[ "$second" = "$c-2" ]
for directory in "docs/specs/$second" "docs/changes/$second" ".mulix/.runtime/$second"; do test -d "$directory"; done
mulix state select "$c" >/dev/null

step specify
expect_hook deny "src/main.go"
expect_hook deny ".mulix/.runtime/$c/state.yaml"
expect_hook allow "docs/specs/$c/spec.md"
mkdir -p "docs/specs/$c"
cat > "docs/specs/$c/spec.md" <<'EOF'
# Spec

- FR-001: Value returns exactly two.
- FR-002: The greeting CLI prints hello.

```mulix-acceptance
criteria:
  - id: AC-001
    requirement: FR-001
    expected: Value returns exactly two.
  - id: AC-002
    requirement: FR-002
    expected: The CLI exits zero and stdout equals hello.
```

## Clarifications

- Q: Is a newline required? → A: No; stdout is exactly hello.
EOF
mulix state set spec_path "docs/specs/$c/spec.md" >/dev/null
mulix state transition spec-complete >/dev/null

step clarify
mulix state transition clarify-complete >/dev/null
[ "$(phase)" = design ]

step design
expect_hook allow ".mulix/.runtime/$c/specs/2026-01-01-greeting-design.md"
expect_hook deny ".mulix/.runtime/$c/brainstorm/1-2/content/a.html"
scratch=$(mulix context show --field scratch-dir)
if command -v cygpath >/dev/null 2>&1; then scratch=$(cygpath -m "$scratch"); fi
expect_hook allow "$scratch/brainstorm/session/content/a.html"
expect_hook deny "$scratch/verification/tests.log"
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

# Executors use runtime-dir; companion tools use scratch-dir. Their actual
# process/env behavior is exercised by TestSessionScripts_*.
runtime=$(mulix context show --field runtime-dir)
case "$runtime" in *"$c") ;; *) printf 'FAIL: runtime context is %s\n' "$runtime" >&2; exit 1 ;; esac

step tasks
expect_hook allow "docs/changes/$c/tasks.md"
expect_hook deny ".mulix/.runtime/$c/specs/2026-01-01-greeting-design.md"
cat > "docs/changes/$c/tasks.md" <<'EOF'
# Greeting Implementation Plan

```mulix-tests
tasks:
  - number: 1
    kind: code
  - number: 2
    kind: code
cases:
  - id: TC-001
    criterion: AC-001
    task: 1
    point: exact numeric value
    given: a fixture module
    when: Value is called
    then: the result equals two
    suite: unit
    test: fixture#TestValue
    file: value_test.go
  - id: TC-002
    criterion: AC-002
    task: 2
    point: real CLI stdout and exit status
    given: the greeting executable source
    when: the CLI is run
    then: exit zero and stdout hello
    suite: integration
    test: fixture#TestGreetingCLI
    file: cli_test.go
```

## Task 1: Greeter

- [ ] **Step 1: Write the failing test**

```markdown
## Task 9: an example heading inside a fence is not a task
- [ ] **Step 1: An example, not an executed step**
```

- [ ] **Step 5: Commit (not authorized in this fixture)**

## Task 2: CLI

- [ ] **Step 1: Write the failing test**
EOF
cat > .mulix/quality.yaml <<'EOF'
schema: mulix.quality.v1
suites:
  - id: unit
    command: [go, test, -json, -count=1, -run, '^TestValue$', .]
    cwd: .
    format: go-json
    timeout_seconds: 120
  - id: integration
    command: [go, test, -json, -count=1, -run, '^TestGreetingCLI$', .]
    cwd: .
    format: go-json
    timeout_seconds: 120
EOF
mulix state set tasks_path "docs/changes/$c/tasks.md" >/dev/null
mulix state transition tasks-complete >/dev/null

step build
expect_hook allow "src/main.go"
expect_hook allow ".mulix/.runtime/$c/sdd/tasks/task-1-report.md"
expect_hook allow "$scratch/verification/tests.log"
expect_hook deny ".mulix/.runtime/$c/verification/tests.log"
expect_hook deny ".mulix/.runtime/$c/migrations/manifest.json"
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
# Simulate only the verified Step 1 status edit in the approved source.
# The existing brief is deliberately stale; fenced examples stay unchecked.
perl -0777 -pi -e 's/\Q- [ ] **Step 1: Write the failing test**\E/- [X] **Step 1: Write the failing test**/' "docs/changes/$c/tasks.md"
cp "$ws/task-1-brief.md" "$work/brief-before.md"
printf 'module fixture\n\ngo 1.27.0\n' > go.mod
printf 'package fixture\nfunc Value() int { return 0 }\n' > value.go
cat > value_test.go <<'EOF'
package fixture
import "testing"
func TestValue(t *testing.T) {
    if Value() != 2 { t.Fatalf("Value = %d; want 2", Value()) }
}
EOF
mulix test run --task 1 --stage red >/dev/null
printf 'package fixture\nfunc Value() int { return 2 }\n' > value.go
done_script=.claude/skills/executing-plans/scripts/task-done
rc=0
bash "$done_script" "docs/changes/$c/tasks.md" 1 "$base" -- bash -c 'printf "expected failure\n"; exit 7' >/dev/null 2>&1 || rc=$?
[ "$rc" -eq 7 ]
test ! -f "$ws/progress.md"
cmp "$work/brief-before.md" "$ws/task-1-brief.md"
grep -q 'expected failure' "$ws/task-1-tests.log"
bash "$done_script" "docs/changes/$c/tasks.md" 1 "$base" -- mulix test run --task 1 --stage green >/dev/null
# Compare the entire refreshed brief, not just the presence of an X marker.
cat > "$work/brief-expected.md" <<'EOF'
## Task 1: Greeter

- [X] **Step 1: Write the failing test**

```markdown
## Task 9: an example heading inside a fence is not a task
- [ ] **Step 1: An example, not an executed step**
```

- [ ] **Step 5: Commit (not authorized in this fixture)**

EOF
cmp "$work/brief-expected.md" "$ws/task-1-brief.md"
[ "$(sed -n '2p' "$ws/progress.md")" = "" ]
grep -q '^## Execution context$' "$ws/progress.md"
grep -q '^## Pre-flight$' "$ws/progress.md"
grep -q '^## Task progress$' "$ws/progress.md"
grep -q '^### Task 1$' "$ws/progress.md"
grep -q '^- Task 1: complete$' "$ws/progress.md"
grep -q '^  - Evidence: \[test log\](task-1-tests.log)$' "$ws/progress.md"
# Successful tests cannot record a task whose brief cannot be extracted.
cp "$ws/progress.md" "$work/ledger-before.md"
expect_fail bash "$done_script" "docs/changes/$c/tasks.md" 99 "$base" -- true
cmp "$work/ledger-before.md" "$ws/progress.md"
expect_fail mulix state transition build-complete   # Task 2 not done, no reports
base=$(git rev-parse HEAD)
mkdir -p cmd/greet
printf 'package main\nimport "fmt"\nfunc main(){fmt.Print("")}\n' > cmd/greet/main.go
cat > cli_test.go <<'EOF'
package fixture
import (
    "os/exec"
    "testing"
)
func TestGreetingCLI(t *testing.T) {
    out, err := exec.Command("go", "run", "./cmd/greet").CombinedOutput()
    if err != nil || string(out) != "hello" { t.Fatalf("CLI: %v %q; want hello", err, out) }
}
EOF
mulix test run --task 2 --stage red >/dev/null
printf 'package main\nimport "fmt"\nfunc main(){fmt.Print("hello")}\n' > cmd/greet/main.go
bash .claude/skills/executing-plans/scripts/task-done "docs/changes/$c/tasks.md" 2 "$base" -- mulix test run --task 2 --stage green >/dev/null
mulix state transition build-complete >/dev/null

step verify
expect_hook allow "docs/changes/$c/report.md"
expect_hook allow "$scratch/verification/full-suite.log"
expect_hook deny ".mulix/.runtime/$c/verification/full-suite.log"
expect_hook deny ".mulix/.runtime/$c/sdd/tasks/progress.md"
expect_hook deny "src/main.go"
cat > "docs/changes/$c/report.md" <<'EOF'
# Delivery review

- AC-001: exact Value assertion reviewed against FR-001.
- AC-002: subprocess exit/stdout assertion reviewed against FR-002.

```mulix-review
reviewer: smoke fixture review (synthetic, not human approval)
result: approved
blocking: []
```
EOF
mulix state set report_path "docs/changes/$c/report.md" >/dev/null
mulix state set verify_result pass >/dev/null
expect_fail mulix state transition verify-pass  # manual flag is not evidence
mulix verify >/dev/null
mulix state transition verify-pass >/dev/null

step archive
expect_hook deny "docs/changes/$c/report.md"
expect_fail mulix state transition archived
mulix state set archive_confirmation confirmed >/dev/null
printf '\n// stale input probe\n' >> value.go
expect_fail mulix state transition archived
mulix verify >/dev/null  # revalidation in not-yet-archived archive phase
mulix state transition archived >/dev/null
test ! -f .mulix/active

step "retained runtime inventory"
while IFS= read -r dir; do
  case "$(basename "$dir")" in specs|sdd) ;; *) printf 'FAIL: extra runtime directory %s\n' "$dir" >&2; exit 1 ;; esac
done < <(find ".mulix/.runtime/$c" -mindepth 1 -maxdepth 1 -type d)

step "legacy git hygiene"
# Historical session keys must remain ignored even though no new launcher
# writes this retained path. Init/update deliberately do not delete history.
mkdir -p ".mulix/.runtime/$c/brainstorm/s1" && echo key > ".mulix/.runtime/$c/brainstorm/s1/.last-token"
git check-ignore -q ".mulix/.runtime/$c/brainstorm/s1/.last-token"
! git check-ignore -q ".mulix/.runtime/$c/state.yaml"

printf '\nsmoke-e2e: all phases passed\n'
