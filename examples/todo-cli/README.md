# todo-cli

A minimal example CLI built to exercise the mulix spec-driven workflow
end to end. See `docs/specs/001-add-task-priority-levels/spec.md` for
the requirements doc, and `docs/changes/001-add-task-priority-levels/`
for the plan/tasks/analyze/report artifact trail.

## Commands

- `todo add <description> [--priority low|medium|high]` — add a task.
  Defaults to `medium` when `--priority` is omitted. The flag is
  accepted in any position among add's arguments.
- `todo list` — list tasks, ordered high → medium → low priority, ties
  broken by creation order (oldest first).
- `todo set-priority <id> <low|medium|high>` — change an existing task's
  priority. Exits non-zero with "task not found" for an unknown id.

The task store is a JSON file (`tasks.json` in the working directory by
default; override with the `TODO_FILE` environment variable).

## Priority Behavior

- Three levels: `low`, `medium`, `high`
- Default: `medium` (used when `--priority` is omitted during `add`)
- Backward compatibility: tasks created before this feature (no `priority`
  field in JSON) are treated as `medium` when listing
- Sort order: `list` always outputs high → medium → low, with same-priority
  tasks sorted oldest-first by creation time

## Implementation Notes

This example was deleted and re-run end to end through mulix's 8-phase
workflow (specify → clarify → plan → tasks → analyze → build → verify →
archive) to validate the updated build-phase flow. During build:

- Execution ran through the subagent dispatch chain
  (`delegated_to_subagents: true` in state); the full SDD artifact set
  lives in `docs/changes/001-add-task-priority-levels/.runtime/sdd/`:
  `progress.md` (ledger), `task_<ID>_brief.md` (requirements, written
  pre-dispatch), `task_<ID>_report.md` (execution records), review
  packages (`review-*.diff`), `review.md` (review verdicts), and
  `dispatch.md` (the dispatch plan).
- All tasks followed TDD discipline (test-first red-green), with one
  real fix-loop catch: the initial stability-only tie-break in
  `SortByPriority` was order-dependent and failed its wave-1 test until
  an explicit `CreatedAt` comparison was added (diff archived as
  `review-T008-fix1.diff`).
- The evidence convention was revised twice during the run and settled
  here: tasks.md is a pure task list (checkbox + one-line description
  per task, nothing appended); each checked task's execution record is
  `task_<ID>_report.md`, structured as a `### Task` checklist where
  each checkbox is exactly one TDD phase — RED, GREEN, or optional
  REFACTOR — with RED/GREEN ticked as they complete. The
  build-complete guard requires every checked task's report with its
  RED and GREEN boxes ticked; there is no `[no-test]` opt-out.
- Delegation does not exempt a task from its report — the guard applies
  identically either way.

Manual smoke-testing during the verify phase (running the built binary
against every acceptance scenario) found no issues this run; the
flag-position parsing bug caught in the previous run of this example is
now prevented by design (hand-rolled argument scan, no `flag.FlagSet`)
and covered by `TestRun_AddWithTrailingPriorityFlagIsParsedCorrectly`.

## Layout

```text
docs/specs/001-add-task-priority-levels/
└── spec.md                 # requirements doc (durable reference)

docs/changes/001-add-task-priority-levels/
├── plan.md                 # plan phase
├── tasks.md                # tasks phase
├── analyze.md              # analyze phase
├── report.md               # verify phase
└── .runtime/
    ├── state.yaml          # mulix's own state for this change
    └── sdd/                # build-phase SDD artifacts (kept after archive)
        ├── progress.md     # ledger: per-task status, fix rounds, rulings
        ├── dispatch.md     # dispatch plan (waves, review protocol)
        ├── task_T00N_brief.md   # per-task requirements (pre-dispatch)
        ├── task_T00N_report.md  # per-task record: ### Task checklist,
        │                       # RED/GREEN(/REFACTOR) boxes ticked
        ├── review-T008-fix1.diff  # review package (diff per range)
        └── review.md       # two-phase review verdicts

src/                        # implementation
tests/unit/                 # tests outside src/models (which has its own _test.go)
cmd/todo/main.go            # CLI entry point
```
