---
name: mulix-specify
description: Use when the active change is in the specify phase, to classify the work, get approval, and write the functional spec.
---

# Specify phase

Do not write spec.md, plan.md, code, or scaffold anything until you've
classified the work and, for anything beyond a spike, gotten explicit
human approval of the approach. Writing a spec for the wrong feature
wastes every phase downstream of it.

## Classify the work first, out loud

- **Spike** — a feasibility question ("can X even work?"). State the
  question and a 2-3 sentence investigation plan, get a nod, investigate
  cheaply, report a recommendation before writing spec.md.
- **Bounded** — a well-scoped change to existing behavior. Explore the
  relevant context, ask clarifying questions one at a time, present a
  short in-chat design, stop for explicit approval before writing spec.md.
- **Architectural** — a new subsystem, or a change that breaks an
  interface. Explore context, ask clarifying questions one at a time
  (purpose, constraints, success criteria), propose 2-3 approaches with
  trade-offs and a recommendation, present the design in sections asking
  approval after each. For this track only, write the agreed design to
  `docs/<date>-<topic>-design.md` before starting spec.md — it's the
  reference spec.md will point back to instead of repeating.

The ratchet is one-way: if you discover hidden complexity mid-exploration,
upgrade the track (bounded → architectural), never downgrade to skip
work. Reaching for "let's just call this bounded" to avoid the extra
approval round is the doubt itself, not a resolution of it.

Once the human has explicitly approved (a real "yes", not silence), write
`docs/specs/<change>/spec.md` from the spec template, replacing every
placeholder with concrete details while preserving section order and
headings.

## Quick guidelines

- Focus on **WHAT** users need and **WHY**. Avoid **HOW** to implement (no
  tech stack, APIs, code structure) — that belongs in the plan phase.
- Write for business stakeholders, not developers.
- Mandatory sections must be completed for every feature. Include optional
  sections only when relevant. When a section doesn't apply, remove it
  entirely — don't leave it as "N/A".
- Do not create a separate requirements-quality checklist here; mulix has
  no `/checklist` equivalent. Spec quality is judged directly against the
  bullets below before calling `spec-complete`.

## Execution flow

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

```
mulix state set spec_path docs/specs/<change>/spec.md
mulix state transition spec-complete
```

The guard checks that `spec_path` points at a real, non-empty file. It
does not check content quality — that's your job, using the guidelines
above, before calling transition, not the guard's.
