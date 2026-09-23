# T010 — Report: set-priority command

### Task
- [x] RED: T009's failing tests are the bar
- [x] GREEN: new src/cli/set_priority.go; root.go dispatch gains
      `set-priority` — id parsed with strconv, level validated against
      the three levels (usage errors exit 2 before any store access);
      unknown id prints "task not found" on stderr and exits non-zero
      with the store untouched (save only happens on the found path)
      (makes T009 pass)
- [ ] REFACTOR: two test-side corrections during verification — the
      implementation was right in both cases: (1) T009's tie test seed
      omitted `--priority high` so it wasn't actually a tie; (2) the
      runTodo helper captured only stdout but "task not found"
      correctly goes to stderr. Test helper and seed fixed

Result: `go test ./tests/unit/` — all green.
