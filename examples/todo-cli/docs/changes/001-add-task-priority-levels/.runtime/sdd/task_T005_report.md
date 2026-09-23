# T005 — Report: add-flag tests

### Task
- [x] RED: wrote four tests in tests/unit/add_test.go, all through the
      real argv layer — stored level per flag value (low/medium/high
      table), medium default when omitted, invalid value exits
      non-zero and writes nothing, trailing `--priority high` parsed
      correctly
- [ ] REFACTOR: none — test-only task

Result: all four FAIL — flag ignored (no priority stored), no default,
invalid value accepted with exit 0, trailing flag silently dropped.
Failing for the right reasons.
