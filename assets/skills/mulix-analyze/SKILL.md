---
name: mulix-analyze
description: Use when the active change is in the analyze phase, to check spec/plan/tasks for cross-artifact inconsistency before implementation starts.
---

# Analyze phase

Strictly read-only: compare spec.md, plan.md, and tasks.md against each
other and against the constitution, then write findings to
`docs/changes/<change>/analyze.md`. Do not modify the other artifacts from
here — if something needs fixing, report it and let the human decide
whether to go back a phase.

## What to look for

- **Duplication** — near-duplicate requirements; flag the
  lower-quality phrasing for consolidation.
- **Ambiguity** — vague adjectives (fast, scalable, secure, intuitive,
  robust) with no measurable criteria; unresolved placeholders (TODO,
  `<placeholder>`, etc.).
- **Underspecification** — requirements with a verb but no object or
  measurable outcome; tasks referencing files/components not defined in
  spec or plan.
- **Constitution alignment** — anything conflicting with a MUST
  principle in `.mulix/memory/constitution.md` (if it has real content).
  This is always the highest severity, and gets fixed by adjusting the
  spec/plan/tasks — never by diluting or reinterpreting the principle.
- **Coverage gaps** — a requirement with zero associated tasks; a task
  with no requirement or story it traces back to.
- **Inconsistency** — the same concept named differently across files;
  an entity in the plan that's absent from the spec (or vice versa); task
  ordering that contradicts stated dependencies.

## Severity

- **CRITICAL** — violates a constitution MUST, or a requirement with
  zero coverage that blocks baseline functionality.
- **HIGH** — a duplicate/conflicting requirement, an ambiguous
  security/performance attribute, an untestable acceptance criterion.
- **MEDIUM** — terminology drift, missing non-functional task coverage,
  an underspecified edge case.
- **LOW** — style/wording, minor redundancy that doesn't affect
  execution order.

## Report structure

Write a findings table (stable IDs prefixed by category initial, e.g.
`D1` for duplication, `A2` for ambiguity), a coverage summary table
(requirement key / has task? / task IDs / notes), and a metrics block
(total requirements, total tasks, coverage %, ambiguity/duplication
counts, critical count). End with a verdict: recommend resolving CRITICAL
issues before build; note that LOW/MEDIUM findings don't have to block.

If the change is small enough that this genuinely adds nothing, say so to
the human and get explicit agreement to skip, then record
`analyze_skipped=true` — don't skip silently.

## Advancing out of this phase

```
mulix state set analyze_path docs/changes/<change>/analyze.md
mulix state transition analyze-complete
```

or, only after explicit agreement to skip:

```
mulix state set analyze_skipped true
mulix state transition analyze-skipped
```
