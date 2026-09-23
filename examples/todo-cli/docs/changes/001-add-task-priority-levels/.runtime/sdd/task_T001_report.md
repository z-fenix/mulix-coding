# T001 — Report: model priority tests

### Task
- [x] RED: wrote four failing tests in src/models/task_test.go —
      empty string → "medium", explicit low/medium/high round-trip,
      high/medium/low ordering, oldest-first ties (slice seeded with
      the later task first, so only a CreatedAt-aware sort passes)
- [ ] REFACTOR: none — test-only task

Result: `go test ./src/models/` fails to compile (no Priority field,
no PriorityOrDefault, no SortByPriority) — failing for the right
reason: the feature is missing.
