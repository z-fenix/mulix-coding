# Tasks: Add Task Priority Levels

**Change**: `001-add-task-priority-levels`
**Input**: Design documents from `docs/changes/001-add-task-priority-levels/` and the spec
from `docs/specs/001-add-task-priority-levels/`
**Prerequisites**: plan.md, spec.md

**Tests**: included as explicit tasks below, test-first per the plan's Test
Strategy.

**Organization**: this is a small bounded change touching one existing
model and a few CLI commands — tasks are listed in dependency order
without the Setup/Foundational/User-Story phase scaffolding, since
there's no new project structure or shared infrastructure to stand up.

## Format: `[ID] [P?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- Include exact file paths in descriptions

## Tasks

- [x] T001 [P] Write `PriorityOrDefault` and priority-sort tests in
      src/models/task_test.go (extends existing test file), covering:
      empty string → "medium", and a slice sort ordering
      high/medium/low with oldest-first ties. Confirm these fail first.
- [x] T002 [US1] Add `Priority string` field (`json:"priority,omitempty"`)
      to the `Task` struct and implement `PriorityOrDefault()` in
      src/models/task.go, making T001 pass.
- [x] T003 [P] Write tests/unit/backward_compat_test.go: loading a store
      file containing a task JSON object with no `priority` key yields
      `PriorityOrDefault() == "medium"`. Confirm it fails first (there's
      no store-loading path exercising this yet).
- [x] T004 Confirm T003 passes against the T002 implementation (no
      separate loader change expected, since `omitempty` + zero-value
      string already round-trips correctly — this task exists to prove
      that assumption rather than to add code).
- [x] T005 [P] Write tests/unit/add_test.go: `--priority high|medium|low`
      sets the field; omitting `--priority` defaults to `medium`; an
      invalid value exits non-zero and writes no task. Confirm it fails
      first.
- [x] T006 [US1] Add `--priority` flag, validation, and default handling
      to src/cli/add.go, making T005 pass.
- [x] T007 [P] Write tests/unit/list_sort_test.go: `todo list` output
      order matches FR-005/FR-006 across mixed priorities and creation
      times. Confirm it fails first.
- [x] T008 [US2] Implement priority-ordered sort in src/cli/list.go
      (depends on T002 for `PriorityOrDefault`), making T007 pass.
- [x] T009 [P] Write tests/unit/set_priority_test.go: valid id updates
      priority; unknown id exits non-zero with "task not found" and
      leaves the store unmodified. Confirm it fails first.
- [x] T010 [US3] Implement `todo set-priority <id> <level>` in
      src/cli/set_priority.go (depends on T002), making T009 pass.
- [x] T011 Run the full test suite (`go test ./...`) and confirm every
      test above passes together, not just individually.

## Dependencies & Execution Order

- T001 before T002 (test-first); T003 before T004; T005 before T006;
  T007 before T008; T009 before T010.
- T002 is a prerequisite for T008 and T010 (`PriorityOrDefault` must
  exist before list/set-priority can use it).
- T001, T003, T005, T007, T009 (the test-writing tasks) touch different
  files and have no dependencies on each other — safe to write in
  parallel before starting the corresponding implementation tasks.
- T011 depends on all of the above.

## Notes

- All tasks above are complete: `go build ./...`, `go vet ./...`, and
  `go test ./...` all pass. See report.md for the full verify-phase
  results.
