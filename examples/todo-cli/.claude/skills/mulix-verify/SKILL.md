---
name: mulix-verify
description: Use when the active change is in the verify phase, to run the project's build/test commands and record the outcome before archiving.
---

# Verify phase

Run the project's actual build and test commands (not just the tests you
wrote during build — the whole suite). Confirm what got built actually
matches spec.md's requirements and plan.md's approach, not just that
tasks.md is fully checked — a checked box is a claim, this phase is where
you check the claim. Write the output/summary to
`docs/changes/<change>/report.md` and record the result honestly:

```
mulix state set report_path docs/changes/<change>/report.md
mulix state set verify_result pass    # or: fail
```

## If it passes

```
mulix state transition verify-pass
```

This sets `archive_confirmation=pending` — the archive phase will require
an explicit human decision before anything is archived.

## If it fails

```
mulix state transition verify-fail
```

This sends the change back to build and increments the failure counter.
Report the actual failure to the human once `verify_failures` climbs past
a few retries — don't keep silently looping without saying so.

Never set `verify_result=pass` because you expect the tests would pass, or
because you already fixed what you assume was wrong. Run them.
