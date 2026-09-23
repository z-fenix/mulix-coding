---
name: mulix-using-mulix
description: Use at the start of any session in a mulix-managed project, and whenever about to write or edit a file, to check which phase gates apply right now.
---

# Using mulix

mulix enforces an eight-phase workflow (specify → clarify → plan → tasks
→ analyze → build → verify → archive) with two layers of enforcement, not
just convention:

1. A **PreToolUse hook** blocks `Write`/`Edit` calls that don't match the
   current phase's whitelist. If a write gets blocked, the error message
   tells you why and what to do next — read it, don't retry the same call.
2. **Guard checks** (`mulix guard <event>`, run automatically inside
   `mulix state transition <event>`) verify concrete evidence (an artifact
   exists and is non-empty, tasks.md has no unchecked boxes, a
   verification report was actually written) before the phase advances.

## Before doing anything else

Run `mulix state show` to see the active change's current phase and the
events legal from it. Don't assume the phase from conversation history —
state lives on disk in `docs/changes/<change>/.runtime/state.yaml`, not in
your context.

## If you think even one other skill might apply

Check `mulix-specify`, `mulix-clarify`, `mulix-plan`, `mulix-tasks`,
`mulix-analyze`, `mulix-build`, `mulix-verify`, and `mulix-archive` — each
corresponds to exactly one phase and only applies while the active change
is in that phase. Invoke the one matching the current phase before taking
any action in that phase.

## Starting a new change

`mulix new "<title>"` creates the change's spec directory
(`docs/specs/<change>/`) and change directory (`docs/changes/<change>/`) plus a
fresh state in the specify phase, and makes it the active change. Do this
before anything else for a new piece of work — don't start writing a spec
by hand first.

## Red flags — stop and reconsider

- "I'll just write the code first and backfill the phase later" — this is
  exactly what the PreToolUse hook exists to prevent. If it blocks you,
  that's it working correctly, not a bug to route around.
- "The guard check is probably fine to skip with --force" — `--force`
  exists for genuine recovery situations, not for impatience. Read the
  failed check's remediation hint first.
- "I'll infer the user approved this from them not objecting" — approving
  a design and confirming archival both require *explicit* confirmation
  recorded in state, not inferred from silence.
