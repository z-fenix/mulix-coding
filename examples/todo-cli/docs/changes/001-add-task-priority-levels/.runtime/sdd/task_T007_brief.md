# T007 — Brief: list-sort tests

Task: Write tests/unit/list_sort_test.go — `todo list` output order
matches FR-005/FR-006 across mixed priorities and creation times;
confirmed failing first.

Context for the implementer:

- Two tests: mixed low/high/medium added in that order must list
  high→medium→low; two same-priority tasks added at different times
  must list the older one first.
- Drive the real argv layer; assert on the printed order.

Dispatched to: subagent-4 (wave 1).
