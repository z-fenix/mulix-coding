# T008 — Report: priority-ordered listing

### Task
- [x] RED: T007's failing tests are the bar
- [x] GREEN: listing moved from root.go into src/cli/list.go — loads
      tasks, calls `models.SortByPriority`, prints each line with its
      resolved priority via `PriorityOrDefault()`
      (makes T007 pass)
- [ ] REFACTOR: fix round 1 below was contract work, not cleanup —
      kept out of this phase

Result: `go test ./tests/unit/` — both list tests pass.

## Fix round 1 (review finding)

Review round 2 flagged the GREEN implementation: it relied purely on
sort stability for the tie-break, which only works when the input slice
happens to be in creation order — the contract wasn't self-contained.
`TestSortByPriority_TiesBreakOldestFirst` in src/models/task_test.go
(seeds the slice in reverse creation order) caught the same hole from
the model side.

Fix: `SortByPriority` now compares `CreatedAt.Before` explicitly as the
in-priority tie-break. Full suite re-run: green. Diff:
review-T008-fix1.diff in this directory.
