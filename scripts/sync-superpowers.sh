#!/usr/bin/env bash
# Vendor every superpowers skill into assets/superpowers/, rewritten for
# mulix:
#
#   - "superpowers:<skill>" references become "<skill>": mulix installs the
#     skills as plain project skills (.claude/skills/<skill>/), not as a
#     namespaced plugin.
#   - retained design/SDD artifacts move under .mulix/.runtime/<change>/;
#     plans use docs/changes/<change>/ and transient output uses system scratch.
#   - the scripts resolve runtime or scratch through mulix context show.
#
# Everything else is copied verbatim. Each patch must apply exactly once;
# upstream drift fails the sync instead of silently shipping a half-
# rewritten copy.
#
# Usage: scripts/sync-superpowers.sh <pinned-superpowers-checkout>
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <superpowers-dir>" >&2
  exit 2
fi

src=$1
[ -d "$src/skills" ] || { echo "no skills/ directory under $src" >&2; exit 2; }

repo=$(cd "$(dirname "$0")/.." && pwd)
live="$repo/assets/superpowers"
expected_version=7.0.2
expected_commit=22f0b463e7a5e579904dd6211a520321035526f7
[ -e "$src/.git" ] && [ "$(git -C "$src" rev-parse HEAD)" = "$expected_commit" ] || { printf 'expected a checkout of upstream commit %s\n' "$expected_commit" >&2; exit 2; }
version=$(git -C "$src" show "$expected_commit:.claude-plugin/plugin.json" | sed -n 's/^[[:space:]]*"version":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
[ "$version" = "$expected_version" ] || { printf 'expected upstream %s, got %s\n' "$expected_version" "${version:-unknown}" >&2; exit 2; }
git -C "$src" diff --quiet HEAD -- skills LICENSE .claude-plugin/plugin.json || { printf 'upstream inputs have local edits\n' >&2; exit 2; }

# Generate everything off the live tree. Exact-match failures are expected
# during upstream upgrades and must not leave an incomplete embedded bundle.
[ ! -L "$live" ] || { printf 'refusing linked vendor destination\n' >&2; exit 2; }
mkdir -p "$repo/assets"
staging=$(mktemp -d "$repo/assets/.superpowers-sync.XXXXXX")
dest="$staging/generated"
previous="$staging/previous"
published=0
cleanup() {
  local rc=$?
  trap - EXIT
  if [ "$published" = 0 ] && [ -e "$previous" ]; then
    if [ -e "$live" ] || ! mv -- "$previous" "$live"; then
      printf 'previous bundle retained at %s; restore it before retrying\n' "$previous" >&2
      exit 1
    fi
  fi
  rm -rf -- "$staging"
  exit "$rc"
}
trap cleanup EXIT
mkdir "$dest"
# Read the pinned tree, not the working directory: ignored files (for
# example a local note or editor output) must never become embedded assets.
git -C "$src" archive "$expected_commit" -- skills LICENSE | tar -x -C "$dest"
printf '%s\n' "$version" > "$dest/VERSION"

# Normalize line endings: the embedded copies are installed byte-for-byte,
# and a CRLF shebang line breaks the scripts under bash.
find "$dest" -type f -exec perl -pi -e 's/\r\n/\n/g' {} +

# --- generic rewrites (all text files) ---
find "$dest/skills" -type f \( -name '*.md' -o -name '*.sh' -o -name '*.js' -o -name '*.cjs' -o -name '*.html' -o -name '*.ts' -o -name '*.dot' -o -name '*.py' -o -perm -u+x \) -print0 |
  xargs -0 perl -pi -e '
    s{superpowers:([a-z][a-z-]*)}{$1}g;
    s{docs/superpowers/specs/}{.mulix/.runtime/<change>/specs/}g;
    s{docs/superpowers/plans/}{docs/changes/<change>/}g;
    s{~/\.superpowers/}{<scratch-dir>/}g;
    s{\.superpowers/}{.mulix/.runtime/<change>/}g;
  '

# patch FILE PERL_SUBSTITUTION: apply a substitution that must match
# exactly once.
patch() {
  local file=$1 expr=$2
  local count
  count=$(PATCH_EXPR="$expr" perl -0777 -ne 'my $n = eval "\$_ =~ $ENV{PATCH_EXPR}"; die $@ if $@; print $n + 0' "$file")
  if [ "$count" != "1" ]; then
    echo "sync-superpowers: patch matched $count times (want 1) in $file: $expr" >&2
    exit 1
  fi
  PATCH_EXPR="$expr" perl -0777 -pi -e 'eval "\$_ =~ $ENV{PATCH_EXPR}"; die $@ if $@' "$file"
}

sp="$dest/skills"

# Companion storage is transient. Literal blocks keep upstream drift visible
# without burying shell code inside Perl replacement-string escaping.
IFS= read -r -d '' MULIX_BRAINSTORM_OLD <<'MULIX_OLD' || true
# Generate unique session directory
SESSION_ID="$$-$(date +%s)"

if [[ -n "$PROJECT_DIR" ]]; then
  # Make the project dir absolute now: the script cds into its own directory
  # before starting the server, which would re-root a relative path there.
  # mkdir -p keeps the existing behavior of creating a missing project dir.
  if ! PROJECT_DIR="$(mkdir -p -- "$PROJECT_DIR" && CDPATH='' cd -- "$PROJECT_DIR" && pwd)"; then
    echo "{\"error\": \"Cannot create or enter --project-dir\"}"
    exit 1
  fi
  SESSION_DIR="${PROJECT_DIR}/.mulix/.runtime/<change>/brainstorm/${SESSION_ID}"
  # Persist the bound port and key per project so a restart reuses them and an
  # already-open browser tab reconnects to the same URL with a valid cookie.
  export BRAINSTORM_PORT_FILE="${PROJECT_DIR}/.mulix/.runtime/<change>/brainstorm/.last-port"
  export BRAINSTORM_TOKEN_FILE="${PROJECT_DIR}/.mulix/.runtime/<change>/brainstorm/.last-token"
else
  SESSION_DIR="/tmp/brainstorm-${SESSION_ID}"
fi
MULIX_OLD
IFS= read -r -d '' MULIX_BRAINSTORM_NEW <<'MULIX_NEW' || true
# Reuse only private, non-symlink directories: umask alone cannot secure
# an existing directory in a shared system temp root.
private_dir() {
  local dir=$1
  if [[ -L "$dir" || ( -e "$dir" && ! -d "$dir" ) ]]; then
    printf 'refusing unsafe scratch directory: %s\n' "$dir" >&2
    return 1
  fi
  if [[ ! -d "$dir" ]]; then
    mkdir -m 700 "$dir" 2>/dev/null || [[ -d "$dir" && ! -L "$dir" ]] || return 1
  fi
  if [[ -L "$dir" || ! -O "$dir" ]]; then
    printf 'scratch directory is not owned by the current user: %s\n' "$dir" >&2
    return 1
  fi
  chmod 700 "$dir" || return 1
}

require_absolute_scratch() {
  case "$1" in
    /*|[a-zA-Z]:/*) return 0 ;;
  esac
  printf 'scratch directory must be absolute: %s\n' "$1" >&2
  return 1
}

if [[ -n "$PROJECT_DIR" ]]; then
  # One successful context snapshot; errors never become shared output.
  SCRATCH_DIR="$(cd "$PROJECT_DIR" && mulix context show --field scratch-dir)" || exit 1
  [[ -n "$SCRATCH_DIR" ]] || { printf 'empty scratch directory\n' >&2; exit 1; }
  if command -v cygpath >/dev/null 2>&1; then
    SCRATCH_DIR="$(cygpath -m "$SCRATCH_DIR")" || exit 1
  fi
  require_absolute_scratch "$SCRATCH_DIR" || exit 1
  private_dir "$SCRATCH_DIR" || exit 1
  private_dir "$SCRATCH_DIR/brainstorm" || exit 1
  SESSION_DIR="$(mktemp -d "$SCRATCH_DIR/brainstorm/session.XXXXXX")" || exit 1
  # Reuse the port and key only within this project/change/session context.
  export BRAINSTORM_PORT_FILE="$SCRATCH_DIR/brainstorm/.last-port"
  export BRAINSTORM_TOKEN_FILE="$SCRATCH_DIR/brainstorm/.last-token"
else
  TEMP_ROOT="${TMPDIR:-/tmp}"
  require_absolute_scratch "$TEMP_ROOT" || exit 1
  SESSION_DIR="$(mktemp -d "$TEMP_ROOT/mulix-brainstorm.XXXXXX")" || exit 1
fi
MULIX_NEW
export MULIX_BRAINSTORM_OLD MULIX_BRAINSTORM_NEW
patch "$sp/brainstorming/scripts/start-server.sh" 's{\Q$ENV{MULIX_BRAINSTORM_OLD}\E}{$ENV{MULIX_BRAINSTORM_NEW}}g'
unset MULIX_BRAINSTORM_OLD MULIX_BRAINSTORM_NEW
patch "$sp/brainstorming/scripts/start-server.sh" 's{Store session files under <path>/\.mulix/\.runtime/<change>/brainstorm/}{Resolve context from <path>; store session files in system scratch}g'
patch "$sp/brainstorming/scripts/stop-server.sh" 's{Persistent directories \(\.mulix/\.runtime/<change>/\) are\n# kept so mockups can be reviewed later\.}{Legacy persistent directories are kept. New sessions\n# use system scratch; only /tmp sessions are deleted here.}g'

find "$sp" -name '*.md' -print0 | xargs -0 perl -pi -e '
  s{(?:/path/to/project/|<project>/)?\.mulix/\.runtime/<change>/brainstorm/}{<scratch-dir>/brainstorm/}g;
  s{/12345-1706000000/}{/session.A1b2C3/}g;
'
patch "$sp/brainstorming/visual-companion.md" 's{ When using `--project-dir`, check `<scratch-dir>/brainstorm/` for the session directory\.}{ Use the saved `screen_dir` and `state_dir`; if needed, query `mulix context show --field scratch-dir` from the same project and session, then inspect its `brainstorm/` directory.}g'
patch "$sp/brainstorming/visual-companion.md" 's{\*\*Note:\*\* Pass the project root as `--project-dir`[^\n]*}{**Note:** Pass the project root as `--project-dir` so mulix resolves the current context and stores all companion files in private system scratch. No current change is required; an invalid context stops startup. Capture the startup JSON paths: temporary files may be cleaned by the system and are not part of the archived change.}g'
patch "$sp/brainstorming/visual-companion.md" 's{If the session used `--project-dir`, mockup files persist in `<scratch-dir>/brainstorm/` for later reference\. Only `/tmp` sessions get deleted on stop\.}{Companion files are temporary, not retained runtime artifacts. The stop script removes sessions under `/tmp`; other system-temp locations may remain until system cleanup. Historical persistent directories are left untouched.}g'
patch "$sp/diagnosing-superpowers/SKILL.md" 's{Enumerate subagent transcripts\. Create\n   `<scratch-dir>/diagnosing-superpowers/<session-id>/`, tell your}{Enumerate subagent transcripts. First query\n   `mulix context show --field scratch-dir` from the project with the current\n   session identity; stop on an error, including an invalid binding. The query\n   works without a current change. Use its absolute path as `<scratch-dir>`,\n   create `diagnosing-superpowers/<session-id>/` there with owner-only\n   permissions (`umask 077`), tell your}g'

# The server reads its version from the plugin root three levels up —
# which, once installed as a project skill, is the user's own project.
patch "$sp/brainstorming/scripts/server.cjs" "s{const SUPERPOWERS_VERSION = readSuperpowersVersion\\(\\);}{const SUPERPOWERS_VERSION = '$version'; // mulix: vendored copy, fixed version}g"

# sdd-workspace is now a canonical Mulix overlay: it owns both the pinned
# directory layout and the historical single-file marker rules. Apply the
# native brief/review adapters to the corresponding upstream commands.
IFS= read -r -d '' MULIX_NATIVE_BRIEF <<'MULIX_NATIVE' || true
if [ $# -eq 3 ]; then
  # The standalone legacy extractor still accepts OUTFILE outside Mulix.
  # In a Mulix project, don't let that argument bypass the native plan.
  probe=$PWD
  while [ ! -d "$probe/.mulix" ]; do
    parent=$(dirname "$probe")
    [ "$parent" != "$probe" ] || break
    probe=$parent
  done
  approved_dir=""
  if [ -d "$probe/.mulix" ]; then
    approved_dir=$(mulix context show --field tasks-dir) || exit 1
  fi
  if [ -n "$approved_dir" ] || [ -d "$plan" ]; then
    printf 'task-brief: native task briefs are written only in their SDD workspace\n' >&2
    exit 2
  fi
elif [ -d "$plan" ]; then
  case "$n" in ''|0*|*[!0-9]*) printf 'task-brief: expected a positive task number\n' >&2; exit 2 ;; esac
  dir=$("${BASH:-bash}" "$here/sdd-workspace" "$plan")
  out="$dir/task-${n}-brief.md"
  if [ -L "$out" ] || { [ -e "$out" ] && [ ! -f "$out" ]; }; then
    printf 'task-brief: refusing unsafe brief path %s\n' "$out" >&2
    exit 2
  fi
  tmp=$(mktemp "$dir/.task-brief.XXXXXX")
  trap 'rm -f -- "$tmp"' EXIT
  mulix _task-input "$n" --plan-path "$plan" > "$tmp"
  mv -f -- "$tmp" "$out"
  printf 'wrote %s: %s lines\n' "$out" "$(wc -l < "$out" | tr -d ' ')"
  exit 0
fi
MULIX_NATIVE
IFS= read -r -d '' MULIX_NATIVE_REVIEW <<'MULIX_NATIVE' || true
native_plan=false
if [ -d "$plan" ]; then
  native_plan=true
  if [ $# -eq 4 ]; then
    printf 'review-package: native review packages are written only in their SDD workspace\n' >&2
    exit 2
  fi
else
  [ -f "$plan" ] || { echo "no such plan file: $plan" >&2; exit 2; }
fi
MULIX_NATIVE
IFS= read -r -d '' MULIX_REVIEW_TARGET <<'MULIX_NATIVE' || true
target=$out
if [ "$native_plan" = true ]; then
  check_output() {
    if [ -L "$out" ] || { [ -e "$out" ] && [ ! -f "$out" ]; }; then
      printf 'review-package: refusing unsafe review output %s\n' "$out" >&2
      exit 2
    fi
  }
  check_output
  target=$(mktemp "$dir/.review-package.XXXXXX")
  trap 'rm -f -- "$target"' EXIT
fi

MULIX_NATIVE
export MULIX_NATIVE_BRIEF MULIX_NATIVE_REVIEW MULIX_REVIEW_TARGET
sdd="$sp/subagent-driven-development/scripts"
patch "$sdd/task-brief" 's{(\nplan=\$1\nn=\$2\n)(?=\[ -f "\$plan" \])}{$1$ENV{MULIX_NATIVE_BRIEF}}'
patch "$sdd/review-package" 's{(\nhead=\$3\n)[^\n]*no such plan file[^\n]*\n}{$1$ENV{MULIX_NATIVE_REVIEW}}'
patch "$sdd/review-package" 's{(?=^\{\n  echo "# Review package:)}{$ENV{MULIX_REVIEW_TARGET}}m'
patch "$sdd/review-package" 's{^\} > "\$out"$}{\} > "\$target"\nif [ "\$native_plan" = true ]; then\n  check_output\n  mv -f -- "\$target" "\$out"\nfi}m'
unset MULIX_NATIVE_BRIEF MULIX_NATIVE_REVIEW MULIX_REVIEW_TARGET
for f in "$sdd/task-brief" "$sdd/review-package"; do
  patch "$f" 's{^(# Usage: [^\n]*)PLAN_FILE ([^\n]*)\[OUTFILE\]$}{${1}PLAN_PATH ${2}[OUTFILE (legacy only)]}m'
  patch "$f" 's{^(  echo "usage: [^\n]*)PLAN_FILE ([^\n]*)\[OUTFILE\]" >&2$}{${1}PLAN_PATH ${2}[OUTFILE (legacy only)]" >&2}m'
done
patch "$sp/executing-plans/scripts/task-start" 's{^# Usage: task-start PLAN_FILE TASK_NUMBER$}{# Usage: task-start PLAN_PATH TASK_NUMBER}m'
patch "$sp/executing-plans/scripts/task-start" 's{usage: task-start PLAN_FILE TASK_NUMBER}{usage: task-start PLAN_PATH TASK_NUMBER}'
patch "$sp/executing-plans/scripts/task-start" 's{^out=\$\("\$sdd/task-brief" "\$plan" "\$n"\)$}{out=\$("\${BASH:-bash}" "\$sdd/task-brief" "\$plan" "\$n")}m'

# Plan workspaces are the change's execution record in mulix (the build-
# complete guard reads the ledger and task reports from them), so the
# end-of-run cleanup keeps them instead of deleting them.
IFS= read -r -d '' MULIX_PLAN_CONTEXT <<'MULIX_CONTEXT' || true
**Under Mulix**, `PLAN_FILE` is the approved plan path. Use the whole
`issues/` directory returned by `mulix context show --field tasks-dir` in
this session for a native change; a legacy change uses its `tasks.md` file.
For native changes, read context from `spec.md` and the numbered task
documents. Pass the same directory to all task and review helpers; there
is no combined plan file. Keep step progress in the ledger.
MULIX_CONTEXT
export MULIX_PLAN_CONTEXT
for f in "$sp/subagent-driven-development/SKILL.md" "$sp/executing-plans/SKILL.md"; do
  patch "$f" 's{^(## Setup\n\n)}{${1}$ENV{MULIX_PLAN_CONTEXT}\n}m'
  patch "$f" 's{<repo-root>(?=/\.mulix/\.runtime/<change>/sdd)}{<mulix-root>}g'
  perl -pi -e '
    s{Final review clean: delete this plan\x27s workspace}{Final review clean: keep this plan\x27s workspace}g;
    s{\[Delete this plan\x27s workspace — the record now lives in git\]}{[Keep this plan\x27s workspace — mulix keeps it as the change\x27s execution record]}g;
  ' "$f"
done
unset MULIX_PLAN_CONTEXT
patch "$sp/subagent-driven-development/references/final-review.md" 's{When the final whole-branch review is clean and its fixes are merged,\ndelete this plan\x27s workspace \(`rm -rf <workspace>`\) — the git history is\nthe record now\. Sibling directories belong to other plans; leave them\nalone\.}{When the final whole-branch review is clean and its fixes are merged,\nkeep this plan\x27s workspace: under mulix it is the change\x27s execution\nrecord (ledger, briefs, reports, review packages), and the build-complete\nguard reads it. Sibling directories belong to other plans; leave them\nalone.}g'
patch "$sp/executing-plans/SKILL.md" 's{When the final review is clean and its fixes are committed, delete this\nplan\x27s workspace directory — the git history is the record now\. Sibling\ndirectories belong to other plans; leave them alone\.}{When the final review is clean and its fixes are committed, keep this\nplan\x27s workspace directory: under mulix it is the change\x27s execution\nrecord, and the build-complete guard reads it. Sibling directories belong\nto other plans; leave them alone.}g'
patch "$sp/subagent-driven-development/references/final-review.md" 's{Before you delete anything, collect}{Before you finish, collect}g'
patch "$sp/executing-plans/SKILL.md" 's{Before you delete anything, collect}{Before you finish, collect}g'

# brainstorming: mulix's design phase invokes it. Its terminal states are
# replaced by mulix's phase gates; say so where the skill starts, so the
# two never read as competing instructions.
patch "$sp/brainstorming/SKILL.md" 's{\A(---\n.*?\n---\n\n# Brainstorming\n)}{$1\n> **Under mulix** (the active change is in the `design` phase — see the\n> `mulix-design` skill): the change already has a clarified spec, so the\n> "quick, clear task" size never applies and the skip-the-questions\n> opt-out is not available. Size the work **bounded** (a small change:\n> the in-chat description is the design) or **architectural** (a\n> project: the written design document), and record the size in mulix\n> state. Spikes are out for the same reason — this change\x27s output\n> ships. The HARD-GATE below still holds, but the approval is recorded\n> in mulix state and the phase ends there: neither size builds anything\n> or invokes writing-plans from here. The `tasks` phase invokes\n> writing-plans, and TDD implementation happens only in `build`.\n}s'

# writing-plans: mulix's tasks phase invokes it to write tasks.md. Task
# sections use "## Task N" headings (the build-complete guard and the
# executors' task-brief both match any "#{2,} Task N" heading), and there
# is no Execution Handoff: the phase ends at mulix's own gate, and the
# execution method is recorded in mulix state instead of asked here.
patch "$sp/writing-plans/SKILL.md" 's{^### Task N: \[Component Name\]$}{## Task N: [Component Name]}m'
patch "$sp/writing-plans/SKILL.md" 's{\n## Execution Handoff\n.*\z}{\n}s'
patch "$sp/writing-plans/SKILL.md" 's{\*\*Save plans to:\*\* `docs/changes/<change>/YYYY-MM-DD-<feature-name>\.md`\n- \(User preferences for plan location override this default\)}{**Save plans to:** `docs/changes/<change>/tasks.md` — the mulix tasks phase\x27s artifact, which the build phase executes and the build-complete guard checks against.}g'

# Executor status belongs to the ledger for directory plans, but legacy
# tasks.md still supports its status-only edits and refreshed briefs.
patch "$sp/subagent-driven-development/SKILL.md" 's{task fits in the project; \(2\) the brief path}{task fits in the project; the approved PLAN_FILE path and task number\n  for step tracking; (2) the brief path}g'
IFS= read -r -d '' MULIX_SDD_COMPLETE <<'MULIX_STATUS' || true
ruling at the cap — refresh this task's brief from the approved plan first:
`bash scripts/task-brief PLAN_FILE N` (from this skill's directory).
For a native task directory, record the implementer's observed step results
in its section of `progress.md`. Leave task documents unchanged. For a legacy
`tasks.md`, the implementer marks only verified steps in that file; preserve
skipped steps and fenced examples. If extraction fails, resolve it before
recording completion.

For a native directory, run
`mulix test receipt --task N --plan-path "$PLAN_FILE" --id-only`.
Only on success, use the exact returned ID in `- Task N: complete (receipt <id>)`.
Record review status and other details under that line. A new runner receipt
needs its own closure; keep previous entries as history.

For a legacy `tasks.md`, run `mulix test receipt --task N` and append one of
these items only on success:
MULIX_STATUS
IFS= read -r -d '' MULIX_IMPLEMENTER_STATUS <<'MULIX_STATUS' || true
    Approved plan: [PLAN_FILE]; task number: [N].
    Native task directory: keep the approved task Markdown unchanged. In your
    report list each verified step as `- Step N: verified — <observed result>`
    for the controller to record in this task's `progress.md` section.
    Legacy tasks.md: after a step actually runs and matches its expected
    result, change only that step's marker in this task from `- [ ]` to `- [X]`,
    then refresh the brief with
    `bash <skills-dir>/subagent-driven-development/scripts/task-brief PLAN_FILE N`.
    Leave unperformed steps and fenced checkbox examples unchanged. Report
    skipped steps explicitly; GREEN is not evidence every step ran.
MULIX_STATUS
export MULIX_SDD_COMPLETE MULIX_IMPLEMENTER_STATUS
patch "$sp/subagent-driven-development/SKILL.md" 's{ruling at the cap — append the completion line to the ledger in the same\nmessage as your other bookkeeping:\n}{$ENV{MULIX_SDD_COMPLETE}}g'
patch "$sp/subagent-driven-development/implementer-prompt.md" 's{(    It contains the full task text from the plan\.\n)}{${1}$ENV{MULIX_IMPLEMENTER_STATUS}}g'
unset MULIX_SDD_COMPLETE MULIX_IMPLEMENTER_STATUS
patch "$sp/executing-plans/SKILL.md" 's{tests, keeps the full output in the workspace, prints the tail, and — only\nif they pass — appends the completion line to the ledger:\n\n`Task <N>: complete \(commits <base7>\.\.<head7>, tests: <command> → <result>\)`\n}{tests, keeps the full output in the workspace, and prints the tail. Only\nif they pass, it refreshes this task\x27s brief from the approved plan via\n`task-brief`, then appends the completion list item to the ledger:\n\nNative completion is `- Task N: complete (receipt <id>)`, with the exact\nvalidated receipt ID. Legacy completion keeps `- Task N: complete`.\nThe command, result, and commit range follow as separate ledger items.\n\nFor a native task directory, record verified steps in that task\x27s ledger\nsection, without editing task Markdown. For a legacy `tasks.md`, mark only\nverified steps in that file; skipped steps and fenced examples keep their\noriginal markers. A failed receipt check or brief refresh records no\ncompletion; resolve it before continuing.\n}g'

# Inline uses the already recorded native GREEN/CHECK for closure. Keep
# the helper's generic argv contract, but hand it the read-only check.
patch "$sp/executing-plans/SKILL.md" 's{- The final test run for the task passed — `task-done` is that run, and\n  it writes the command and result into the ledger line\.}{- The task\x27s `mulix test run` GREEN/CHECK passed. At closure, `task-done`\n  verifies its existing receipt and records the check; do not rerun the suite.}g'
patch "$sp/executing-plans/SKILL.md" 's{Run this skill\x27s `scripts/task-done PLAN_FILE N BASE -- <test command>`\nby its absolute path, from the working tree you\x27re executing in, with the test command the brief names for the whole task\. It runs the\ntests, keeps the full output in the workspace, and prints the tail\. Only\nif they pass, it refreshes this task\x27s brief from the approved plan via\n`task-brief`, then appends the completion list item to the ledger:}{Run `bash <skills-dir>/executing-plans/scripts/task-done PLAN_FILE N BASE -- mulix test receipt --task N`\nfrom the project root after the planned GREEN/CHECK has executed. The helper\nkeeps the receipt-check output in the workspace. Both the `task-brief`\nrefresh and the receipt check must succeed before it appends the completion\nlist item to the ledger:}g'
perl -pi -e 's{task-done: run tests, ledger the result; mark todo complete}{task-done: check existing receipt; mark todo complete}g' "$sp/executing-plans/SKILL.md"
# Only the standard task-done log is retained; ad-hoc output is temporary.
patch "$sp/executing-plans/SKILL.md" 's{context for the rest of the session\. Redirect long test output to a file\nin the workspace and read its tail; read a brief, not the whole plan\.}{context for the rest of the session. Under mulix, query\n`mulix context show --field scratch-dir` with the current identity and put\nad-hoc RED, rerun, debug and build output in its `verification/` directory.\nKeep command/result summaries and relevant RED/GREEN evidence in the existing\ntask report and ledger. Only task-done maintains `task-N-tests.log` in the\nretained workspace. Read a brief, not the whole plan.}g'

# Bind resumed progress to the plan path, not a basename for native plans.
IFS= read -r -d '' MULIX_INLINE_RESUME <<'MULIX_RESUME' || true
- Check for this plan's ledger at `<workspace>/progress.md`. If its first
  line names your plan path, resume at the first task without current closure.
  Native closure names the latest GREEN/CHECK receipt. A retry or changed
  task definitions can make old entries historical. Legacy `- Task <N>: complete`
  records still count in legacy plans. After compaction, use the ledger and
  `git log` rather than recollection.
  Leave a ledger that names another plan untouched.
MULIX_RESUME
IFS= read -r -d '' MULIX_SDD_RESUME <<'MULIX_RESUME' || true
If the first line names the approved plan path, resume at the first task
without current closure. For native plans the identity is the Mulix-root-relative
`issues/` directory; a `- Task N: complete (receipt <id>)` line must name the
latest valid task receipt. Later tasks can advance the source without undoing
that closure. A superseded receipt or changed task definitions make previous
entries historical. Legacy plans still accept their original completion lines.

If the last line for a task is a fix round, resume at the next round. Leave
ledgers that name another plan untouched, and never adopt the old flat
`sdd/progress.md` for a native task directory.
MULIX_RESUME
export MULIX_INLINE_RESUME MULIX_SDD_RESUME
patch "$sp/executing-plans/SKILL.md" 's{- Check for this plan\x27s ledger at `<workspace>/progress\.md`\. If its first\n  line names your plan file, tasks with a `Task <N>: complete` line are\n  DONE — do not redo them; resume at the first task without one\. Their\n  commits exist in git even when your context no longer remembers making\n  them: after compaction, trust the ledger and `git log` over your own\n  recollection\. A ledger whose first line names a different plan file is\n  another plan\x27s progress: leave it and start your own, fresh\.\n}{$ENV{MULIX_INLINE_RESUME}}'
patch "$sp/subagent-driven-development/references/resuming.md" 's{^If the ledger\x27s first line names your plan file,.*\z}{$ENV{MULIX_SDD_RESUME}}ms'
unset MULIX_INLINE_RESUME MULIX_SDD_RESUME

# Runtime ledger tools and templates are mulix-owned. Apply their canonical
# overlay after upstream rewrites so future syncs keep the sectioned format.
for skill in executing-plans subagent-driven-development; do
  patch "$sp/$skill/SKILL.md" 's{- Create the ledger with its identity as the first line:\n  `# SDD ledger — plan: <plan file path>`\.}{- Initialize the ledger with `bash ../subagent-driven-development/scripts/progress-init PLAN_FILE`\n  (path relative to this skill directory), then follow\n  [the shared ledger format](../subagent-driven-development/progress-format.md).\n  It preserves the first-line identity and all existing history. Native\n  closure requires a receipt ID; plain completion lines count only for legacy plans.}g'
done
cp -R "$repo/scripts/superpowers-overrides/." "$sp/"

# Normalize completion/ruling examples only after exact resume-block patches.
for f in "$sp/subagent-driven-development/SKILL.md" "$sp/executing-plans/SKILL.md"; do
  perl -pi -e 's{`Task (<N>|[0-9]+):}{`- Task $1:}g; s{(ledger: |Ledger: )Task ([0-9]+):}{$1- Task $2:}g' "$f"
done

# Nothing may still point at the upstream locations.
if grep -rnE '\.superpowers/|docs/superpowers/|superpowers:[a-z]' "$sp"; then
  echo "sync-superpowers: upstream paths/references survived the rewrite (see above)" >&2
  exit 1
fi

files=$(find "$sp" -type f | wc -l | tr -d ' ')
if [ -e "$live" ]; then
  mv -- "$live" "$previous"
fi
[ ! -e "$live" ] && [ ! -L "$live" ] || { printf 'vendor destination appeared during sync\n' >&2; exit 1; }
mv -- "$dest" "$live"
published=1
printf 'vendored superpowers %s into %s (%s files)\n' "$version" "$live" "$files"
