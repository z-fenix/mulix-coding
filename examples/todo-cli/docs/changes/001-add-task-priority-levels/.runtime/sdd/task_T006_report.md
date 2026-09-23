# T006 — Report: add argument parsing and validation

### Task
- [x] RED: T005's four failing tests are the bar
- [x] GREEN: new src/cli/add.go — `parseAddArgs` scans the argument
      list for `--priority` wherever it appears (no `flag.FlagSet`); a
      missing flag value, an invalid level (exit 2, before any store
      write), and extra positional arguments are rejected; defaults to
      `models.PriorityMedium`. root.go's runAdd moved there
      (makes T005 pass)
- [ ] REFACTOR: usage() text updated to document the flag — no
      structural cleanup needed

Result: `go test ./tests/unit/` — all green including the trailing-flag
regression test.
