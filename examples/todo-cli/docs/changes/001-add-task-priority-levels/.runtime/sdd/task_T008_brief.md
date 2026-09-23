# T008 — Brief: priority-ordered listing

Task: Move listing into src/cli/list.go with the priority-ordered sort
(depends on T002), making T007 pass.

Context for the implementer:

- Print each line with its resolved priority via
  `PriorityOrDefault()` so pre-priority tasks display as medium.
- The sort lives in the models package (T002's helper); the CLI only
  calls it.

Dispatched to: subagent-4.
