# mulix-coding

A Go CLI that brings spec-driven development, an enforced phase state
machine, and TDD/brainstorming discipline together for **Claude Code**.
It's a from-scratch Go reimplementation that borrows ideas from four
existing projects rather than wrapping any of them:

- **spec-kit** — the constitution → specify → clarify → plan → tasks →
  analyze → implement workflow shape and template structure.
- **comet** — the phase state machine, guard-check gating, and (the core
  enforcement mechanism) a Claude Code `PreToolUse` hook that blocks
  `Write`/`Edit` calls that don't match the current phase.
- **superpowers** — the brainstorming skill's spike/bounded/architectural
  classification and the test-driven-development red-green-refactor
  discipline, both folded directly into mulix's `mulix-specify` and
  `mulix-build` skills rather than kept as a separate phase.
- **caveman** — the discipline of writing `SKILL.md` `description` fields
  as trigger conditions only (never a process summary, so the model can't
  use the summary as an excuse to skip loading the real content), and the
  principle of delegating exploration to cheap subagents that report back
  citations instead of full file dumps.

**Phase 1 scope**: Claude Code only. No other agent host, no caveman
compression engine/BM25/browse infrastructure, no comet Native
workflow/dashboard, no spec-kit self-update. See [`docs/PLAN.md`](docs/PLAN.md)
for the full phase breakdown and what's explicitly out of scope.

mulix does port spec-kit's **preset** system (a template-override stack)
and **taskstoissues** (GitHub issue conversion) — see below.

## The workflow

```
specify → clarify → plan → tasks → analyze → build → verify → archive
```

There is no separate brainstorm phase: `mulix-specify` opens by
classifying the work (spike/bounded/architectural) and getting explicit
approval before spec.md gets written, and `mulix-build` repeats that
classification per task before writing its first test. Brainstorming is a
discipline embedded at those two points, not a phase with its own state
node.

A change's artifacts split across two directories: `docs/specs/<change>/`
holds the requirements doc (`spec.md`), meant to stay a useful reference
long after the change is archived; `docs/changes/<change>/` holds the process
artifacts produced while executing that spec (`plan.md`, `tasks.md`,
`analyze.md`, `report.md`) plus the change's own runtime state at
`docs/changes/<change>/.runtime/state.yaml`, not in conversation context. Two
independent mechanisms enforce phase gating from that state:

1. **Guards** (`internal/guard`) check concrete evidence — an artifact
   exists and is non-empty, `tasks.md` has no unchecked boxes, a
   verification report was actually written — before a transition is
   allowed to apply.
2. **The PreToolUse hook** (`mulix hook`) intercepts every `Write`/`Edit`
   call Claude Code makes and blocks any file outside the current phase's
   whitelist, independent of whether the agent remembers to check first.

## Install into a project

```
mulix init
```

This writes `.mulix/` (shared constitution template, bundled templates,
the `active`-change marker once a change exists), `.claude/skills/mulix-*/SKILL.md`
(one skill per phase, plus a `mulix-using-mulix` bootstrap skill), and
merges a `PreToolUse` hook entry into `.claude/settings.json` without
disturbing any hooks/settings you already have. It does not create
`docs/specs/` or `docs/changes/` — those come from `mulix new`, once there's an
actual change to hold. Re-running `mulix init` is safe (it skips files
that already exist); pass `--force` to overwrite mulix-owned files after
an upgrade.

If the [superpowers](https://github.com/obra/superpowers) plugin isn't
detected (project-scope `.claude/plugins/`, or the user-scope
`~/.claude/plugins/installed_plugins.json` entry Claude Code's plugin
installer writes), `mulix init` prints a one-line, non-blocking hint
about it — the build phase can use its skill chain to delegate task
execution to a reviewed subagent chain (see `mulix-build` below), but
nothing requires it.

## Everyday commands

```
mulix new "<title>"              # create a change, starts in the specify phase
mulix state show [--change ID]   # see the current phase and what can happen next
mulix state set <field> <value>  # record an artifact path or flag (does not change phase)
mulix state transition <event>   # run guards, then advance the phase if they pass
mulix guard <event>              # run guards read-only, without transitioning
mulix status                     # list every change and its phase
```

`mulix hook` is the `PreToolUse` entry point Claude Code invokes; you
won't normally run it by hand.

## Templates

`assets/templates/*.md` (installed to `.mulix/templates/` by `mulix
init`) closely mirror spec-kit's own `templates/*.md`: same section
headings, the same `FR-###`/`SC-###` requirement numbering, the same
Setup/Foundational/User-Story task phases, the same `## Clarifications`
and Constitution Check conventions. This is deliberate — content produced
by mulix should be recognizable to anyone coming from spec-kit. The one
structural template spec-kit has that mulix doesn't port is
`checklist-template.md`, since mulix has no equivalent `/checklist`
command in its phase list.

Guards (`internal/guard`) only depend on two exact strings inside these
templates regardless of everything else in them: a literal `##
Clarifications` heading in `spec.md`, and literal `- [ ]` checkbox syntax
in `tasks.md`. Everything else is free-form prose the relevant skill fills
in.

The `SKILL.md` files that drive each phase (`assets/skills/mulix-*`)
mirror spec-kit's corresponding `templates/commands/*.md` prompt in the
same way — the specify/clarify/plan/tasks/analyze/verify/taskstoissues
skills port the substance of each spec-kit command's execution steps,
checklist formats, and severity/classification rules, without spec-kit's
`.specify/extensions.yml` hook mechanism, multi-script variants, or
`handoffs` field, since mulix has no equivalent infrastructure for any of
those. `mulix-build` merges spec-kit's `implement` command (task
execution order, progress/failure handling) with superpowers'
test-driven-development discipline and per-task spike/bounded/
architectural classification, all in one skill, since mulix treats build
as a single phase. When superpowers' planning/dispatch skill chain
(`writing-plans`, `subagent-driven-development`/`executing-plans`,
`test-driven-development`) is installed, `mulix-build` prefers delegating
tasks.md execution to it instead of running tasks directly — one clean-
context subagent per task, spec-compliance and code-quality review before
a task counts as done — and records that with `mulix state set
delegated_to_subagents true`; that flag is informational only, since the
build-complete guard's test-evidence check applies identically either
way. tasks.md itself stays a pure task list (checkbox + one-line
description per task): the per-task requirements live in
`docs/changes/<change>/.runtime/sdd/task_<ID>_brief.md` and the
per-task execution records in `task_<ID>_report.md` — a `### Task`
checklist with one checkbox per TDD phase (RED, GREEN, optional
REFACTOR), whose RED and GREEN boxes must be ticked for the guard to
pass. Alongside them sit the chain's `progress.md` (ledger),
`review-*.diff` (review packages), `review.md` (two-phase review
verdicts), and `dispatch.md` (dispatch plan). Those artifacts go under
`docs/changes/<change>/.runtime/sdd/`, the one subdirectory the
PreToolUse hook carves out of the otherwise fully-blocked `.runtime/`
tree, and only during the build phase. `mulix-archive` and `mulix-using-mulix` have no
spec-kit counterpart (spec-kit has no archive phase or cross-phase meta
command) and are unchanged; spec-kit's `constitution` and `converge`
commands likewise have no dedicated mulix skill — constitution is handled
directly by `mulix init`, and converge (scanning the codebase for gaps
against spec/plan/tasks and appending catch-up tasks) is a newer spec-kit
feature with no equivalent step in mulix's current phase list.

## Presets

A preset overrides one or more of mulix's bundled templates
(`spec-template`, `plan-template`, etc). The override stack, checked in
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
```

Module: `github.com/mulix-dev/mulix-coding`, Go 1.27.

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

