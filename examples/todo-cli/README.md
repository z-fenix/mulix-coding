# todo-cli

A minimal example CLI built to exercise the mulix spec-driven workflow
end to end. See `docs/specs/001-add-task-priority-levels/spec.md` for
the requirements doc, and `docs/changes/001-add-task-priority-levels/`
for the plan/tasks/analyze/report artifact trail.

## Commands

- `todo add <description> [--priority low|medium|high]` — add a task.
  Defaults to `medium` when `--priority` is omitted.
- `todo list` — list tasks, ordered high → medium → low priority, ties
  broken by creation order (oldest first).
- `todo set-priority <id> <low|medium|high>` — change an existing task's
  priority. Exits non-zero with "task not found" for an unknown id.

## Priority Behavior

- Three levels: `low`, `medium`, `high`
- Default: `medium` (used when `--priority` is omitted during `add`)
- Backward compatibility: tasks created before this feature (no `priority`
  field in JSON) are treated as `medium` when listing
- Sort order: `list` always outputs high → medium → low, with same-priority
  tasks sorted oldest-first by creation time

## Implementation Notes

This example was built through mulix's 8-phase workflow (specify →
clarify → plan → tasks → analyze → build → verify → archive), following
TDD discipline (test-first red-green-refactor) during the build phase.
All 11 tasks from `docs/changes/001-add-task-priority-levels/tasks.md`
were completed in dependency order.

Manual smoke-testing during the verify phase (running the built binary
directly, not just the unit tests) caught a real bug that the unit
tests had missed: the CLI's argv parsing silently dropped
`--priority` when it trailed the task description
(`todo add "buy milk" --priority high`), because Go's `flag` package
stops parsing at the first non-flag argument. See
`docs/changes/001-add-task-priority-levels/report.md` for the root
cause and fix.

## Layout

```text
docs/specs/001-add-task-priority-levels/
└── spec.md                # requirements doc (durable reference)

docs/changes/001-add-task-priority-levels/
├── plan.md                # plan phase
├── tasks.md                # tasks phase
├── analyze.md              # analyze phase
├── report.md               # verify phase
└── .runtime/state.yaml     # mulix's own state for this change

src/                        # implementation
tests/unit/                 # tests outside src/models (which has its own _test.go)
cmd/todo/main.go            # CLI entry point
```
