# mulix-coding

A Go CLI that brings spec-driven development, an enforced phase state
machine, and the full superpowers skill set together for **Claude Code**
and **DeepSeek Harness**. It borrows from four existing projects:

- **spec-kit** — the constitution → specify → clarify front half of the
  workflow, its spec template, preset system, and taskstoissues.
- **comet** — the phase state machine, guard-check gating, and (the core
  enforcement mechanism) a Claude Code `PreToolUse` hook that blocks
  `Write`/`Edit` calls that don't match the current phase.
- **superpowers** — embedded whole: every skill ships inside mulix and
  runs the design → tasks → build → verify → archive half of the
  workflow (brainstorming, writing-plans, test-driven-development,
  subagent-driven-development / executing-plans,
  verification-before-completion, finishing-a-development-branch), with
  its artifacts under `.mulix/.runtime/`.
- **caveman** — the discipline of writing `SKILL.md` `description` fields
  as trigger conditions only (never a process summary, so the model can't
  use the summary as an excuse to skip loading the real content).

**Phase 1 scope** was Claude Code only: no other agent host, no caveman
compression engine/BM25/browse infrastructure, no comet Native
workflow/dashboard, no spec-kit self-update. DeepSeek Harness arrived
later as a second agent host (Phase 14) — see [Install into a
project](#install-into-a-project). The rest of that list is still out of
scope; [`docs/PLAN.md`](docs/PLAN.md) has the full phase breakdown.

mulix does port spec-kit's **preset** system (a template-override stack)
and **taskstoissues** (GitHub issue conversion) — see below.

## The workflow

```
specify → clarify → design → tasks → build → verify → archive
```

| Phase   | What happens | Artifact |
|---------|--------------|----------|
| specify | WHAT/WHY, no design | `docs/specs/<change>/spec.md` |
| clarify | up to 5 targeted questions | spec.md `## Clarifications` |
| design  | `brainstorming`: classify (bounded / architectural), design, explicit approval | `.mulix/.runtime/<change>/specs/*-design.md` (architectural) |
| tasks   | `writing-plans`: the approved design becomes an executable plan, self-reviewed, then approved by the human | `docs/changes/<change>/tasks.md` (`## Task N` sections) |
| build   | `test-driven-development`, executed by `subagent-driven-development` or `executing-plans` (chosen at build start) | code + tests; `.mulix/.runtime/<change>/sdd/tasks/` |
| verify  | `verification-before-completion`: full suite, spec check, rulings review | `docs/changes/<change>/report.md` |
| archive | `finishing-a-development-branch`: merge / PR / keep | — |

A change's files live in three places: `docs/specs/<change>/` holds the
requirements doc, meant to stay a useful reference long after the
change is archived; `docs/changes/<change>/` holds the plan (`tasks.md`)
and the verification report; `.mulix/.runtime/<change>/` holds
everything produced while the change runs — its state (`state.yaml`),
the approved design doc, visual-companion sessions (git-ignored: they
carry a session key), and the build's execution workspace (ledger, task
briefs, task reports with RED/GREEN evidence, test logs, review
packages). `.mulix/.runtime/_shared/` catches output from the embedded
skills when no change is active.

New changes use a local-calendar-date ID, for example
`2026-10-14-add-login`. The spec, change, and runtime directories share
that ID; same-day/same-title collisions get `-2`, `-3`, etc. Existing
`001-*` changes are not renamed and still work with `mulix state select`.
The artifact filenames remain `spec.md`, `tasks.md`, and `report.md`.

Two mechanisms provide workflow control from that state:

1. **Guards** (`internal/guard`) check concrete evidence before a
   transition applies — an artifact exists and is non-empty, the design
   approval and execution method are recorded, tasks.md has `## Task N`
   sections numbered 1..n, the execution ledger has a `Task N: complete`
   line for every task, and every task's report carries RED and GREEN
   evidence.
2. **The PreToolUse hook** (`mulix hook`) checks Claude Code's
   `Write`/`Edit` calls against the phase whitelist, including runtime
   subdirectories (design: `specs/`, `brainstorm/`; build: `sdd/`). Raw
   `state.yaml` edits are denied through these tools, not through every
   possible writer.

### Enforcement boundary — not a security sandbox

The hook matches **only `Write|Edit`**. Bash, MCP, NotebookEdit, and other
filesystem-writing tools are not automatically intercepted. Missing tool-input
`file_path` fields are allowed; request parsing, root discovery, active/state loading,
and response-generation failures **fail open**. No active change also
allows the call. DeepSeek Harness has no equivalent hook: agents follow
phase discipline, while CLI transitions run guards.

Guards check the **structure of recorded evidence**, not whether a human
really approved something or a test actually ran. `flow.Table.GuardRefs`
is the single source of readiness checks. `state transition --force`
skips readiness only, never an illegal phase/event check. Neither hook nor
guards authenticate a local writer or make the filesystem unbypassable.

## Embedded superpowers

All fifteen [superpowers](https://github.com/obra/superpowers) skills
are vendored into `assets/superpowers/` (MIT; license and version
installed to `.mulix/superpowers/`) and installed by `mulix init` as
plain project skills in the host's skills directory (`.claude/skills/`
on Claude Code, `.dsh/skills/` on DeepSeek Harness) — whole directories,
with their prompt templates, references, and scripts. mulix doesn't
detect or require the superpowers plugin; if it's also enabled, disable
it for mulix projects so its bootstrap and namespaced copies don't
compete with the phase gates.

`scripts/sync-superpowers.sh <superpowers-dir>` regenerates the vendored
copy. It rewrites `superpowers:<skill>` references to the unprefixed
names, moves every artifact location under `.mulix/.runtime/` (the
scripts resolve the change from `.mulix/active`), and applies a small set
of exact patches — brainstorming's terminal states give way to mulix's
design gate, writing-plans writes `docs/changes/<change>/tasks.md` with
`## Task N` headings and no execution handoff, the executors keep their
workspace as the change's record. Each patch must match exactly once, so
upstream drift fails the sync instead of shipping a half-rewritten copy.

## Install into a project

```
mulix init
```

This writes `.mulix/` (shared constitution, bundled templates, the
superpowers license/version, `.mulix/.runtime/.gitignore`, and the
`active`-change marker once a change exists), the skills for the
selected agent hosts, and — for Claude Code — merges a `PreToolUse`
hook entry into `.claude/settings.json` without disturbing any
hooks/settings you already have. It does not create `docs/specs/` or
`docs/changes/` — those come from `mulix new`, once there's an actual
change to hold. `--dir` targets a project directory other than the
current one; `--force` backs up differing existing resources before
replacing them. Existing customized files skipped by normal init are not
silently registered as official vendor content. Init refuses unresolved
pending updates, including with `--force`: resolve them through update
first.

### Safe resource updates

`mulix update` installs resources embedded in the **current binary**; it
does not upgrade the CLI executable. `.mulix/install-state.json` records
installed hosts, per-file official SHA-256 digests, and pending updates.
For each managed file, B is the last accepted official content, L is the
local file, and N is the current bundled content. A clean three-way merge
stores **L = merged result, B = pure N**, so repeated updates keep local
customizations instead of adopting them as a vendor baseline.

Older installations are proved **per file**: a digest matching B, exact
B=N bytes, or a frozen known official entry fingerprint. Unknown or
corrupt baselines do not become trusted just because L=B. L=N can safely
rebuild the baseline, after protecting a different old B. Digests are local
consistency checks, not signatures or authentication against a malicious
local writer. Corrupt installation metadata is an error even with force;
repair it deliberately, not by treating it as an absent record.

Conflicts leave L and B untouched. Candidates live in
`.mulix/.update-conflicts/<content-id>/merged.md` and `incoming.md`, with a
pending record; retries preserve them. To accept one:

1. Read the reported candidate and incoming file, resolve manually, and
   put the chosen result in the reported **managed target** (not just the
   candidate). Remove this tool's conflict marker lines.
2. Run `mulix update --accept .mulix/templates/spec-template.md` (substitute
   the reported project-relative path; repeat `--accept` for more files).
3. Acceptance keeps your resolved L and advances B to pure N. It requires
   a pending update for the same incoming version; an unknown legacy file
   without pending cannot be accepted this way. If the binary changed,
   finish with the matching version or explicitly choose force.

The canonical entry is **`mulix-using`**. Only exact old entry files
`using-mulix/SKILL.md` and `mulix-using-mulix/SKILL.md` are migration
sources. Known pristine sources or a single customization with a trusted
ancestor can migrate. Unknown sources, competing customizations, and
custom target collisions stay blocked; auxiliary files are never
recursively deleted. A rename resolution must use frontmatter
`name: mulix-using`. Accepted cleanup checkpoints allow interrupted
cleanup to resume only while source/target/baseline/backup hashes agree.

`mulix update --force` is an explicit discard decision: it backs up first,
then installs current N and removes obsolete managed resources or exact
old entry sources. It cannot be combined with `--accept`. Normal obsolete
cleanup requires a trusted B and an untouched L. Templates/overrides,
constitution, runtime data, backups, and conflict candidates are outside
the updater's obsolete-file ownership.

Destructive changes preserve old L, old B, and original metadata in a
unique `.mulix/.backups/update-*/` directory (`local/`, `baseline/`,
`install-state.json` if it existed). The first snapshot is not overwritten;
backup failure stops replacement/deletion. Paths are printed even when a
later operation fails. Backups and conflict files have local ignore rules;
no-op updates create no backup. LF/CRLF versions of the required final ignore
rules are accepted without rewriting existing bytes. Inspect these snapshots for manual recovery
and retain them until satisfied; there is no automatic whole-run rollback.
Conflicted/blocked updates and I/O failures return nonzero and report
completed work rather than claim overall success. A `partial` outcome names
mutations already completed within the failing file and the stage that failed;
it is not a successful whole-file commit. If forced retirement of an obsolete
pending file deleted L/B before metadata saving failed, repeat explicit force
to finish retirement; normal retry stays blocked and candidates are retained.
Windows case-only path aliases share managed/shipped identity, and excluded
overrides remain excluded regardless of directory casing.

State, active, and resource writes prepare a same-directory unique file,
write/sync/close it, then replace the target without first truncating the
old file. **No multi-session lock, concurrent-writer isolation, or
cross-file transaction is provided**. In particular, Go does not guarantee
atomic rename on non-Unix platforms: this is not an absolute all-Windows-
filesystems atomicity promise.

### Agent hosts

Skills are installed per host, selected with a repeatable
comma-separated flag (default `claude`):

```
mulix init                     # Claude Code only (historical default)
mulix init --host dsh          # DeepSeek Harness only
mulix init --host claude,dsh   # both
```

| Host | Skills directory | Enforcement | Bootstrap |
|------|------------------|-------------|-----------|
| `claude` (Claude Code) | `.claude/skills/` | PreToolUse hook + guards | — |
| `dsh` (DeepSeek Harness) | `.dsh/skills/` | guards only (no hook equivalent) | mulix section merged into `AGENTS.md` |

Both hosts receive the same skill set: the `mulix-*` phase skills,
the `mulix-using` bootstrap skill, and every embedded superpowers
skill. mulix's own skills are written per host — wherever they state
the project skills directory they name the installing host's
directory. The `.dsh/skills/` root is DeepSeek Harness's
highest-priority local discovery root, and the harness loads the
project's `AGENTS.md` every session; the merged mulix section states
the phase discipline that the hook enforces on Claude Code, because on
this host nothing technical blocks an out-of-phase write — the guard
checks inside `mulix state transition` are the enforcement.

`mulix update` keeps every host tree the project installed (it derives
them from installation metadata and baseline skill footprints, falling
back only to exact current/legacy entry footprints), so neither a
claude-only nor a dsh-only project grows the other host's tree. Without
host evidence it asks for `mulix init --host ...`, not default Claude.
A dsh project's `AGENTS.md` section is refreshed only when its canonical
entry is ready; blocked migration does not point it at a missing entry.
To add a host to an existing project, re-run `mulix init --host dsh`
(idempotent — it fills in only what's missing).

## Everyday commands

```
mulix new "<title>"              # create a change, starts in the specify phase
mulix state show [--change ID]   # see the current phase and what can happen next
mulix state set <field> <value>  # record an artifact path or flag (does not change phase)
mulix state transition <event>   # run guards, then advance the phase if they pass
mulix state select <change>      # set the active change for commands that omit --change
mulix guard <event>              # run guards read-only, without transitioning
mulix status                     # list every change and its phase
```

`--change ID` is accepted by `state show`, `state set`, `state
transition`, and `guard`; each defaults to the active change, which
`state select` sets. `mulix new --branch <name>` records a git branch
name in the change's state — mulix does not create the branch itself.

`mulix state transition --force` applies a transition even when guard
checks fail, but only for an event legal from the current phase. It is a
readiness recovery override, not permission to skip the phase table; do
not use it to conceal missing approval or verification.

Settable fields: `spec_path`, `tasks_path`, `report_path`,
`clarify_skipped`, `design_track` (`bounded|architectural`),
`design_path`, `design_approved`, `execution_method`
(`subagent-driven|inline`), `verify_result`, `archive_confirmation`.

`mulix hook` is the `PreToolUse` entry point Claude Code invokes; you
won't normally run it by hand.

## Templates

`assets/templates/*.md` are installed to `.mulix/templates/` by `mulix
init`. `spec-template.md` and `constitution-template.md` mirror
spec-kit's (`FR-###`/`SC-###` numbering, User Scenarios, `##
Clarifications`, principle placeholders). `tasks-template.md` is the
layout writing-plans produces: plan header with Spec and Design links,
Global Constraints, Review Focus, and one `## Task N: <name>` section
per task with RED → GREEN → commit steps. Steps start as
`- [ ] **Step N: ...**`; mark only steps actually executed and verified
`- [X] **Step N: ...**` (capital X). This status-only edit to an approved
plan is allowed, but task definitions, commands, and other approved text
stay unchanged. An unperformed or unauthorized commit remains `[ ]` even
when GREEN passes. Checkboxes do not replace ledger or RED/GREEN evidence.

Guards depend on exactly two structural rules in those files: a literal
`## Clarifications` heading in spec.md, and `## Task N` headings in
tasks.md numbered 1..n (headings inside ``` fences don't count). Those
headings are also what the build executors extract task briefs by and
what the ledger's `Task N: complete` lines refer to.

The phase skills (`assets/skills/mulix-*`) are thin: `mulix-specify` and
`mulix-clarify` port spec-kit's specify/clarify commands; `mulix-design`,
`mulix-tasks`, `mulix-build`, `mulix-verify`, and `mulix-archive` each
invoke an embedded superpowers skill and add only what mulix needs
around it — where output goes, which paths apply, and how the phase's
gate replaces the skill's own hand-off.

## Presets

A preset overrides one or more of mulix's bundled templates
(`spec-template`, `tasks-template`, etc). The override stack, checked in
order, is:

```
.mulix/templates/overrides/<name>.md   (project override, always wins)
installed presets, by priority          (lower number = higher precedence)
embedded bundled template             (core used by preset resolve)
```

Currently `preset resolve` uses embedded core content, not local edits to
`.mulix/templates/<name>.md`. Aligning those core sources is deferred to a
separate change; this update work does not fix that behavior.

A preset is a directory with a `preset.yml` manifest plus the template
files it declares (see `internal/preset/manifest.go` for the exact
schema). Each declared template can compose with the layer below it via
`strategy: replace|prepend|append|wrap` (`wrap` substitutes a
`{CORE_TEMPLATE}` placeholder), the same four strategies spec-kit's own
preset system uses.

```
mulix preset add <dir-or-https-zip-url-or-catalog-id>
mulix preset list [--all]
mulix preset info <id>
mulix preset resolve <template-name>      # print what mulix would actually use
mulix preset set-priority <id> <n>
mulix preset enable|disable <id>
mulix preset remove <id> [--purge]
mulix preset search <query>               # searches the bundled + configured catalogs
mulix preset catalog list|add|remove
```

`preset add` accepts a local directory, an `https://` URL to a zip archive
(GitHub's "Source code (zip)" layout is handled automatically), or a
preset id already listed in the bundled or a configured catalog. mulix
ships with no bundled preset content yet (unlike spec-kit's lean/
constitution-sync), so the bundled catalog is currently metadata-only.

## Converting tasks to GitHub issues

The `mulix-taskstoissues` skill (installed by `mulix init` like any other
skill) turns a change's `tasks.md` into one GitHub issue per task, via the
`gh` CLI. Unlike spec-kit's version (which calls a GitHub MCP server's
tools), this needs nothing beyond `gh auth login` already having been
run — no MCP server configuration required. It is phase-independent: it
works any time `tasks.md` exists and is not gated by `internal/flow`.

## Development

```
go build ./...
go vet ./...
go test ./...
bash scripts/smoke-e2e.sh    # init + one change through all 7 phases in a
                              # throwaway git repo (needs git + bash)
bash scripts/smoke-update.sh # update/rename/conflict/accept (also needs Python 3)
./scripts/sync-superpowers.sh <superpowers-dir>   # re-vendor superpowers
```

Module: `github.com/z-fenix/mulix-coding`, Go 1.27.

## Build, install, release

Shell scripts under `scripts/` — no CI or external tooling required, only
`git` and a Go toolchain.

```
./scripts/build.sh            # build bin/mulix natively, version stamped
                              # from MULIX_VERSION, git describe, or "dev"
./scripts/build.sh test       # build + go vet + go test
./scripts/install.sh          # clone the repo, compile locally, install to
                              # ~/.local/bin (--version, --prefix, --source)
./scripts/release.sh          # cross-compile linux/darwin/windows x
                              # x86_64/arm64 into dist/ + sha256 checksums
./scripts/release.sh --dry-run
```

