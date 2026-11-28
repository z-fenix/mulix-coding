---
name: mulix-design
description: Use when the active change is in the design phase, before any plan or code exists for it.
---

# Design phase

Use this phase for design work on the selected delivery change only. For
an unrelated request or throwaway exploration, return to `mulix-using` to
route it before invoking a design skill. A spike is not a design track.

**REQUIRED SUB-SKILL:** invoke `brainstorming` now and follow it
exactly. This skill only adds what mulix needs around it: where its
output goes, which path applies, and how the phase ends.

## Inputs

Run `mulix state show` and read the clarified spec at `spec_path` and
`.mulix/memory/constitution.md` if substantive. The spec is the requirements
authority: do not re-ask answered questions or design past its Out of Scope.

Follow the project's domain-doc pointer (normally `docs/agents/domain.md`)
to the glossary and the ADRs relevant to this design. Cite each applicable
ADR by ID/path and the constraint it contributes; use the decision as
settled context, not something to restate or ask the human to approve
again. If your approach contradicts one, name it and explain why revisiting
it is justified. Draft a new decision only if a durable tradeoff needs a
record; capture the candidate in the permitted design output, not by
writing or moving ADR/glossary files during this phase.

## Paths

Classify out loud, as brainstorming says, then record it:

```
mulix state set design_track bounded        # or: architectural
```

- **bounded** — a well-scoped change to a flow that already exists in
  this repo. Clarifying questions, a short design in chat, then STOP for
  an explicit yes. No design doc.
- **architectural** — anything else (new subsystem, new project,
  interfaces others depend on). The full brainstorming process: the
  conversation, then the written design at
  `.mulix/.runtime/<change>/specs/YYYY-MM-DD-<topic>-design.md`, its
  builder check (a fresh subagent reads it as the builder and at most one
  round of questions comes back), and the human's review of the written
  file.

When in doubt, take architectural. The ratchet is one-way: hidden
complexity found mid-design upgrades bounded → architectural (record the
new track), never the reverse.

## Where things go

- Design doc: `.mulix/.runtime/<change>/specs/` (architectural only).
- Visual companion: pass the project root as `--project-dir` and carry the
  identity as described in `mulix-using`'s session context rules. The
  launcher queries `mulix context show --field scratch-dir` and puts private
  sessions in its `brainstorm/` directory. Save the returned screen/state
  paths; these are temporary materials, not archived runtime output.
- Keep agent-authored design documents and companion files in their respective
  paths. Claude Code's hook checks `Write`/`Edit`, with mode-specific error
  handling described in `mulix-using`; Bash, MCP and other writers are not
  automatically gated. Other hosts rely on discipline. Uncovered writes are
  not permission to edit the spec, tasks or source here. If the design shows
  the spec itself is wrong, stop and tell the human; changing requirements
  is their call.

## Ending the phase

Brainstorming's terminal states are replaced by this gate. After the
human's explicit approval of the stage actually presented — the in-chat
design (bounded) or the written spec file (architectural) — record it:

```
mulix state set design_path .mulix/.runtime/<change>/specs/<file>.md   # architectural only
mulix state set design_approved true
mulix state transition design-approved
```

Do not invoke writing-plans or any implementation skill from here — the
tasks phase does that. For a bounded design, carry its approach, files
touched, and testing plan forward in your reply so the tasks phase can
write them down: it's the only record of an in-chat design.

The existing gate checks track, explicit design approval and, for
architectural, a nonempty design under the change's `specs/` directory.
For GitHub/GitLab the bound spec must still be current; handle drift via
authorized `tracker preview spec` / `tracker apply spec`, not a design-phase
edit or a forced transition. Do not record approval of a different design.
