# T007 — Report: list-sort tests

### Task
- [x] RED: wrote two tests in tests/unit/list_sort_test.go through the
      real argv layer — mixed low/high/medium seeded in that order must
      list high→medium→low; two same-priority tasks must list the older
      one first
- [ ] REFACTOR: none — test-only task

Result: `TestList_OrdersHighMediumLow` FAIL (creation order preserved,
no sorting). `TestList_TiesBreakOldestFirst` passes trivially for now —
with no sort, creation order is the output order; it becomes a real
constraint once the sort exists.
