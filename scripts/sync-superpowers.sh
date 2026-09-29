#!/usr/bin/env bash
# Vendor every superpowers skill into assets/superpowers/, rewritten for
# mulix:
#
#   - "superpowers:<skill>" references become "<skill>": mulix installs the
#     skills as plain project skills (.claude/skills/<skill>/), not as a
#     namespaced plugin.
#   - every artifact location moves under .mulix/.runtime/ — per change
#     (.mulix/.runtime/<change>/...) for brainstorming/planning/SDD output,
#     .mulix/.runtime/_shared/ for change-independent output.
#   - the scripts resolve <change> themselves from .mulix/active.
#
# Everything else is copied verbatim. Each patch must apply exactly once;
# upstream drift fails the sync instead of silently shipping a half-
# rewritten copy.
#
# Usage: scripts/sync-superpowers.sh <superpowers-checkout-or-plugin-dir>
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <superpowers-dir>" >&2
  exit 2
fi

src=$1
[ -d "$src/skills" ] || { echo "no skills/ directory under $src" >&2; exit 2; }

repo=$(cd "$(dirname "$0")/.." && pwd)
dest="$repo/assets/superpowers"

version=$(sed -n 's/^[[:space:]]*"version":[[:space:]]*"\([^"]*\)".*/\1/p' "$src/.claude-plugin/plugin.json" | head -n 1)
[ -n "$version" ] || { echo "cannot read version from $src/.claude-plugin/plugin.json" >&2; exit 2; }

# Empty the destination in place rather than removing it: IDE indexers on
# Windows hold directory handles open, which makes removing the directory
# itself fail with "Device or resource busy".
mkdir -p "$dest/skills"
find "$dest" -mindepth 1 -not -path "$dest/skills" -delete 2>/dev/null || true
find "$dest/skills" -mindepth 1 -delete
cp -r "$src/skills/." "$dest/skills/"
cp "$src/LICENSE" "$dest/LICENSE"
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
    s{~/\.superpowers/}{.mulix/.runtime/_shared/}g;
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

# Brainstorm server: session files go to the active change's runtime dir.
patch "$sp/brainstorming/scripts/start-server.sh" 's{  SESSION_DIR="\$\{PROJECT_DIR\}/\.mulix/\.runtime/<change>/brainstorm/\$\{SESSION_ID\}"\n}{  # mulix: resolve the active change so sessions land in its runtime dir.\n  MULIX_CHANGE="\$(tr -d "\\r\\n" < "\$\{PROJECT_DIR\}/.mulix/active" 2>/dev/null || true)"\n  RUNTIME_DIR="\$\{PROJECT_DIR\}/.mulix/.runtime/\$\{MULIX_CHANGE:-_shared\}"\n  SESSION_DIR="\$\{RUNTIME_DIR\}/brainstorm/\$\{SESSION_ID\}"\n}g'
patch "$sp/brainstorming/scripts/start-server.sh" 's{export BRAINSTORM_PORT_FILE="\$\{PROJECT_DIR\}/\.mulix/\.runtime/<change>/brainstorm/\.last-port"}{export BRAINSTORM_PORT_FILE="\$\{RUNTIME_DIR\}/brainstorm/.last-port"}g'
patch "$sp/brainstorming/scripts/start-server.sh" 's{export BRAINSTORM_TOKEN_FILE="\$\{PROJECT_DIR\}/\.mulix/\.runtime/<change>/brainstorm/\.last-token"}{export BRAINSTORM_TOKEN_FILE="\$\{RUNTIME_DIR\}/brainstorm/.last-token"}g'

# mulix's .mulix/.runtime/.gitignore already ignores brainstorm sessions;
# the upstream advice would ignore the whole change runtime dir instead.
patch "$sp/brainstorming/visual-companion.md" 's{ Remind the user to add `\.mulix/\.runtime/<change>/` to `\.gitignore` if it\x27s not already there\.}{ Session directories are already git-ignored by `.mulix/.runtime/.gitignore` (written by `mulix init`) — they hold the session key.}g'

# The server reads its version from the plugin root three levels up —
# which, once installed as a project skill, is the user's own project.
patch "$sp/brainstorming/scripts/server.cjs" "s{const SUPERPOWERS_VERSION = readSuperpowersVersion\\(\\);}{const SUPERPOWERS_VERSION = '$version'; // mulix: vendored copy, fixed version}g"

# SDD workspace: per-plan directories under the active change's runtime dir.
patch "$sp/subagent-driven-development/scripts/sdd-workspace" 's{base="\$root/\.mulix/\.runtime/<change>/sdd"\n}{# mulix: the workspace lives in the active change\x27s runtime dir. The\n# mulix root (the directory holding .mulix/) may sit below the git root.\nmulix_root=\$PWD\nwhile [ "\$mulix_root" != "/" ] && [ ! -d "\$mulix_root/.mulix" ]; do\n  mulix_root=\$(dirname "\$mulix_root")\ndone\n[ -d "\$mulix_root/.mulix" ] || mulix_root=\$root\nchange=\$(tr -d "\\r\\n" < "\$mulix_root/.mulix/active" 2>/dev/null || true)\nbase="\$mulix_root/.mulix/.runtime/\$\{change:-_shared\}/sdd"\n}g'

# Plan workspaces are the change's execution record in mulix (the build-
# complete guard reads the ledger and task reports from them), so the
# end-of-run cleanup keeps them instead of deleting them.
for f in "$sp/subagent-driven-development/SKILL.md" "$sp/executing-plans/SKILL.md"; do
  perl -pi -e '
    s{Final review clean: delete this plan\x27s workspace}{Final review clean: keep this plan\x27s workspace}g;
    s{\[Delete this plan\x27s workspace — the record now lives in git\]}{[Keep this plan\x27s workspace — mulix keeps it as the change\x27s execution record]}g;
  ' "$f"
done
patch "$sp/subagent-driven-development/SKILL.md" 's{When the final whole-branch review is clean and its fixes are merged,\ndelete this plan\x27s workspace \(`rm -rf <workspace>`\) — the git history is\nthe record now\. Sibling directories belong to other plans; leave them\nalone\.}{When the final whole-branch review is clean and its fixes are merged,\nkeep this plan\x27s workspace: under mulix it is the change\x27s execution\nrecord (ledger, briefs, reports, review packages), and the build-complete\nguard reads it. Sibling directories belong to other plans; leave them\nalone.}g'
patch "$sp/executing-plans/SKILL.md" 's{When the final review is clean and its fixes are committed, delete this\nplan\x27s workspace directory — the git history is the record now\. Sibling\ndirectories belong to other plans; leave them alone\.}{When the final review is clean and its fixes are committed, keep this\nplan\x27s workspace directory: under mulix it is the change\x27s execution\nrecord, and the build-complete guard reads it. Sibling directories belong\nto other plans; leave them alone.}g'
patch "$sp/subagent-driven-development/SKILL.md" 's{Before you delete anything, collect}{Before you finish, collect}g'
patch "$sp/executing-plans/SKILL.md" 's{Before you delete anything, collect}{Before you finish, collect}g'

# brainstorming: mulix's design phase invokes it. Its terminal states are
# replaced by mulix's phase gates; say so where the skill starts, so the
# two never read as competing instructions.
patch "$sp/brainstorming/SKILL.md" 's{\A(---\n.*?\n---\n\n# Brainstorming Ideas Into Designs\n)}{$1\n> **Under mulix** (the active change is in the `design` phase — see the\n> `mulix-design` skill): the spike path does not apply, since the change\n> already has a clarified spec; classify bounded or architectural. The\n> bounded path ends at the approved in-chat design and the architectural\n> path at the approved written spec — neither implements or invokes\n> writing-plans from here. Record the approval in mulix state and\n> transition; the `tasks` phase invokes writing-plans, and TDD\n> implementation happens only in `build`.\n}s'

# writing-plans: mulix's tasks phase invokes it to write tasks.md. Task
# sections use "## Task N" headings (the build-complete guard and the
# executors' task-brief both match any "#{2,} Task N" heading), and there
# is no Execution Handoff: the phase ends at mulix's own gate, and the
# execution method is recorded in mulix state instead of asked here.
patch "$sp/writing-plans/SKILL.md" 's{^### Task N: \[Component Name\]$}{## Task N: [Component Name]}m'
patch "$sp/writing-plans/SKILL.md" 's{\n## Execution Handoff\n.*\z}{\n}s'
patch "$sp/writing-plans/SKILL.md" 's{\*\*Save plans to:\*\* `docs/changes/<change>/YYYY-MM-DD-<feature-name>\.md`\n- \(User preferences for plan location override this default\)}{**Save plans to:** `docs/changes/<change>/tasks.md` — the mulix tasks phase\x27s artifact, which the build phase executes and the build-complete guard checks against.}g'

# Nothing may still point at the upstream locations.
if grep -rnE '\.superpowers/|docs/superpowers/|superpowers:[a-z]' "$sp"; then
  echo "sync-superpowers: upstream paths/references survived the rewrite (see above)" >&2
  exit 1
fi

echo "vendored superpowers $version into $dest ($(find "$sp" -type f | wc -l | tr -d ' ') files)"
