# Feature Specification: Add Task Priority Levels

**Change**: `001-add-task-priority-levels`

**Created**: 2025-01-01

**Status**: Draft

**Input**: User description: "add task priority levels"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Set priority when adding a task (Priority: P1)

A user adding a new task wants to mark how urgent it is, so tasks that
matter more stand out from routine ones.

**Why this priority**: Without a way to set priority at creation time,
the feature has no entry point — every other story depends on tasks
actually having a priority value.

**Independent Test**: Can be fully tested by running `todo add "some
task" --priority high` and confirming the stored task has
`priority: high`. Delivers value on its own: the data model change is
visible and usable immediately.

**Acceptance Scenarios**:

1. **Given** an empty task list, **When** the user runs
   `todo add "buy milk" --priority high`, **Then** a task is created
   with priority `high`.
2. **Given** an empty task list, **When** the user runs
   `todo add "buy milk"` without `--priority`, **Then** a task is
   created with the default priority `medium`.

---

### User Story 2 - List tasks ordered by priority (Priority: P2)

A user viewing their task list wants the most urgent tasks to appear
first, so they don't have to scan the whole list to find what matters.

**Why this priority**: Setting a priority is only useful if it changes
what the user sees. This is the story that makes User Story 1's data
visible in a useful order.

**Independent Test**: Can be fully tested by adding tasks with mixed
priorities and running `todo list`, then checking the output order.

**Acceptance Scenarios**:

1. **Given** tasks with priorities low, high, and medium (added in that
   order), **When** the user runs `todo list`, **Then** the output
   order is high, medium, low.
2. **Given** two tasks with the same priority added at different times,
   **When** the user runs `todo list`, **Then** the older task is
   listed first (ties broken by creation order, oldest first).

---

### User Story 3 - Change an existing task's priority (Priority: P3)

A user realizes a task's urgency has changed after creating it, and
wants to update the priority without deleting and recreating the task.

**Why this priority**: Useful, but the feature is already viable
without it — a user can work around a wrong priority by deleting and
re-adding the task. This story removes that friction.

**Independent Test**: Can be fully tested by adding a task, running
`todo set-priority <id> high`, and confirming the stored priority
changed.

**Acceptance Scenarios**:

1. **Given** an existing task with priority `low`, **When** the user
   runs `todo set-priority <id> high`, **Then** the task's priority is
   `high`.
2. **Given** no task exists with the given id, **When** the user runs
   `todo set-priority <id> high`, **Then** the command exits non-zero
   with a "task not found" error and no task is modified.

### Edge Cases

- What happens when `--priority` is given a value other than
  `low`/`medium`/`high`? The command must reject it with a clear error
  and exit non-zero, rather than silently storing an invalid value.
- How does the system handle tasks that were created before this
  feature existed (no `priority` field in their stored JSON)? They
  must be treated as `medium` rather than causing a parse error or
  being sorted incorrectly.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support three priority levels: `low`,
  `medium`, `high`.
- **FR-002**: `todo add` MUST accept an optional `--priority` flag
  accepting one of the three levels.
- **FR-003**: `todo add` MUST default to `medium` when `--priority` is
  omitted.
- **FR-004**: `todo add` MUST reject any `--priority` value outside the
  three levels with a non-zero exit and a clear error message.
- **FR-005**: `todo list` MUST order tasks high → medium → low.
- **FR-006**: `todo list` MUST break ties within the same priority by
  creation order, oldest first.
- **FR-007**: System MUST provide `todo set-priority <id> <level>` to
  change an existing task's priority.
- **FR-008**: `todo set-priority` MUST exit non-zero with a "task not
  found" error when given an unknown id, without modifying any task.
- **FR-009**: System MUST treat tasks stored without a `priority` field
  (pre-existing data from before this feature) as `medium`.

### Key Entities

- **Task**: A single to-do item. Existing attributes (id, description,
  created-at, done) are unchanged. This feature adds `priority: low |
  medium | high`, defaulting to `medium` when absent.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can set a task's priority at creation time and see
  it reflected in `todo list`'s ordering, with zero additional setup.
- **SC-002**: Existing tasks created before this feature continue to
  load and list correctly (treated as `medium`), with no data
  migration step required from the user.
- **SC-003**: Invalid `--priority` input is rejected before any task is
  written, so the task list is never left in a partially-applied state.

## Assumptions

- The existing task store format is a JSON file on disk, one task per
  entry; this feature adds a field rather than changing the storage
  mechanism.
- Priority is the only new sort dimension — no request for
  secondary manual ordering (drag-and-drop, custom ranks) is in scope.
- This is a single-user, local CLI tool; no concurrent-write conflict
  handling is needed beyond what already exists.

## Out of Scope

- Custom/user-defined priority levels beyond low/medium/high.
- Bulk priority updates (changing multiple tasks' priority in one
  command).
- Any notification or highlighting beyond sort order (e.g., color
  output, due-date-based priority escalation).

## Clarifications

No `NEEDS CLARIFICATION` markers were introduced above — the feature
description and defaults (medium as default, high→medium→low sort,
backward-compatible treatment of missing `priority`) were unambiguous
enough to specify directly. `clarify_skipped=true` is recorded in state
for this reason.
