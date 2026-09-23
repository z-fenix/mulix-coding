# T010 — Brief: set-priority command

Task: Implement `todo set-priority <id> <level>` in
src/cli/set_priority.go (depends on T002), making T009 pass.

Context for the implementer:

- Validate id (strconv) and level against the three levels; usage
  errors exit non-zero before any store read/write.
- Unknown id: "task not found" on stderr, non-zero exit, store never
  written (save only happens on the found path).

Dispatched to: subagent-5.
