# T004 — Report: backward-compat against T002 (verification-only)

### Task
- [x] RED: T003's failing test is the bar this task confirms against
- [x] GREEN: no loader change written — re-ran
      TestLoad_TaskWithoutPriorityFieldDefaultsToMedium after T002
      landed: passes. `omitempty` + zero-value string round-trips
      exactly as assumed; the assumption is now proven, not assumed
- [ ] REFACTOR: none — no code written
