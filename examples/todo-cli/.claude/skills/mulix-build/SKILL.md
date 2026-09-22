---
name: mulix-build
description: Use when the active change is in the build phase, before writing any implementation code, for every task in tasks.md.
---

# Build phase

This phase is unrestricted by the PreToolUse hook (source and test files
live outside the change directory), but that only removes the *outer*
gate. The discipline below is the real gate, and it's enforced by you
checking your own work, task by task.

This phase merges two things: task-execution order and progress tracking
against tasks.md, and a strict test-driven discipline for how each task's
code gets written. Neither replaces the other — task-execution order
without TDD discipline produces code nobody trusts; TDD discipline
without task-execution order produces code that ignores the plan.

## Part 1 — Executing tasks.md

### Before writing any code

Read tasks.md for the full task list, plan.md for tech stack and file
structure, and spec.md for what each task is actually supposed to
achieve — don't start from tasks.md alone. Execute phase by phase in the
order tasks.md lays out: complete each phase before moving to the next,
run sequential tasks in order, and only run `[P]`-marked tasks together
when they touch different files.

### Classify each task before starting it

Not every task deserves the same amount of up-front thought:

- **Spike-sized** — the task's approach is obvious from tasks.md and
  plan.md. Go straight to Part 2's red step.
- **Bounded** — there's more than one reasonable way to implement this
  task, or it touches code you haven't read yet. Skim the relevant
  existing code first and settle on an approach in a sentence or two
  before writing the first test.
- **Architectural** — the task implies a design decision plan.md didn't
  already make (a new interface shape, a data model choice with real
  trade-offs). Stop and work out the approach explicitly — state the
  options and which one you're taking and why — before touching any
  code. If the decision is big enough to affect other tasks, surface it
  to the human rather than deciding alone.

The ratchet is one-way: if a task you assumed was bounded turns out to
hide a real design decision, upgrade it to architectural before
proceeding, never downgrade to avoid the extra thought.

### Progress and failure handling

Report progress after each completed task, not just at the end of a
phase. If a non-parallel task fails, halt and report it with enough
context to debug — don't push forward into the next task assuming it'll
sort itself out. For `[P]` tasks, keep going on the ones that succeed and
report the ones that failed; don't let one parallel failure block
unrelated work.

## Part 2 — Writing each task's code

### Load the TDD skill before writing anything

Before the first task's red step, invoke the
`superpowers:test-driven-development` skill and follow it exactly for
every task in this phase. That skill is the authoritative statement of
the discipline — where anything in this section disagrees with it, the
skill wins. If the phase spans a long session and your context has been
compacted, re-invoke it before continuing.

If that skill is not installed in this environment, hold yourself to the
inline version below instead — it is a faithful fallback, not an excuse
to skip TDD.

### The iron law (fallback summary)

**No production code without a failing test first.** If you catch
yourself having written implementation code before a test exists for it,
delete it and start over — don't keep it "as reference." Code written to
satisfy a test you haven't seen fail proves nothing about whether the
test would have caught the bug it's meant to catch.

### Red-green-refactor, per task

1. **Red** — write one minimal test for one behavior from the current
   task. Real code, not mocks, unless a mock is truly unavoidable.
2. **Verify red** — actually run it. Confirm it fails for the *right*
   reason (the feature is missing), not because of a typo. If it passes
   immediately, the test is wrong — fix the test, not the assumption that
   it's fine.
3. **Green** — write the simplest code that passes. Resist adding
   anything the test doesn't require yet; that's premature and belongs in
   a later task's red step, not this one.
4. **Verify green** — run it. Confirm it passes, confirm nothing else
   regressed, confirm the output is clean (no stray warnings).
5. **Refactor** — clean up now that it's green: dedupe, rename, extract.
   Don't add behavior here.
6. Move to the next behavior / next task.

### Before marking a task done, check all of:

This list mirrors the verification checklist in
`superpowers:test-driven-development` — run it after the skill's own
checklist, not instead of it.

- [ ] Every new function/behavior has a test
- [ ] You watched each test fail before making it pass
- [ ] Each failure was for the right reason, not a typo or setup bug
- [ ] The implementation is the minimal code that passes, nothing more
- [ ] All tests pass, including previously-passing ones
- [ ] Test output is clean (no warnings, no skipped assertions)
- [ ] Tests exercise real behavior, not mocked-out behavior standing in for it
- [ ] The task's checkbox in tasks.md is checked *and* an evidence line
      `- tests: <test reference>` was added directly beneath it — or the
      task line was marked `[no-test]` if it legitimately has no test
      (docs, config, scaffolding)

Can't check all of these? You skipped TDD for that task. Go back and do
it properly rather than checking the box anyway — the verify phase's
guard doesn't inspect *how* tests were written, only that tasks.md is
fully checked, so this discipline is on you.

### Bugs found mid-build

Write a reproducing failing test before touching the fix. Never patch
behavior you can't first demonstrate is wrong.

## Delegating exploration

If a task requires searching or reading through a lot of unfamiliar code
before you can write the first test, delegate that exploration to a
subagent and have it report back only concrete `path:line` citations, not
full file dumps — keep your own context focused on the task's tests and
implementation, not on search noise.

## Advancing out of this phase

Only once every checkbox in tasks.md is checked:

```
mulix state transition build-complete
```

The guard counts unchecked boxes in tasks.md *and* checks the TDD
evidence trail: every checked task needs a `- tests: <test reference>`
line directly beneath it, or a `[no-test]` marker on the task line. The
guard verifies the evidence trail was left, not that the red-green cycle
was honestly run — that part is still on you via the TDD skill and the
checklist above. Evidence added without a test that actually failed
first defeats the whole point; don't fabricate it.
