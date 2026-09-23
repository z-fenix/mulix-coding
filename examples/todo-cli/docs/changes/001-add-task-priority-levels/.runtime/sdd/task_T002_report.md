# T002 — Report: Priority field, PriorityOrDefault, sort helper

### Task
- [x] RED: re-ran T001's tests — compile failure against the unmodified
      model (Priority field undefined); the right failure
- [x] GREEN: minimal implementation in src/models/task.go —
      `Priority string` with `json:"priority,omitempty"`,
      `PriorityOrDefault()` switching on the three levels (default
      medium), `priorityRank` + stable `SortByPriority`
      (makes T001's RED tests pass)
- [ ] REFACTOR: none — the medium default already lives in the single
      accessor T004/T008/T010 will consume

Result: `go test ./src/models/` — all 6 tests pass.
