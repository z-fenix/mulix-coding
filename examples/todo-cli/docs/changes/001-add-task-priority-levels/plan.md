# Implementation Plan: Add Task Priority Levels

**Change**: `001-add-task-priority-levels` | **Date**: 2026-09-23 | **Spec**: docs/specs/001-add-task-priority-levels/spec.md

**Input**: Feature specification from `docs/specs/001-add-task-priority-levels/spec.md`

## Summary

Add a `priority` field (`low`/`medium`/`high`, default `medium`) to the
Task model, sort `todo list` output by priority (high→medium→low, ties
broken oldest-first), and add `todo set-priority <id> <level>` to
change an existing task's priority.

## Technical Context

**Language/Version**: Go 1.27

**Primary Dependencies**: standard library only (`encoding/json`);
no new dependency needed for this change.

**Storage**: a single JSON file on disk, one task per array entry
(existing mechanism, unchanged).

**Testing**: `go test`

**Target Platform**: CLI (single-user, local)

**Project Type**: CLI

**Performance Goals**: N/A — task counts are small (personal to-do
list scale); no performance requirement beyond "instant" for local
disk I/O.

**Constraints**: must remain backward compatible with task JSON
written before this feature existed (no `priority` field present).

**Scale/Scope**: single user, local file; no concurrency concerns.

## Constitution Check

No `docs/constitution.md` exists for this example project — nothing to
gate against.

## Approach

Add `Priority` as a string-typed field on the existing `Task` struct
with `omitempty` so old JSON without the field decodes cleanly. Default
resolution (`medium` for empty/missing) is centralized in one accessor
so both `add` (when `--priority` is omitted) and `list`/`set-priority`
(when reading pre-existing tasks) apply the same rule instead of
duplicating the default in multiple places.

Sorting is implemented as a stable sort keyed on
(priority-rank-descending, created-at-ascending) so the tie-break rule
in FR-006 falls out of stability rather than needing a secondary
explicit comparison.

Argument parsing for `todo add` scans the argument list for
`--priority` wherever it appears (FR-002), rather than using
`flag.FlagSet`, which stops parsing at the first non-flag token — the
CLI's usage puts the description before the flag, so a `flag`-based
parser would silently drop it.

## Design Notes

`Task.Priority string` — one of `"low" | "medium" | "high"`, empty
string on disk (from old data) treated as `"medium"` by a single
`Task.PriorityOrDefault()` accessor used everywhere ordering or display
matters. Validation of the `--priority` flag value happens at the CLI
layer, before any task is constructed, so an invalid value never
reaches the store.

## Project Structure

### Documentation (this change)

```text
docs/specs/001-add-task-priority-levels/
└── spec.md               # Feature specification (specify phase)

docs/changes/001-add-task-priority-levels/
├── plan.md               # This file (plan phase)
├── tasks.md              # Dependency-ordered checklist (tasks phase)
├── analyze.md            # Cross-artifact consistency findings (analyze phase)
├── report.md             # Verification results (verify phase)
└── .runtime/
    ├── state.yaml        # mulix's own state for this change
    └── sdd/              # build-phase dispatch/review artifacts
```

### Source Code (repository root)

```text
src/
├── models/
│   ├── task.go            # Task struct + PriorityOrDefault, sort helper
│   └── task_test.go       # extended: priority default + sort comparator
├── services/
│   └── store.go           # JSON load/save (existing, unchanged shape)
└── cli/
    ├── root.go            # command dispatch (existing)
    ├── add.go             # new: --priority flag, validation, default
    ├── list.go            # priority-ordered listing
    └── set_priority.go    # new: todo set-priority <id> <level>

tests/
└── unit/
    ├── add_test.go
    ├── list_sort_test.go
    ├── set_priority_test.go
    └── backward_compat_test.go   # pre-existing tasks without priority
```

**Structure Decision**: Single-project CLI layout, matching the existing
`src/{models,services,cli}` + `tests/unit` structure already in place —
this feature extends it rather than restructuring.

## Risks

- Backward compatibility with pre-existing task JSON lacking
  `priority` is the main risk; mitigated by centralizing the default in
  `PriorityOrDefault()` and covering it with a dedicated test
  (`backward_compat_test.go`) rather than relying on every call site
  getting the default right independently.

## Test Strategy

Test-first, in dependency order:

1. `src/models/task_test.go` (pre-existing, extended): `PriorityOrDefault`
   returns `medium` for empty string, and the sort comparator orders
   high→medium→low with oldest-first ties.
2. `tests/unit/backward_compat_test.go`: loading a store file with a
   task JSON object that has no `priority` key at all yields
   `PriorityOrDefault() == "medium"`.
3. `tests/unit/add_test.go`: `--priority high|medium|low` sets the
   field; omitting it defaults to `medium`; an invalid value exits
   non-zero without writing a task; the flag is accepted in any
   argument position.
4. `tests/unit/list_sort_test.go`: `todo list` output order matches
   FR-005/FR-006 across mixed priorities and creation times.
5. `tests/unit/set_priority_test.go`: valid id updates priority; unknown
   id exits non-zero with "task not found" and leaves the store
   unmodified.

Each test is written and confirmed failing before the corresponding
implementation is written (red-green), per the build phase's TDD
discipline.

## Complexity Tracking

No constitution violations — nothing to justify.
