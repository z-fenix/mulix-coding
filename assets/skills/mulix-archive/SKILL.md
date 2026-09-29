---
name: mulix-archive
description: Use when the active change is in the archive phase, after verification has passed, to integrate the work with an explicit human decision.
---

# Archive phase

**REQUIRED SUB-SKILL:** invoke `finishing-a-development-branch` and
follow it: verify the suite is green, detect the environment, present
its menu exactly as written (merge locally / push and open a PR / keep
as-is — discarding only on an explicit request), and carry out the
human's choice.

The PreToolUse hook blocks Write/Edit in this phase; the finishing
skill works through git commands, which are not gated. Commit the
change's record before integrating: `docs/specs/<change>/`,
`docs/changes/<change>/`, and `.mulix/.runtime/<change>/` (its state,
design doc, and execution workspace — the visual-companion sessions are
git-ignored).

Only after the human has chosen — never inferred from silence — record
it:

```
mulix state set archive_confirmation confirmed
mulix state transition archived
```

The guard requires `archive_confirmation=confirmed`; there is no path
that reaches `archived` without it. Archiving clears the active-change
marker.
