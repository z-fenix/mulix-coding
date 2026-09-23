# T002 — Brief: Priority field, PriorityOrDefault, sort helper

Task: Add `Priority string` field (`json:"priority,omitempty"`) to the
`Task` struct and implement `PriorityOrDefault()` and the priority sort
helper in src/models/task.go, making T001 pass.

Context for the implementer:

- T001's tests are the acceptance bar: empty → "medium", explicit
  levels round-trip, high/medium/low order, oldest-first ties (the tie
  test seeds the slice later-task-first, so the sort must compare
  CreatedAt itself, not rely on stability).
- `omitempty` keeps pre-feature JSON (no priority key) decoding
  cleanly — do not add a loader change.
- Centralize the medium default in one accessor; T004/T008/T010 consume
  it.

Dispatched to: subagent-1.
