---
name: mulix-clarify
description: Use when the active change is in the clarify phase, to resolve ambiguity in the spec before design begins.
---

# Clarify phase

Run `mulix state show` and read the spec at `spec_path`. The output is a
spec with the answers incorporated before design, or an explicit skip.
For a GitHub/GitLab change, start with `mulix tracker preview spec` to read
and identify the authoritative Issue body. Use the local snapshot as
read-only; resolve any existing drift with the human before advancing.

## Scan for ambiguity first

Read the spec and classify each of these categories as Clear / Partial /
Missing (keep this scan internal, don't dump it unless nothing needs
asking):

- **Functional scope & behavior** — core user goals, explicit
  out-of-scope declarations, actor/persona differences
- **Domain & data model** — entities, attributes, relationships,
  identity/uniqueness, lifecycle/state transitions, scale assumptions
- **Interaction & UX flow** — critical journeys, error/empty/loading
  states, accessibility/localization
- **Non-functional quality** — performance, scalability, reliability,
  observability, security/privacy, compliance
- **Integration & external dependencies** — external services/APIs and
  their failure modes, data formats, versioning
- **Edge cases & failure handling** — negative scenarios, rate limiting,
  concurrent-edit conflicts
- **Constraints & tradeoffs** — technical constraints, explicit
  tradeoffs or rejected alternatives
- **Terminology** — consistent canonical terms, no drift
- **Completion signals** — testable acceptance criteria
- **Placeholders** — leftover TODOs, vague adjectives ("robust",
  "intuitive") lacking quantification

For each Partial/Missing category, it's a candidate question — unless
the answer wouldn't materially change implementation, or it's really a
design/task-breakdown question that belongs in design/tasks instead.

## Ask up to 5, one at a time

Build a prioritized queue (max 5 total) of the `NEEDS CLARIFICATION`
markers and other real ambiguity found above, ranked by how much
downstream rework a wrong guess would cause. Then:

- Present **exactly one question at a time**, never a batch dump.
- Each question must be answerable with a short multiple-choice (2-5
  mutually exclusive options) or a short-phrase answer (<=5 words).
- State your recommended option and why, so the human can just say "yes"
  if they agree.
- For each accepted answer, add `- Q: <question> → A: <answer>` under
  `## Clarifications` / `### Session YYYY-MM-DD`, and update the affected
  requirement, criterion or scenario. Use the draft body for a remote change
  and edit the spec directly only for local Markdown or legacy.
- Stop when all critical ambiguity is resolved, the human says "done" /
  "good" / "no more", or you hit 5 questions — whichever comes first.

If there is genuinely nothing worth asking (a rare, small bounded
change), say so to the human and get their explicit agreement to skip,
then record `clarify_skipped=true` — don't skip silently.

## Confirm remote clarifications (GitHub/GitLab only)

Present the exact revised body and the bound Issue's verified host, project
and number. Get explicit authorization to edit that Issue's body using
its provider; keep the Mulix marker intact. Answers to questions are not
authorization to send a remote edit. Do not upload a replacement Issue.

After the authorized remote edit, run `mulix tracker preview spec` and
have the human confirm the exact snapshot/token, then run
`mulix tracker apply spec --expect TOKEN`. For a conflicting local edit,
review a new `preview spec --backup-local` and use that token with
`apply spec --backup-local --expect TOKEN`. This is the local sync path,
not permission to edit the snapshot with another tool. Stop on stale
approval or an unreachable/mismatched target.

## Advancing out of this phase

The recorded spec must contain `## Clarifications`, or an explicit skip
must be recorded. Both remote exits require the verified current spec
snapshot. Run the matching `mulix guard` first, then:

```sh
mulix state transition clarify-complete
```

or, only after explicit agreement to skip:

```sh
mulix state set clarify_skipped true
mulix guard clarify-skipped
mulix state transition clarify-skipped
```
