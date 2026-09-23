# T001 — Brief: model priority tests

Task (from tasks.md): Write `PriorityOrDefault` and priority-sort tests
in src/models/task_test.go (extends the existing test file), covering
empty string → "medium" and a slice sort ordering high/medium/low with
oldest-first ties; confirmed failing first.

Context for the implementer:

- `Task` today is ID/Description/CreatedAt/Done (src/models/task.go);
  the Priority field does not exist yet.
- The test file already holds the NextID tests — extend it, don't
  replace it.
- The tie test must seed the slice with the later task first, so it can
  only pass if the sort itself orders by CreatedAt (not merely stable).

Dispatched to: subagent-1 (wave 1).
