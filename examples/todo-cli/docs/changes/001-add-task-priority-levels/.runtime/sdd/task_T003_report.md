# T003 — Report: backward-compat test

### Task
- [x] RED: wrote TestLoad_TaskWithoutPriorityFieldDefaultsToMedium in
      tests/unit/backward_compat_test.go — store file whose task JSON
      has no `priority` key at all, listed through the real `cli.Run`
      argv layer
- [ ] REFACTOR: none — test-only task

Result: FAIL — list output carries no priority annotation yet.
Failing for the right reason.
