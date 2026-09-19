---
name: mulix-plan
description: Use when the active change is in the plan phase, to turn a clarified spec into a concrete implementation approach.
---

# Plan phase

Write `docs/changes/<change>/plan.md` from the plan template: Technical Context,
approach, design notes, risks, and a test strategy. Keep it proportional
to how the change was classified during specify — a spike's plan is a
few sentences, a bounded change's plan is a short paragraph, an
architectural change's plan can point back at the design doc written
during specify rather than repeating it.

## Fill Technical Context

Language/version, primary dependencies, storage, testing approach, target
platform, and scale/scope. Mark anything genuinely unknown as `NEEDS
CLARIFICATION` rather than guessing silently — unlike the spec phase,
here an unresolved unknown blocks the plan from being useful, so resolve
it yourself (see below) rather than leaving it for a human.

## Resolve unknowns before writing the rest

For each `NEEDS CLARIFICATION` in Technical Context, each dependency, and
each integration point, work out the answer before continuing — check
existing code/config in this repo first, then reasonable defaults for the
stack. Record what was decided and why directly in the plan's design
notes; don't leave a marker in the final plan.md the way spec.md
tolerates NEEDS CLARIFICATION for the human to resolve later.

## Design

- Extract the entities the feature needs (fields, relationships,
  validation rules, state transitions if any) into the plan's design
  notes — skip this if the change has no new data shape.
- If the change exposes an interface (a CLI command, a public function, an
  API endpoint) document its contract here. Skip if the change is purely
  internal.
- The test strategy section matters most: it's what the build phase's TDD
  discipline will actually follow, so be concrete about what gets tested
  and in what order, not just "add tests."

## Constitution Check

If `.mulix/memory/constitution.md` has real content (not just
placeholders), check the plan against it and note any violation plus its
justification. An unjustified violation blocks this phase — resolve it by
changing the plan, not by ignoring the principle.

## Advancing out of this phase

```
mulix state set plan_path docs/changes/<change>/plan.md
mulix state transition plan-complete
```
