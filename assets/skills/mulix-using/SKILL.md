---
name: mulix-using
description: Use when starting a session, exploring a throwaway prototype, resuming a change, or about to write in a mulix-managed project.
---

# Using mulix

## Route the request before registering

Run `mulix context show` with the current session identity. A successful
result with an empty `change` is a valid no-change context. A resolution
error is not an empty context; stay read-only until it is resolved.

- **Exploration (spike):** The user wants to learn or try something out,
  not deliver it yet. Read-only investigation needs no workspace. For an
  authorized runnable prototype, require a valid no-change context. If a
  change is selected, keep that context read-only. Open a separately
  authorized, isolated workspace in the host and recheck `mulix context show`
  there before writing; changing the shell directory alone doesn't change
  the hook's context. In the no-change workspace, resolve `scratch-dir`,
  create that directory privately if absent, and make a fresh private
  subdirectory for the prototype. Use `brainstorming`'s standalone spike
  path. Keep delivery files, active/session selections and Issues unchanged.
  Report the findings and the location of any retained prototype; its test
  log is not delivery evidence.
- **Delivery:** The user wants to implement a change for delivery. Resume
  the intended change or [register a new one](#register-a-delivery-change).
  Then follow its current phase. An active change does not make an
  unrelated exploration part of that change.

If intent is unclear and it would determine whether to register a change,
ask whether this is a throwaway experiment or work to deliver. Do not ask
again when intent is already clear.

## The delivery workflow

mulix provides a seven-phase workflow with write-tool checks and
transition evidence guards, backed by agent discipline:

```
specify → clarify → design → tasks → build → verify → archive
```

| Phase   | Skill           | Embedded skill it runs                          | Output |
|---------|-----------------|-------------------------------------------------|--------|
| specify | `mulix-specify` | —                                               | `spec_path`; publish a bound spec only for a remote tracker |
| clarify | `mulix-clarify` | —                                               | spec `## Clarifications`; confirmed sync for a remote spec |
| design  | `mulix-design`  | `brainstorming`                                 | approved design; `specs/*-design.md` under runtime (architectural) |
| tasks   | `mulix-tasks`   | `writing-plans`                                 | `tasks_dir` task documents, or legacy `tasks_path` |
| build   | `mulix-build`   | `test-driven-development` + `subagent-driven-development` or `executing-plans` | code, tests, receipts and `sdd/<plan>/` ledger |
| verify  | `mulix-verify`  | `verification-before-completion`                | `report_path` and current verification receipt |
| archive | `mulix-archive` | `finishing-a-development-branch`                | local archive; optional separately authorized spec close |

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
   and cannot make an illegal event legal or bypass remote sync.

## Resolve the delivery phase

For the selected delivery change, run `mulix state show` before choosing a
phase skill. Use its phase and paths, not conversation history or the
project's default tracker:

| `artifact_layout` and `tracker` | Artifact and publication rules |
| --- | --- |
| `runtime-issues-v1`, `markdown` | Use the pinned `spec_path`, `tasks_dir` and `report_path`. All artifacts are local. |
| `runtime-issues-v1`, `github` or `gitlab` | Same pinned paths. Publish only through the phase's `mulix tracker publish` dry run and authorized `--expect` command. Once bound, the Issue body is authoritative and its local snapshot is read-only to ordinary writers. |
| Legacy (empty layout) | Use the existing spec, single `tasks_path` plan and report in `docs/`; run the existing local workflow without tracker commands. |

For a remote change, the forward guards require the verified project and
current spec Issue from `spec-complete`; `tasks-complete` through `archived`
also require an exact, current binding for every task document. For drift,
review `mulix tracker preview <spec|task-N>` and use `mulix tracker apply`
only with authorization for that exact token. For a local conflict, review
again with `--backup-local` and apply with the same flag and fresh token.
Never substitute a different writer or `--force`. `verify-fail` is local
recovery, not forward acceptance.

Invoke the skill for the current phase before acting. `setup-mulix` installs
unprefixed superpowers skills under `{{SKILLS_DIR}}/`, e.g. `brainstorming`.
The phase skill replaces their hand-offs: pass the CLI gate before invoking
an executor or the next phase's skill.

## Session context

Run `mulix context show` to read mode, session, change and source. With no
configuration the mode is **legacy** and commands use global `.mulix/active`.
Enable session mode only on the human's instruction with
`mulix context mode session`.

In **session** mode, use the host-provided identity: `MULIX_SESSION_ID` from
SessionStart, or explicit `--session-id '<host-id>'` when it is not inherited.
Explicit identity overrides the environment. No identity means global use;
an empty/invalid supplied identity is an error, not permission to guess one.
For bundled helpers that query context themselves (including the visual
companion), pass the same host identity as `MULIX_SESSION_ID` in the helper's
environment if SessionStart did not export it. A flag on a previous CLI
call is not inherited by the helper.

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

- Retained runtime output includes `state.yaml`, native `spec.md`, `issues/`
  and `report.md`, `specs/` for approved design documents, `tests/` for
  authored Karate sources, and `sdd/<plan>/` for execution records. Only build
  authors `tests/`; use `mulix test run` for task scope and `mulix verify` for
  final scope. Karate reports and rolling logs stay in scratch.
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

## Register a delivery change

For new work the user intends to deliver, run `mulix new "<title>"`
before drafting the delivery spec. It creates an owned runtime directory
and state in `specify`, and selects it for the effective context. With
tracker setup it pins the tracker and native paths; otherwise it creates
the legacy docs directories.

**Promotion** is registering a delivery change after the user decides to
ship an exploration. Bring the findings into Specify and keep prototype
files separate. Follow the same seven phases and tracker rules; no
exploration receipt, approval or passing log is inherited. In Build,
follow `mulix-build`'s promotion steps before bringing over source.
The decision to deliver does not authorize commits, pushes or publication.

IDs use the local date, e.g. `2026-10-14-add-login`, with `-2`, `-3` for
collisions. Existing changes keep their IDs and layouts; select them as-is.

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
