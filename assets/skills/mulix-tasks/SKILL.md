---
name: mulix-tasks
description: Use when the active change is in the tasks phase, after its design was approved and before any code is written.
---

# Tasks phase

**REQUIRED SUB-SKILL:** invoke `writing-plans` now and follow it
exactly. Its output is this change's `tasks.md`. This skill only adds
what mulix needs around it.

## Inputs

- `docs/specs/<change>/spec.md` — the requirements authority.
- The approved design: `design_path` from `mulix state show`
  (architectural), or the in-chat design approved in the design phase
  (bounded).
- `.mulix/memory/constitution.md`, if it has real content — every task
  must comply with its MUST principles; an unjustified violation is a
  plan defect, fixed here, not in build.

## Output: `docs/changes/<change>/tasks.md`

Write it exactly as writing-plans specifies — plan header, Global
Constraints, Review Focus, then one section per task — with these
mulix specifics:

- **Header:** `**Spec:**` names `docs/specs/<change>/spec.md`, and
  `**Design:**` the design doc (or "approved in chat" for bounded, with the
  design's approach summarized under Architecture). Keep the "For agentic
  workers" line; the build phase decides which executor runs it.
- **Task headings:** `## Task N: <name>`, N = 1, 2, 3... in execution
  order, no gaps, each exactly once. The build executors extract briefs by
  this heading, and the guards match ledger lines and reports to it.
- **Steps:** code tasks use RED → verify RED → GREEN → verify GREEN with
  `mulix test run --task N --stage red|green`, and commit only if authorized.
  Verification/documentation tasks declare a concrete reason and run a real
  `mulix test run --task N --stage check` instead of invented RED/GREEN.
  Include complete test/code blocks and `Expected:` lines. Start every step
  as `- [ ] **Step N: ...**`; mark `[X]` only after actual verified execution.
- **Acceptance mapping:** use the `mulix-tests` fence in the tasks template.
  Map every AC and task to concrete TC cases with point, Given/When/Then,
  suite, exact emitted test name and project-relative test source file.
  Derive assertions from the spec; review normal, boundary, error, permission
  and integration cases. Declare each task's kind and non-code reason.
- **Mandatory suites:** read `.mulix/templates/quality-template.md`, inspect
  the project's actual test commands and configure `.mulix/quality.yaml`.
  All declared suites run at final verification, including nested modules and
  required integration checks; configuration remains project-owned.
- `.mulix/templates/tasks-template.md` shows the resulting layout. If a
  preset overrides it (`mulix preset resolve tasks-template`), follow the
  override's structure — but the `## Task N` headings are not optional.

A small bounded change is still a plan: one or two tasks is fine; the
headings and steps are not.

## Review before the gate

1. Run writing-plans' Self-Review and fix what it finds inline.
2. Link tasks.md for the human and ask whether it captures what they
   want. Wait for their answer; revise on feedback.

There is no execution handoff here: don't ask how to execute and don't
invoke an executor. The build phase asks.

## Advancing out of this phase

```
mulix state set tasks_path docs/changes/<change>/tasks.md
mulix state transition tasks-complete
```

The guard checks tasks.md exists inside `docs/changes/<change>/`, is
non-empty, has `## Task N` sections numbered 1..n, and a complete acceptance
matrix referring to configured suites. Missing mappings and policy fail closed.
