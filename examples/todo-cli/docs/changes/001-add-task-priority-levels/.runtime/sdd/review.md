# Build Review Records — 001-add-task-priority-levels

Per-task review outcomes for the dispatch plan in dispatch.md. Two
rounds per task: (1) spec compliance against the task line and its FR,
(2) code quality. A task's line in tasks.md is checked off only after
both rounds pass.

| Task | Round 1 (spec) | Round 2 (quality) | Notes |
|------|----------------|-------------------|-------|
| T001 | pass | pass | Covers empty→medium, explicit levels, order, tie-break |
| T002 | pass | pass | Default centralized in `PriorityOrDefault`; omitempty keeps old JSON decodable |
| T003 | pass | pass | Asserts via the real CLI path (list shows medium), not a loader internal |
| T004 | pass | — | Verification-only; confirmed `omitempty` round-trip assumption holds |
| T005 | pass | pass | Found one test bug during review: the first seed of the tie-break test omitted `--priority high`, so it wasn't testing a tie; fixed before T008 review |
| T006 | pass | pass | Hand-rolled scan (no flag.FlagSet) so trailing `--priority` is honored; invalid value rejected before any store write |
| T007 | pass | pass | Asserts on real `todo list` output ordering |
| T008 | pass | pass | Review round 2 flagged the initial stability-only tie-break as order-dependent; `SortByPriority` now compares `CreatedAt` explicitly, making the contract input-order-independent |
| T009 | pass | pass | Unknown id leaves store byte-identical (asserted before/after) |
| T010 | pass | pass | Errors go to stderr with non-zero exit; store untouched on failure |
| T011 | pass | — | Full suite green: 11/11 tests across models + tests/unit |

## Fix Loop Log

- T008 initial implementation relied purely on sort stability for the
  tie-break, which only works when the input slice is already in
  creation order. The wave-1 test (`TestSortByPriority_TiesBreakOldestFirst`
  in src/models/task_test.go) caught it; fix loop iteration 1 added the
  explicit `CreatedAt.Before` comparator. Re-reviewed: pass.
- No other task needed more than one review round.
