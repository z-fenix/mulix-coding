---
name: mulix-tasks
description: Use when the active change is in the tasks phase, to break the plan into a dependency-ordered checklist before implementation.
---

# Tasks phase

Load plan.md (tech stack, structure) and spec.md (user stories with
priorities) and generate `docs/changes/<change>/tasks.md`: an actionable,
dependency-ordered checklist organized by user story so each story is an
independently completable, independently testable increment.

## Checklist format (required)

Every task MUST follow this exact form:

```
- [ ] T001 [P] [US1] Description with file path
```

- **Checkbox**: always `- [ ]`. Keep the syntax exact and don't write
  that literal marker in prose elsewhere in this file — the
  build-complete guard counts unchecked checkbox lines, and a stray
  mention in an explanatory comment can get miscounted as a real task.
- **Task ID**: sequential, `T001`, `T002`, ... in execution order.
- **`[P]` marker**: only if the task is parallelizable — touches
  different files and has no dependency on an incomplete task.
- **`[Story]` label**: required for user-story-phase tasks
  (`[US1]`/`[US2]`/...), absent for Setup/Foundational/Polish tasks.
- **Description**: a clear action with an exact file path — vague
  enough that "an LLM can't complete it without additional context" is a
  failing task, not a passing one.

## Phase structure

- **Setup** — project initialization, shared infrastructure.
- **Foundational** — blocking prerequisites that must complete before
  any user story starts. Keep this to what's genuinely shared; anything
  only one story needs belongs in that story's phase instead.
- **One phase per user story**, in priority order (P1, P2, P3...). Within
  a story: tests (if the project does TDD) → models → services →
  interface → integration. Map each entity/contract from the plan to the
  story that needs it; if something serves multiple stories, put it in
  Setup or the earliest story instead of duplicating it.
- **Polish** — cross-cutting concerns that don't belong to one story.

For a small bounded change with only one real user story, a single Phase
3 is enough — don't invent P2/P3 phases that don't exist just to fill out
the structure.

## Advancing out of this phase

```
mulix state set tasks_path docs/changes/<change>/tasks.md
mulix state transition tasks-complete
```
