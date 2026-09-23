# Build Dispatch Plan — 001-add-task-priority-levels

Phase: build | Mode: subagent dispatch chain | Recorded: 2026-09-23

Every dispatched subagent loads the test-driven-development skill before
writing code; each task below is dispatched only after its test-writing
counterpart has been reviewed (see review.md).

## Artifacts in this directory

- `progress.md` — the ledger: per-task status, fix rounds, rulings.
- `task_<ID>_brief.md` — per-task requirements (written pre-dispatch).
- `task_<ID>_report.md` — per-task execution record: `### Task`
  checklist with the RED/GREEN/REFACTOR phases ticked as they complete.
- `review-*.diff` — review packages (diff per reviewed range).
- `review.md` — two-phase review verdicts and the fix-loop log.


## Waves

### Wave 1 (test-writing tasks, parallel — independent files)

| Task | Dispatch | Files touched |
|------|----------|---------------|
| T001 | subagent-1 | src/models/task_test.go |
| T003 | subagent-2 | tests/unit/backward_compat_test.go |
| T005 | subagent-3 | tests/unit/add_test.go |
| T007 | subagent-4 | tests/unit/list_sort_test.go |
| T009 | subagent-5 | tests/unit/set_priority_test.go |

Gate: every wave-1 task must end RED (tests fail against the current
code) before its implementation task is dispatched.

### Wave 2 (implementation tasks, dependency-ordered)

| Task | Depends on | Files touched |
|------|-----------|---------------|
| T002 | T001 | src/models/task.go |
| T004 | T002, T003 | — (verification-only) |
| T006 | T005 | src/cli/add.go (+ root.go dispatch entry) |
| T008 | T002, T007 | src/cli/list.go (+ root.go dispatch entry) |
| T010 | T002, T009 | src/cli/set_priority.go (+ root.go dispatch entry) |

T006/T008/T010 each touch root.go's dispatch switch, so they are
dispatched serially, not in parallel.

### Wave 3

| Task | Depends on | Purpose |
|------|-----------|---------|
| T011 | all | full-suite green run |

## Review Protocol

Each wave-2 task's output passes through two review rounds before its
task line is checked off in tasks.md:

1. Spec compliance — does the change satisfy the task's description and
   the FR it traces to in analyze.md's coverage table?
2. Code quality — naming, error handling, no scope creep beyond the
   task line.

Fix loops repeat until both rounds pass; results are logged in review.md.
