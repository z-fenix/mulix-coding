---
name: mulix-using
description: Use at the start of any session in a mulix-managed project, and whenever about to write or edit a file, to check which phase gates apply right now.
---

# Using mulix

mulix provides a seven-phase workflow with write-tool checks and
transition evidence guards, backed by agent discipline:

```
specify → clarify → design → tasks → build → verify → archive
```

| Phase   | Skill           | Embedded skill it runs                          | Output |
|---------|-----------------|-------------------------------------------------|--------|
| specify | `mulix-specify` | —                                               | `docs/specs/<change>/spec.md` |
| clarify | `mulix-clarify` | —                                               | spec.md `## Clarifications` |
| design  | `mulix-design`  | `brainstorming`                                 | approved design; `.mulix/.runtime/<change>/specs/*-design.md` (architectural) |
| tasks   | `mulix-tasks`   | `writing-plans`                                 | `docs/changes/<change>/tasks.md` (`## Task N` plan) |
| build   | `mulix-build`   | `test-driven-development` + `subagent-driven-development` or `executing-plans` | code, tests; `.mulix/.runtime/<change>/sdd/tasks/` (ledger, briefs, reports) |
| verify  | `mulix-verify`  | `verification-before-completion`                | `docs/changes/<change>/report.md` |
| archive | `mulix-archive` | `finishing-a-development-branch`                | merged / PR / kept |

1. On Claude Code, a **PreToolUse hook** checks only `Write`/`Edit`
   against the current phase's whitelist. Read a deny's reason and next
   action; don't retry the same call or route around it. **Bash, MCP,
   NotebookEdit, and other writers are not automatically gated.** Missing
   `file_path` tool-input fields are allowed. Legacy mode keeps fail-open
   parsing/root/active/state/output errors. In session mode, invalid bindings,
   configuration, active/state loading, or lock failures block writes when the
   project is identifiable; only a missing binding permits global fallback.
   No current change allows ordinary writes outside runtime, but retained runtime
   needs an owning change and session control metadata is still CLI-only. An unidentifiable project root warns and allows the call.
   Hosts without this hook (such as DeepSeek Harness) rely on your
   discipline for all writes. This is not a filesystem security sandbox.
2. **Guard checks** (`mulix guard <event>`, run automatically inside
   `mulix state transition <event>`) check recorded evidence (artifact
   existence, acceptance mappings, approval flags, ledger completion and
   source-bound runner receipts). Use `mulix test run` and `mulix verify` for
   actual executions; read `.mulix/templates/quality-template.md` when configuring
   suites or interpreting receipts. Local records do not authenticate human
   consent, assertion quality or a writer controlling all files. `--force` with
   a nonempty `--reason` records a recovery bypass, never test qualification,
   and cannot make an illegal event legal.

## Session context

Run `mulix context show` to read mode, session, change and source. With no
configuration the mode is **legacy** and commands use global `.mulix/active`.
Enable session mode only on the human's instruction with
`mulix context mode session`.

In **session** mode, use the host-provided identity: `MULIX_SESSION_ID` from
SessionStart, or explicit `--session-id '<host-id>'` when it is not inherited.
Explicit identity overrides the environment. No identity means global use;
an empty/invalid supplied identity is an error, not permission to guess one.

| Source | Next action |
| --- | --- |
| `session-binding` | Work on that bound change, regardless of global active. |
| `global-fallback` | No binding exists; this invocation uses global active. A later global switch affects the next invocation. |
| Binding invalid or archived | Repair with `mulix state select <change> --session-id '<host-id>'`, or explicitly unbind with `mulix context unbind --session-id '<host-id>'`. A broken binding never falls back. |

In session mode with identity, `new` / `state select` bind only this session.
`--global` on those two commands changes global active without changing
bindings. Unbind removes only this session record and resumes global fallback.
Archiving retains bindings and clears global active only if it still names
that archived change: an A→X binding with global Y stays bound to archived X,
not Y, until explicitly repaired or unbound.

For retained runtime artifacts, query `mulix context show --field runtime-dir`
with the same identity; the SDD scripts use that query themselves. For temporary
material, query `mulix context show --field scratch-dir`; the brainstorm launcher
uses this query. A query error stops creation. With no current change, JSON
inspection succeeds with an empty `runtime_dir` and a valid `scratch_dir`, while
querying `runtime-dir` alone fails. Each running tool/script keeps its startup
snapshot; a switch affects the next call, not an already-running process.

## Runtime artifact policy

- Retained change output is `.mulix/.runtime/<change>/state.yaml`, `specs/`
  for approved design documents, and `sdd/<plan>/` for execution records.
  The final verification report is `docs/changes/<change>/report.md`.
- SDD uses the native files as needed: `plan-path`, `progress.md`,
  `task-N-brief.md`, `task-N-report.md`, `task-N-tests.log`, and
  `review-<base>..<head>.diff`, plus `sdd/.gitignore`. Keep the workspace
  for guards and recovery; keep existing progress formats and RED/GREEN evidence.
- Temporary companion files go under `<scratch-dir>/brainstorm/` in design;
  ad-hoc RED/rerun logs, build binaries and verification or migration material
  go under `<scratch-dir>/verification/` or `migrations/` in build/verify.
  Diagnostics use `<scratch-dir>/diagnosing-superpowers/` in any phase.
  Resolve the absolute scratch path first with the current identity, create
  private directories, and retain necessary command/output summaries in the
  existing task report, ledger or final report rather than more evidence files.
- A valid no-change context still has scratch for standalone companion/diagnosis;
  retained SDD work needs a selected change. Legacy `brainstorm/`, `verification/`,
  `migrations/` and `_shared/` inside runtime are history, not new destinations.
  Init/update preserve those files and their sensitive-file ignore rules.

## Before doing anything else

Run `mulix state show` to see the active change's phase and the events
legal from it. Don't assume the phase from conversation history — state
lives on disk in `.mulix/.runtime/<change>/state.yaml`, not in your
context.

Then invoke the phase's skill from the table before taking any action
in that phase. Each applies only while the active change is in its
phase.

## The embedded superpowers skills

`mulix init` installs every superpowers skill as a project skill in
`{{SKILLS_DIR}}/` (unprefixed: `brainstorming`, not
`superpowers:brainstorming`). They keep their full discipline; mulix
places retained artifacts under `.mulix/.runtime/`, transient material in
context-scoped system scratch, and replaces their hand-offs with phase gates:

- The phase skill decides when one runs. brainstorming ends at the
  design gate, writing-plans at the tasks gate (no execution handoff),
  the executors at build-complete (the branch is finished in archive).
- Supporting skills apply whenever their trigger fires, in any phase:
  `systematic-debugging` for any bug or unexpected failure,
  `verification-before-completion` before any "done/fixed/passing" claim,
  `receiving-code-review` when review feedback arrives,
  `dispatching-parallel-agents` for independent investigations.
- Outside a mulix change, companion/diagnostic tools use the valid no-change
  scratch context. Plan executors need a selected change for retained SDD
  records; an empty or invalid runtime query never creates shared output.

## Starting a new change

`mulix new "<title>"` creates the change's spec directory
(`docs/specs/<change>/`), change directory (`docs/changes/<change>/`),
and runtime directory (`.mulix/.runtime/<change>/`) plus a fresh state in
the specify phase, then selects it for the effective context: the current
session in session mode with identity, otherwise global active. Do this before
anything else for a new piece of work — don't start writing a spec by
hand first. New IDs use the local date, e.g. `2026-10-14-add-login`,
with `-2`, `-3` for same-day/title collisions. All three directories share
that ID; filenames stay spec.md/tasks.md/report.md. Old `001-*` changes
remain selectable with `mulix state select <change>`; don't rename them.

## Red flags — stop and reconsider

- "I'll just write the code first and backfill the phase later" — this
  is exactly what the phase gates exist to prevent: the hook on Claude
  Code, the guards and this discipline on every host. Routing around
  them is a bug, not a shortcut.
- "brainstorming says to invoke writing-plans / the plan says to execute
  now" — under mulix the next step is always the phase gate. Transition,
  then the next phase's skill takes over.
- "The guard check is probably fine to skip with --force" — `--force`
  exists for genuine recovery situations, not for impatience. Read the
  failed check's remediation hint first.
- "I'll infer the user approved this from them not objecting" — the
  design approval, the execution method, and archival all require an
  *explicit* answer recorded in state, not inferred from silence.
