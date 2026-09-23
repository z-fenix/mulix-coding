# T009 — Brief: set-priority tests

Task: Write tests/unit/set_priority_test.go — valid id updates priority;
unknown id exits non-zero with "task not found" and leaves the store
unmodified; confirmed failing first.

Context for the implementer:

- Unknown-id test must compare store bytes before/after (read twice and
  diff), not mtime.
- Error text goes to stderr; capture both streams in the test helper.

Dispatched to: subagent-5 (wave 1).
