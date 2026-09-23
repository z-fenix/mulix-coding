# T003 — Brief: backward-compat test

Task: Write tests/unit/backward_compat_test.go — a store file containing
a task JSON object with no `priority` key loads and lists as `medium`;
confirmed failing first.

Context for the implementer:

- Drive the real argv layer (`cli.Run` with `TODO_FILE` pointed at a
  temp store), not a loader internal — the pre-feature store shape is
  exactly `{"id","description","created_at","done"}`.
- Assert the list output shows `medium` for the old task.

Dispatched to: subagent-2 (wave 1).
