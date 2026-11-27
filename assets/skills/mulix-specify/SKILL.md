---
name: mulix-specify
description: Use when the active change is in the specify phase, to write the functional spec before any design or code.
---

# Specify phase

Run `mulix state show`. For `runtime-issues-v1`, write the draft at its
pinned `spec_path`. For legacy, use its recorded path or
`docs/specs/<change>/spec.md` if unset. Fill `.mulix/templates/spec-template.md`
(or `mulix preset resolve spec-template` if a preset overrides it) with
concrete details, preserving the section order and headings.

This phase fixes WHAT the change must do and WHY. HOW — approaches,
components, data flow — is the design phase's job (brainstorming), and
the task breakdown is the tasks phase's (writing-plans). Don't classify
the work, propose approaches, or explore designs here; if the request is
already a design, extract the requirements it implies and leave the
design itself for the design phase.

spec.md is the change's durable requirements record: it outlives the
change, and every later phase checks its output against it.

## Quick guidelines

- Focus on **WHAT** users need and **WHY**. Avoid **HOW** to implement (no
  tech stack, APIs, code structure) — that belongs in the design phase.
- Write for business stakeholders, not developers.
- Mandatory sections must be completed for every feature. Include optional
  sections only when relevant. When a section doesn't apply, remove it
  entirely — don't leave it as "N/A".
- Do not create a separate requirements-quality checklist here; mulix has
  no `/checklist` equivalent. Spec quality is judged directly against the
  bullets below before calling `spec-complete`.

## Execution flow

Read the actual `.mulix/memory/constitution.md`, not its template. Fill
Acceptance Baseline & Scope and Acceptance Mapping: anchor each requirement
to an observable AC, the applicable source versions, approved exclusions and
unresolved gates. Use a separate acceptance source only when the project
explicitly defines it; preserve its original IDs without assuming approval.
Keep the native `mulix-acceptance` criteria regardless of that source.

1. Parse the feature description. If it's empty, stop and ask for one —
   don't invent a feature out of nothing.
2. Extract key concepts: actors, actions, data, constraints.
3. For unclear aspects:
   - Make an informed guess based on context and industry standards first.
   - Only mark inline with `NEEDS CLARIFICATION: <specific question>` when
     the choice significantly impacts scope or user experience, multiple
     reasonable interpretations exist with different implications, or no
     reasonable default exists.
   - Prioritize by impact: scope > security/privacy > user experience >
     technical detail. Guessing wrong here is more expensive to unwind
     than asking later in the clarify phase — but marking everything
     `NEEDS CLARIFICATION` just to avoid a judgment call defeats the
     point of this phase.
4. Fill User Scenarios & Testing. If no clear user flow can be determined,
   stop — you can't spec a feature you can't describe a user using.
5. Generate Functional Requirements. Each one must be testable. Use
   reasonable defaults for unspecified details and record them in
   Assumptions instead of leaving a gap.
6. Define Success Criteria: measurable, technology-agnostic outcomes.
   Include both quantitative metrics (time, performance, volume) and
   qualitative measures (satisfaction, completion rate). Each criterion
   must be verifiable without knowing the implementation.
7. Identify Key Entities, if the feature involves data.

### Success criteria guidelines

Good: "Users can complete checkout in under 3 minutes", "95% of searches
return results in under 1 second" — measurable, technology-agnostic,
user-focused, verifiable without implementation details.

Bad: "API response time is under 200ms" (too technical — say "users see
results instantly" instead), "Redis cache hit rate above 80%"
(technology-specific). If a criterion names a framework, database, or
tool, rewrite it from the user's point of view.

### Reasonable defaults (don't ask about these)

Data retention, performance targets, error handling, and integration
pattern all have industry-standard defaults for the domain — use them and
note the assumption rather than raising a clarification question.

## Advancing out of this phase

For legacy only, record the actual path using `mulix state set spec_path
<path>`. The native path is already set.

For a native GitHub/GitLab change, publish the spec through this one route:

1. Run `mulix tracker publish spec --dry-run`. Show the human the exact
   target and content and obtain authorization to publish them.
2. Run `mulix tracker publish spec --expect TOKEN` with that token. If an
   earlier publication is pending, use a fresh dry run and authorization
   for reconciliation; do not create another Issue by hand.

If the spec is already bound, preserve its snapshot and check sync instead
of republishing. Local Markdown and legacy changes skip publication. Then:

```sh
mulix guard spec-complete
mulix state transition spec-complete
```

The guard checks a real spec and exactly one `mulix-acceptance` YAML fence
from the template: unique AC identifiers, a concrete expected result, and
coverage of every FR identifier. Define observable expectations independently
of future implementation. Semantic completeness and product value still need
review; structure alone does not establish them.
