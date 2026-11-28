# mulix-coding

A Go CLI that brings spec-driven development, an enforced phase state
machine, and the full superpowers skill set together for **Claude Code**
and **DeepSeek Harness**. It borrows from four existing projects:

- **spec-kit** — the constitution → specify → clarify front half of the
  workflow, its spec template and preset system.
- **comet** — the phase state machine, guard-check gating, and (the core
  enforcement mechanism) a Claude Code `PreToolUse` hook that blocks
  `Write`/`Edit` calls that don't match the current phase.
- **superpowers** — embedded whole: every skill ships inside mulix and
  runs the design → tasks → build → verify → archive half of the
  workflow (brainstorming, writing-plans, test-driven-development,
  subagent-driven-development / executing-plans,
  verification-before-completion, finishing-a-development-branch), with
  retained artifacts under `.mulix/.runtime/` and transient output in
  context-scoped system scratch.
- **caveman** — the discipline of writing `SKILL.md` `description` fields
  as trigger conditions only (never a process summary, so the model can't
  use the summary as an excuse to skip loading the real content).

**Phase 1 scope** was Claude Code only: no other agent host, no caveman
compression engine/BM25/browse infrastructure, no comet Native
workflow/dashboard, no spec-kit self-update. DeepSeek Harness arrived
later as a second agent host (Phase 14) — see [Install into a
project](#install-into-a-project). The rest of that list is still out of
scope; [`docs/PLAN.md`](docs/PLAN.md) has the full phase breakdown.

For GitHub or GitLab delivery changes, use the
[bound Issue workflow](#publish-and-synchronize-bound-issues). Markdown
changes do not need a remote.

## The workflow

Start with [`mulix-using`](assets/skills/mulix-using/SKILL.md). A throwaway
exploration stays out of the delivery workflow. When you decide to ship it,
register a delivery change: reuse prototype source only after a behavioral
RED on the delivery baseline. A prototype's passing tests and approvals do
not carry over.

```
specify → clarify → design → tasks → build → verify → archive
```

| Phase   | What happens | Artifact |
|---------|--------------|----------|
| specify | WHAT/WHY, no design | `spec_path`; publish only for a remote tracker |
| clarify | up to 5 targeted questions | spec `## Clarifications`; remote body then confirmed sync |
| design  | `brainstorming`: classify, consult applicable ADRs, design, explicit approval | `specs/*-design.md` under runtime (architectural) |
| tasks   | `writing-plans`: executable tasks and acceptance mapping | per-task Markdown in `tasks_dir`, or legacy `tasks_path` |
| build   | `test-driven-development` with the chosen executor | code, tests and `sdd/<plan>/` evidence |
| verify  | `verification-before-completion`: full suite, spec check, rulings review | `report_path` and current verification receipt |
| archive | `finishing-a-development-branch`: merge / PR / keep | local archive; optional spec Issue close afterward |

Run `mulix state show` for the current change's layout, tracker and paths.
After tracker setup, `mulix new` pins `runtime-issues-v1` and that tracker:
`.mulix/.runtime/<change>/spec.md`, `issues/NN-short-name.md` (one file per
numbered task), and `report.md`. Markdown is local-only. GitHub/GitLab use
these paths for drafts, then retain snapshots of the bound authoritative Issue
bodies. Changing the project default affects future changes only. Without
tracker setup and for existing legacy changes, the spec and single `tasks.md`
plan and report remain in `docs/specs/<change>/` and `docs/changes/<change>/`.
Legacy and native changes can coexist without a migration.

The runtime directory also holds `state.yaml`, design docs in `specs/`,
authored Karate sources in `tests/` and execution records in `sdd/<plan>/`
(`sdd/issues/` for native). The latter retains the plan marker, progress ledger,
briefs/reports, task logs and review packages. New changes start with an empty
`tests/`; it holds only authored Karate `.feature` entries and helpers.
Visual companions, diagnostics, ad-hoc RED/rerun logs, verification tooling,
migration manifests and build binaries go in private context-scoped system
scratch. Put necessary command/output summaries in the existing ledger or report.
Historical runtime directories and older session-key ignore rules are preserved.

Projects that opt into Karate use the `karate-jsonl` suite described in the
[quality policy template](assets/templates/quality-template.md). Task runs
select only their owned feature entries; `mulix verify` runs every task and
final-only entry plus the other mandatory suites. `mulix test receipt` checks
an existing task result without launching another test run. Supply the pinned
RC1 JAR separately; `init` and `update` do not install Java or a JAR.

New changes use a local-calendar-date ID, for example
`2026-10-14-add-login`. Collisions get `-2`, `-3`, etc. Existing `001-*`
changes keep their IDs and remain selectable with `mulix state select`.

Two mechanisms provide workflow control from that state:

1. **Guards** check the defined artifacts, local approval flags, the owning
   ledger and actual execution receipts. A separate remote-sync check runs
   first on both read-only `mulix guard` and the locked `state transition`;
   recovery `--force` cannot override it. Final verification runs every
   mandatory suite and binds the reviewed report.
2. **The PreToolUse hook** (`mulix hook`) checks Claude Code's
   `Write`/`Edit` against the phase whitelist. It protects exact published
   snapshots even where a phase permits drafts. The check does not apply to
   other writers; use the confirmed `tracker apply` path, not another writer.
   Transient output is scoped to the resolved scratch directory:
   `brainstorm/` in design, `verification/` or `migrations/` in build/verify,
   and `diagnosing-superpowers/` for diagnostics. `state.yaml` is CLI-owned.

### Enforcement boundary — not a security sandbox

The hook matches **only `Write|Edit`**. Bash, MCP, NotebookEdit, and other
filesystem-writing tools are not automatically intercepted. Missing tool-input
`file_path` fields are allowed. **Legacy mode** retains fail-open behavior for
request parsing, root discovery, active/state loading, and output errors.
**Session mode** blocks invalid bindings, configuration, active/state loading,
and lock errors when the project is identifiable; missing bindings alone
fall back to global active. An unidentifiable project root still warns and
allows the call. No current change allows ordinary writes outside runtime, but retained runtime requires an
owning change and session-mode control metadata stays CLI-only. DeepSeek Harness has no equivalent hook:
agents follow phase discipline, while CLI transitions run guards.

`flow.Table.GuardRefs` is the single source of readiness checks. Guards consume
CLI-generated execution receipts rather than handwritten pass claims. They do
not authenticate reviewer consent or prove assertions match product intent.
Additional project gates come from the project's constitution. Projects using
the former bundled checker must follow the
[optional compatibility migration](compat/formal-workbench/README.md) before updating.
`state transition --force --reason "recovery explanation"` records readiness
bypasses, never makes an illegal phase/event legal and never creates test
qualification. Neither hook nor local receipts authenticate a writer controlling
the filesystem; this is not cryptographic attestation.

## Multiple sessions in one checkout

Existing and newly initialized projects default to **legacy** mode. To opt in:

```bash
mulix context mode session
```

Use the real identity supplied by the host, not a made-up ID. When SessionStart
provides `MULIX_SESSION_ID`, commands inherit it; otherwise pass `--session-id`
explicitly. For example, if the host supplied `session-A`:

```bash
mulix state select 2026-10-28-add-login --session-id session-A
```

`new` and `state select` then affect only that session. `--global` on either
command updates `.mulix/active` without changing session bindings. Without a
session identity, commands keep using global active. Query the effective
selection and its `session-binding`, `global-fallback`, or `global` source:

```bash
mulix context show --json
```

An unbound session dynamically follows global active at each new invocation;
merely reading the context never binds it. A damaged/missing-target/archived
binding is an error, not a reason to use a different global change. Repair it
by selecting a valid change for that session, or explicitly return to fallback:

```bash
mulix context unbind --session-id session-A
```

Archiving keeps session bindings and only clears global active if it still
points to the archived change. An already-running script keeps its startup
context. SDD scripts query `mulix context show --field runtime-dir`; companion
scripts query `mulix context show --field scratch-dir`. Failed resolution stops
creation rather than falling back. Both queries use the current identity.

`context show --json` includes an absolute `scratch_dir`, derived from the
project, change and session without creating files. It remains available for
a valid no-change context, whose `runtime_dir` is empty. Querying `runtime-dir`
alone without a current change fails; create/select a change before running a
plan. Scratch directories are private, temporary and not archived; do not use
one context's scratch as another context's output path.

Context files, hashed-ID binding files, and persistent lock files live under
`.mulix/context.json`, `.mulix/sessions/`, and `.mulix/.locks/`, ignored by local
rules installed by init. Session-mode writes require unambiguous Windows path
components (no trailing-dot/space aliases), and relative targets use the same
physical-path policy as absolute targets. State updates use bounded native process locks. This
isolates context, **not source-file edits, Git branches, ports or databases**;
multiple sessions editing the same source still need coordination.

After upgrading the CLI/resources, invoke the `setup-mulix` skill and explicitly select the Claude Code host to install the new startup hook.

Setup merges SessionStart plus Write/Edit PreToolUse hooks without replacing
unrelated hook fields; ordinary `mulix update` never edits host settings.
An already-open session needs restart/resume or explicit identity parameters.
A real Claude CLI 2.1.312 probe verified SessionStart and PreToolUse for
concurrent bound sessions; automatic `CLAUDE_ENV_FILE` injection was not
available in that mode, so explicit `--session-id` remains the fallback.
Desktop, resumed sessions, subagents and Unix were not exercised.
See [integration evidence and limits](docs/verification/session-hook-contract.md).
DSH-only init installs no Claude hooks; its agents can use explicit identities.

## Embedded superpowers

All fifteen [superpowers](https://github.com/obra/superpowers) skills
are vendored into `assets/superpowers/` (MIT; license and version
installed to `.mulix/superpowers/`) and installed by `setup-mulix` as
plain project skills in the host's skills directory (`.claude/skills/`
on Claude Code, `.dsh/skills/` on DeepSeek Harness) — whole directories,
with their prompt templates, references, and scripts. mulix doesn't
detect or require the superpowers plugin; if it's also enabled, disable
it for mulix projects so its bootstrap and namespaced copies don't
compete with the phase gates.

`scripts/sync-superpowers.sh <superpowers-dir>` regenerates the vendored
copy. The current source pin is `v7.0.2` at
`22f0b463e7a5e579904dd6211a520321035526f7`; use that clean checkout for
release verification. The script stages the generated files and checks all
patches before replacing the embedded tree. It rewrites
`superpowers:<skill>` references to the unprefixed
names, moves retained design/SDD output under `.mulix/.runtime/`, directs
transient companion/diagnosis material to system scratch (the scripts resolve
paths through `mulix context show`), and applies exact patches for the phase
handoffs and execution ledger. The `mulix-tasks` phase skill chooses the
native per-task template or the legacy combined plan. The executors keep their
workspace as the change's record. Each patch must match exactly once, so
upstream drift fails the sync instead of shipping a half-rewritten copy.
The mulix-owned runtime ledger tools and templates are maintained in
`scripts/superpowers-overrides/` and copied into the embedded skills by
that sync; edit the overlay and refresh its matching files under
`assets/superpowers/skills/` together. New progress files use
`# SDD progress — plan: ...` as their first line, followed by execution
context, pre-flight, per-task progress, and final-review sections, with
separate completion markers and linked evidence. Existing files stay
unchanged: legacy `# SDD ledger` identities and plain completion lines
remain valid.

## Install into a project

Obtain [`assets/skills/setup-mulix/`](assets/skills/setup-mulix/) from the
source release matching your installed binary. This setup skill is not
installed by the binary. Copy it to your host's skill discovery directory
(for example, `.claude/skills/setup-mulix/` or `.dsh/skills/setup-mulix/`), then
invoke `setup-mulix` in the destination project. The skill asks you to choose
hosts and a Markdown, GitHub, or GitLab tracker, presents the selection for
confirmation, and runs the bundled setup bridge. Remote targets require an
explicit hostname and project; an unreachable target is saved as pending only
with your explicit approval. Authentication stays with `gh` or `glab`, and
setup does not authorize remote writes.

This writes `.mulix/` (shared constitution, bundled templates, the
superpowers license/version, `.mulix/.runtime/.gitignore`, and the
`active`-change marker once a change exists), the skills for the
selected agent hosts, and — for Claude Code — merges `PreToolUse` and
`SessionStart` hook entries into `.claude/settings.json` without disturbing any
hooks/settings you already have. New changes in a configured project use
runtime paths, not new `docs/specs/` or `docs/changes/` directories. The
destination is selected through the skill; an explicit force choice backs up
differing managed resources before replacing them for the selected hosts.
The layout applies to the whole project, so setup also checks installed hosts
that were not selected. Reconcile stale workflow resources with `mulix update`
before enabling the new layout.
Existing customized files skipped by normal setup are not silently registered
as official vendor content. Setup refuses unresolved pending updates, even
with force: resolve them through update first.

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
the updater's obsolete-file ownership. The retired Issue exporter is a
narrow exception to force: only its exact installed `SKILL.md` is removed,
after backing up a verified unmodified vendor copy. Customized, unknown or
pending copies are reported and retained even with force; resolve those
manually. Update does not change historical docs or Issue records.

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
pending file (other than the retired exporter above) deleted L/B before
metadata saving failed, repeat explicit force to finish retirement; normal
retry stays blocked and candidates are retained.
Windows case-only path aliases share managed/shipped identity, and excluded
overrides remain excluded regardless of directory casing.

State, active, and resource writes prepare a same-directory unique file,
write/sync/close it, then replace the target without first truncating the
old file. **No multi-session lock, concurrent-writer isolation, or
cross-file transaction is provided**. In particular, Go does not guarantee
atomic rename on non-Unix platforms: this is not an absolute all-Windows-
filesystems atomicity promise.

### Agent hosts

Skills are installed per host. Select Claude Code, DeepSeek Harness, or both when invoking `setup-mulix`.

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
host evidence it asks for explicit selection through `setup-mulix`, not default Claude.
A dsh project's `AGENTS.md` section is refreshed only when its canonical
entry is ready; blocked migration does not point it at a missing entry.
To add a host to an existing project, re-run `setup-mulix` and select that host (idempotent — it fills in only what's missing).

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

`mulix state transition --force --reason "recovery explanation"` can bypass
ordinary readiness for a legal transition, recording failed checks, the
reason and time. It cannot bypass tracker sync; a forced archive can be
archived but unqualified. `state show` reports the override count. Receipts
and override records are not settable through `state set`.

Settable fields: `clarify_skipped`, `design_track` (`bounded|architectural`),
`design_path`, `design_approved`, `execution_method` (`subagent-driven|inline`),
`verify_result`, and `archive_confirmation`. Legacy can also set `spec_path`,
`tasks_path` and `report_path`; native paths are pinned.

`mulix hook` is the `PreToolUse` entry point Claude Code invokes; you
won't normally run it by hand.

## Verifiable acceptance (P0)

Before `tasks-complete`, configure project-owned `.mulix/quality.yaml` using the
installed `quality-template.md` ([source reference](assets/templates/quality-template.md)).
Each suite declares an argv command, project-relative cwd, timeout and `go-json`
or generic JSON-lines adapter. Every configured suite is mandatory at final
verification; declare nested modules and integration suites explicitly.

- spec.md: one `mulix-acceptance` YAML fence, AC → FR + observable expectation.
- Native task documents: one `mulix-tests` YAML fence per numbered task.
  For legacy, use one fence in the combined `tasks.md`. Map each TC to an
  AC/task, point, Given/When/Then, suite, emitted test identity and source file.
  Cover every AC and task across the plan.
- report.md: one `mulix-review` YAML fence, reviewer reference, `result: approved`
  and `blocking: []`. Semantic test quality and reviewer independence need review.

```sh
# Build phase, after writing the behavior test:
mulix test run --task 1 --stage red
# Implement without changing the mapped test or approved contract, then:
mulix test run --task 1 --stage green
# Record each verified task in the owning ledger with its current receipt.
# A reasoned documentation/verification task uses --stage check.
mulix state transition build-complete
# Obtain and record whole-change review in the final report at report_path,
mulix verify
mulix state transition verify-pass
# Get the human's finishing decision before archive confirmation.
```

A successful RED command means the runner observed a real mapped test failure;
it retains the subprocess's nonzero exit. Infrastructure failures, empty suites,
all-skipped/missing mapped tests, malformed streams and false PASS text fail
closed. Incomplete/failed retries supersede old successes. Corrective
`verify-fail` and new `build-complete` cycles invalidate earlier final verification
without erasing task provenance. Receipts in existing
state.yaml bind physical project, change, spec/plan/policy, test files and source;
full streams live in private system scratch, while existing reports retain
summaries and state retains per-test counts/identities and stream digests.

Final verification also binds the report and Git revision. Source, policy,
contract or report changes stale normal verify/archive acceptance; only the
legacy parser normalizes verified checkbox-only progress. `mulix verify` can refresh evidence in not-yet-archived
archive phase after authorized finishing changes. If it fails there,
`mulix state transition verify-fail` returns to build; already archived changes
cannot reopen through that event. Directly setting `verify_result=pass` creates
no evidence. Existing v2 state remains readable, but continuing old work requires
the new contracts/receipts; historical archived examples are not rewritten.

This is not the product-quality ceiling: only declared suites and mapped cases
are checked. Ignored inputs/external dependencies, assertion adequacy, CI and
native-platform/real-host qualification still need their own controls (P1/P2).

## Templates

Setup installs `assets/templates/*.md` in `.mulix/templates/`.
`spec-template.md` and `constitution-template.md` mirror spec-kit's format.
For native plans, use `issue-task-template.md` for each `NN-short-name.md`.
Number from 1 without gaps, matching the one `## Task N:` heading and
its matrix. Record steps and `- Task N: complete (receipt <id>)` in the
owning ledger; don't change task Markdown to record progress.

For legacy only, use `tasks-template.md`: one combined file with plan
header, Spec and Design links, Global Constraints, Review Focus and ordered
`## Task N:` sections. Mark only verified steps `[X]`; leave skipped steps
unchecked, including any unauthorized commit. Checkboxes and the legacy
plain completion line are progress, not runner evidence.

The `clarify-complete` guard checks for a `## Clarifications` heading.
Task headings outside code fences and the acceptance mappings drive review,
brief extraction and verification.

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

## Publish and synchronize bound Issues

Only native GitHub/GitLab changes use these commands. They use the
change's pinned host, project and Issue identities through the caller's
`gh` or `glab` authentication, not the ambient Git remote or default CLI
project. Markdown and legacy need no tracker options or network access.

1. **Specify:** write the local spec draft. Run `mulix tracker publish spec
   --dry-run`, review the exact target and body, then after explicit approval
   run `mulix tracker publish spec --expect TOKEN`.
2. **Clarify:** for a bound spec, make any authorized edit on its authoritative
   Issue, preserving the binding marker. Review `mulix tracker preview spec`
   and get confirmation before `mulix tracker apply spec --expect TOKEN`.
   Apply writes only the local snapshot. Use the same `preview`/`apply` pattern
   for `task-N` as appropriate. If local changes conflict, review a new preview
   with `--backup-local` and pass that flag and token to apply. An old token
   cannot approve new content or another Issue.
3. **Tasks:** write one draft per task using the per-task template. Run
   `mulix tracker publish tasks --dry-run`, then after exact authorization
   `mulix tracker publish tasks --expect TOKEN`. Pending publication uses a
   fresh dry run for reconciliation; it never blindly retries creation.

Publication is not automatic. Specify, clarify and design exits recheck the
verified spec; from `tasks-complete` through `archived`, the forward gates
also check the exact set of bound task documents. Missing, inaccessible,
replaced or drifted bindings block the transition before any force override.
`verify-fail` returns to Build locally without checking the remote.

### Optional Issue close

- **Task:** in active Build, after the real current GREEN/CHECK receipt and
  its matching native ledger entry, run `mulix tracker close task-N --dry-run`.
  With separate explicit authorization, run `mulix tracker close task-N
  --expect TOKEN`.
- **Spec:** after `archived` succeeds, run `mulix tracker close spec --change
  ID --dry-run`. With separate explicit authorization, run `mulix tracker
  close spec --change ID --expect TOKEN`. Use the explicit archived ID after
  global active is cleared; a session bound to a different change may not
  mutate it. Failure to close leaves the local archive intact; review a fresh
  dry run before retrying. GitLab hosts must support WorkItem closure; there
  is no fallback mutation through a reusable IID.

Closing an Issue is optional and does not run tests, authenticate review,
prove target-branch integration, or replace archival confirmation. Tokens
bind exact operations and evidence; they are not human identity or consent.

## Development

```bash
go build ./...
go vet ./...
go test -count=1 ./...
go -C examples/todo-cli test -count=1 ./...
bash scripts/smoke-e2e.sh       # seven phases in a throwaway Git repository
bash scripts/smoke-sessions.sh  # multi-process sessions and stale-state gates
bash scripts/smoke-update.sh    # update/rename/conflict/accept; needs Python 3
bash scripts/sync-superpowers.sh <superpowers-dir>   # re-vendor superpowers
```

Module: `github.com/z-fenix/mulix-coding`, Go 1.27.

For the mandatory full/vet/race/Python/coverage/final-artifact suite, use
[`scripts/quality.py` as documented in docs/quality.md](docs/quality.md).
It records real exits and raw logs, tests installation/upgrades in private
prefixes, and rejects unapproved skips or stale source/artifact evidence.
The pinned-tool six-platform native workflow ends in **P1 required**;
writing the workflow does not activate remote branch protection.

## Build, install, release

The native developer build needs Bash, Git and Go. The release/installer
wrappers additionally need Python 3.10+ (standard library only); installing
an existing verified bundle needs neither Git nor Go.

```bash
bash scripts/build.sh          # bin/mulix; MULIX_VERSION, git describe, or "dev"
bash scripts/build.sh test     # developer build + go vet + go test
bash scripts/release.sh        # six targets in NEW dist/mulix_<version>/
bash scripts/release.sh --dry-run  # plan only: no build or acceptance evidence
bash scripts/install.sh --bundle /path/to/bundle --prefix /chosen/bin
bash scripts/install.sh --source /path/to/checkout --prefix /chosen/bin
```

Release output includes a source/version/architecture-bound manifest and
checksums. Existing output directories are refused, never cleared. Both
installation modes stage and version-check before atomic replacement;
failures preserve the previous executable. Explicit installation without
`--source`/`--bundle` retains the remote source-clone default to `~/.local/bin`.
See [quality and delivery boundaries](docs/quality.md) before publishing:
use the same bundle that passed the native matrix, not a fresh rebuild.

