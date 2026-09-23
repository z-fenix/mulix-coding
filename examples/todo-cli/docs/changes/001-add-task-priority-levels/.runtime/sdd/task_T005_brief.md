# T005 — Brief: add-flag tests

Task: Write tests/unit/add_test.go — `--priority high|medium|low` sets
the field, omitting it defaults to `medium`, an invalid value exits
non-zero and writes no task, and the flag is parsed correctly when it
trails the description; confirmed failing first.

Context for the implementer:

- All tests drive the real `cli.Run` argv layer with `TODO_FILE` at a
  temp store.
- The trailing-flag case pins the documented usage shape
  (`todo add <description> --priority <level>`); a `flag.FlagSet`-style
  parser that stops at the first positional would fail it.

Dispatched to: subagent-3 (wave 1).
