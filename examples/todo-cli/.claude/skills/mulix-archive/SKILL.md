---
name: mulix-archive
description: Use when the active change is in the archive phase, after verification has passed, to close out the change with an explicit human decision.
---

# Archive phase

The PreToolUse hook blocks all writes in this phase — there is nothing
left to produce, only a decision to make.

Present the human with an explicit choice; never assume one from silence:

- **Merge** — merge the branch now.
- **Open a PR** — open a pull request instead of merging directly.
- **Keep** — leave the branch as-is, unmerged, for later.
- **Discard** — abandon the branch/change entirely.

Only after they choose, record it:

```
mulix state set archive_confirmation confirmed
mulix state transition archived
```

The guard requires `archive_confirmation=confirmed`; there is no path that
reaches `archived` without it.
