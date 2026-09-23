# T009 — Report: set-priority tests

### Task
- [x] RED: wrote two tests in tests/unit/set_priority_test.go through
      the real argv layer — valid id updates the stored priority;
      unknown id exits non-zero with "task not found" and leaves the
      store byte-identical (read before/after and compared)
- [ ] REFACTOR: none — test-only task

Result: both FAIL — `todo: unknown command "set-priority"`. Failing for
the right reason.
