# mulix-coding

A Go CLI that brings spec-driven development, an enforced phase state
machine, and the full superpowers skill set together for **Claude
Code**. It borrows from four existing projects:

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

**Phase 1 scope**: Claude Code only. No other agent host, no caveman
compression engine/BM25/browse infrastructure, no comet Native
workflow/dashboard, no spec-kit self-update. See [`docs/PLAN.md`](docs/PLAN.md)
for the full phase breakdown and what's explicitly out of scope.

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
| tasks   | `writing-plans`: the approved design becomes an executable plan, reviewed by a plan-reviewer subagent | `docs/changes/<change>/tasks.md` (`## Task N` sections) |
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

Two independent mechanisms enforce phase gating from that state:

1. **Guards** (`internal/guard`) check concrete evidence before a
   transition applies — an artifact exists and is non-empty, the design
   approval and execution method are recorded, tasks.md has `## Task N`
   sections numbered 1..n, the execution ledger has a `Task N: complete`
   line for every task, and every task's report carries RED and GREEN
   evidence.
2. **The PreToolUse hook** (`mulix hook`) intercepts every `Write`/`Edit`
   call Claude Code makes and blocks any file outside the current phase's
   whitelist — including inside `.mulix/.runtime/`, where `state.yaml` is
   never writable and each phase may write only the subdirectory it
   produces (design: `specs/`, `brainstorm/`; build: `sdd/`).

## Embedded superpowers

All fourteen [superpowers](https://github.com/obra/superpowers) skills
are vendored into `assets/superpowers/` (MIT; license and version
installed to `.mulix/superpowers/`) and installed by `mulix init` as
plain project skills in `.claude/skills/<name>/` — whole directories,
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
`active`-change marker once a change exists), `.claude/skills/` (one
`mulix-*` skill per phase, the `using-mulix` bootstrap skill, and every
embedded superpowers skill), and merges a `PreToolUse` hook entry into
`.claude/settings.json` without disturbing any hooks/settings you
already have. It does not create `docs/specs/` or `docs/changes/` —
those come from `mulix new`, once there's an actual change to hold.
Re-running `mulix init` is safe (it skips files that already exist);
`mulix update` refreshes installed files after an upgrade, three-way
merging local edits and removing files this version no longer ships if
they're untouched.

## Everyday commands

```
mulix new "<title>"              # create a change, starts in the specify phase
mulix state show [--change ID]   # see the current phase and what can happen next
mulix state set <field> <value>  # record an artifact path or flag (does not change phase)
mulix state transition <event>   # run guards, then advance the phase if they pass
mulix guard <event>              # run guards read-only, without transitioning
mulix status                     # list every change and its phase
```

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
per task with RED → GREEN → commit steps.

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
.mulix/templates/<name>.md              (core, bundled)
```

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
./scripts/smoke-e2e.sh        # init + one change through all 7 phases in a
                              # throwaway git repo (needs git + bash)
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

