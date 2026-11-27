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
mulix _setup --host claude --tracker markdown >/dev/null
test -f .claude/skills/brainstorming/SKILL.md
test -f .claude/skills/mulix-using/SKILL.md
test -f .mulix/templates/issue-task-template.md
test -x .claude/skills/subagent-driven-development/scripts/sdd-workspace || [ "${OS:-}" = "Windows_NT" ]
test -f .mulix/.runtime/.gitignore

step new
mulix new "Add greeting" >/dev/null
IFS= read -r c < .mulix/active
[[ "$c" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}-add-greeting$ ]]
change_dir=".mulix/.runtime/$c"
spec="$change_dir/spec.md"
plan="$change_dir/issues"
report="$change_dir/report.md"
test -d "$plan"
test -f "$change_dir/state.yaml"
mulix new "Add greeting" >/dev/null
IFS= read -r second < .mulix/active
[ "$second" = "$c-2" ]
test -d ".mulix/.runtime/$second/issues"
test ! -e "docs/specs/$second"
test ! -e "docs/changes/$second"
mulix state select "$c" >/dev/null

step specify
expect_hook deny "src/main.go"
expect_hook deny "$change_dir/state.yaml"
expect_hook deny "docs/specs/$c/spec.md"
expect_hook allow "$spec"
cat > "$spec" <<'EOF'
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
expect_hook deny "$plan/01-greeter.md"
expect_hook deny "$spec"
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
expect_hook allow "$plan/01-greeter.md"
expect_hook deny "$plan/nested/01-greeter.md"
expect_hook deny "docs/changes/$c/tasks.md"
expect_hook deny "$change_dir/specs/2026-01-01-greeting-design.md"
cat > "$plan/01-greeter.md" <<'EOF'
## Task 1: Greeter

```mulix-tests
tasks:
  - number: 1
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
```

**Step 1: Write the failing test**

````markdown
## Task 9: an example heading inside a fence is not a task
```
- [ ] **Step 1: An example, not an executed step**
````

**Step 5: Commit (not authorized in this fixture)**
EOF
cat > "$plan/02-cli.md" <<'EOF'
## Task 2: CLI

```mulix-tests
tasks:
  - number: 2
    kind: code
cases:
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

**Step 1: Write the failing test**
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
mulix state transition tasks-complete >/dev/null

step build
expect_hook allow "src/main.go"
expect_hook deny "$plan/01-greeter.md"
expect_hook allow "$change_dir/sdd/issues/task-1-report.md"
expect_hook allow ".mulix/.runtime/$c/tests/features/task-1/api.feature"
expect_hook deny ".mulix/.runtime/$c/tests/features/task-1/run.json"
expect_hook allow "$scratch/verification/tests.log"
expect_hook deny ".mulix/.runtime/$c/verification/tests.log"
expect_hook deny ".mulix/.runtime/$c/migrations/manifest.json"
expect_hook deny ".mulix/.runtime/$c/state.yaml"
expect_hook deny ".mulix/.runtime/$c/specs/2026-01-01-greeting-design.md"
scripts=.claude/skills/subagent-driven-development/scripts
ws=$(bash "$scripts/sdd-workspace" "$plan")
case "$ws" in */.mulix/.runtime/$c/sdd/issues) ;; *) echo "FAIL: workspace $ws" >&2; exit 1 ;; esac
[ "$(< "$ws/plan-path")" = "$plan" ]
bash "$scripts/task-brief" "$plan" 1 >/dev/null
cmp "$plan/01-greeter.md" "$ws/task-1-brief.md"
expect_fail mulix state transition build-complete   # no execution method, no ledger
mulix state set execution_method inline >/dev/null
base=$(git rev-parse HEAD)
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
expect_fail bash "$done_script" "$plan" 1 "$base" -- true
expect_fail bash "$done_script" "$plan" 1 "$base" -- mulix test receipt --task 1
test ! -f "$ws/progress.md"
cmp "$work/brief-before.md" "$ws/task-1-brief.md"
grep -q 'lacks runner-generated green' "$ws/task-1-tests.log"
mulix test run --task 1 --stage green >/dev/null
ledger=$(bash "$scripts/progress-init" "$plan")
printf '\n### Task 1\n\n- Step 1: verified — observed behavioral RED.\n' >> "$ledger"
state_file="$change_dir/state.yaml"
before_receipt=$(git hash-object --no-filters "$state_file")
bash "$done_script" "$plan" 1 "$base" -- mulix test receipt --task 1 >/dev/null
[ "$before_receipt" = "$(git hash-object --no-filters "$state_file")" ]
cmp "$work/brief-before.md" "$plan/01-greeter.md"
cmp "$work/brief-before.md" "$ws/task-1-brief.md"
grep -Fxq "# SDD progress — plan: $plan" "$ledger"
[ "$(sed -n '2p' "$ws/progress.md")" = "" ]
grep -q '^## Execution context$' "$ws/progress.md"
grep -q '^## Pre-flight$' "$ws/progress.md"
grep -q '^## Task progress$' "$ws/progress.md"
grep -q '^### Task 1$' "$ws/progress.md"
receipt_id=$(mulix test receipt --task 1 --id-only)
grep -Fxq -- "- Task 1: complete (receipt $receipt_id)" "$ws/progress.md"
grep -q '^  - Evidence: \[test log\](task-1-tests.log)$' "$ws/progress.md"
# No receipt or ledger entry can be created for an unapproved task.
cp "$ws/progress.md" "$work/ledger-before.md"
expect_fail bash "$done_script" "$plan" 99 "$base" -- mulix test receipt --task 99
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
mulix test run --task 2 --stage green >/dev/null
before_receipt=$(git hash-object --no-filters "$state_file")
bash "$done_script" "$plan" 2 "$base" -- mulix test receipt --task 2 >/dev/null
[ "$before_receipt" = "$(git hash-object --no-filters "$state_file")" ]
cmp "$plan/02-cli.md" "$ws/task-2-brief.md"
mulix state transition build-complete >/dev/null

step verify
expect_hook allow "$report"
expect_hook deny "docs/changes/$c/report.md"
expect_hook allow "$scratch/verification/full-suite.log"
expect_hook deny "$change_dir/verification/full-suite.log"
expect_hook deny "$change_dir/sdd/issues/progress.md"
expect_hook deny "src/main.go"
cat > "$report" <<'EOF'
# Delivery review

- AC-001: exact Value assertion reviewed against FR-001.
- AC-002: subprocess exit/stdout assertion reviewed against FR-002.

```mulix-review
reviewer: smoke fixture review (synthetic, not human approval)
result: approved
blocking: []
```
EOF
mulix state set verify_result pass >/dev/null
expect_fail mulix state transition verify-pass  # manual flag is not evidence
mulix verify >/dev/null
grep -Fq -- "- Input \"$spec\": SHA-256 " "$report"
grep -Fq -- "- Input \"$plan/01-greeter.md\": SHA-256 " "$report"
grep -Fq -- "- Input \"$plan/02-cli.md\": SHA-256 " "$report"
mulix state transition verify-pass >/dev/null

step 'archive (local phase gates)'
expect_hook deny "$report"
expect_fail mulix state transition archived
mulix state set archive_confirmation confirmed >/dev/null
printf '\n// stale input probe\n' >> value.go
expect_fail mulix state transition archived
mulix verify >/dev/null  # revalidation in not-yet-archived archive phase
mulix state transition archived >/dev/null
test ! -f .mulix/active

step "retained runtime inventory"
test ! -e "docs/specs/$c"
test ! -e "docs/changes/$c"
test ! -e "$change_dir/tasks.md"
test ! -e "$plan/tasks.md"
test -d "$change_dir/tests"
[ -z "$(find "$change_dir/tests" -mindepth 1 -print)" ] # non-Karate fixture creates only the empty source directory
while IFS= read -r dir; do
  case "$(basename "$dir")" in specs|tests|sdd|issues) ;; *) printf 'FAIL: extra runtime directory %s\n' "$dir" >&2; exit 1 ;; esac
done < <(find "$change_dir" -mindepth 1 -maxdepth 1 -type d)

step "legacy git hygiene"
# Historical session keys must remain ignored even though no new launcher
# writes this retained path. Init/update deliberately do not delete history.
mkdir -p ".mulix/.runtime/$c/brainstorm/s1" && echo key > ".mulix/.runtime/$c/brainstorm/s1/.last-token"
git check-ignore -q ".mulix/.runtime/$c/brainstorm/s1/.last-token"
! git check-ignore -q ".mulix/.runtime/$c/state.yaml"

printf '\nsmoke-e2e: all phases passed\n'
