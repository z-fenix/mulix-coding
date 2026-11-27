---
name: mulix-archive
description: Use when the active change is in the archive phase, after verification has passed, to integrate the work with an explicit human decision.
---

# Archive phase

Run `mulix state show` and read the current verified report (`report_path`
for native changes) and latest verification receipt. If the project
constitution names an additional delivery gate, confirm its recorded
evidence is current using that project's procedure. Stop on missing evidence,
input drift or unresolved blockers; return to the phase that owns the fix.
Project-specific evidence supplements the native receipt and does not replace
the human integration decision below.

**REQUIRED SUB-SKILL:** invoke `finishing-a-development-branch` for its
branch options and integration steps. Its test gate uses the current Mulix
verification receipt; do not rerun an unchanged suite for closure. Present
its menu (merge locally / push and open a PR / keep as-is — discarding only
on an explicit request) and carry out the human's choice.

Do not author project files in this phase. Claude Code's PreToolUse
hook checks `Write`/`Edit`, not Bash, MCP, NotebookEdit or other writers;
missing paths are allowed; legacy errors fail open, while invalid session
context blocks writes (see `mulix-using`). Hosts without
the hook rely on discipline. This is not a sandbox or permission to route
around the whitelist. Authorized finishing uses git/CLI commands, which
are not gated by this hook. Finish any authorized code/revision-changing Git
operations before the final readiness check. If they stale the receipt, run
`mulix verify` again in this not-yet-archived phase before `archived`; it binds
all suites to the current source/revision/report. If that revalidation fails,
`mulix state transition verify-fail` returns to build for correction. Preserve
the change's state, spec, task, report and design paths as shown in state;
legacy artifacts remain in their original layout. Only commit/push when
explicitly authorized; a commit skipped by authorization remains skipped.
Keep the git-ignored `sdd/` workspace locally
for the change's execution record; do not clean it away. Companion sessions,
diagnostic files and ad-hoc verification output live in system scratch and
are not part of the retained record.

Only after the human has chosen — never inferred from silence — record
it:

```
mulix state set archive_confirmation confirmed
mulix state transition archived
```

The normal guard requires `archive_confirmation=confirmed` and a successful
current verification receipt. GitHub/GitLab also require the current bound
spec and every task. Recovery `--force --reason` never bypasses remote sync
or creates verification, consent, or integration evidence. Archiving only
clears global active if it still names this change; session bindings remain.

## Optional spec Issue close (GitHub/GitLab only)

After the `archived` transition actually succeeds, use the explicit change
ID, even after global active is cleared. Inspect it with `mulix state show
--change ID`; do not reselect an archived change. Close before switching the
session to work on another change.

1. Run `mulix tracker close spec --change ID --dry-run` and show the bound
   Issue and eligibility. Get separate authorization for that close.
2. Run `mulix tracker close spec --change ID --expect TOKEN` with the
   approved token. Leave the Issue open if authorization is absent.

If close fails, report it separately, leaving the local archive intact.
Retry only with a fresh reviewed dry run. Close is tracker status, not
proof of tests, review, branch integration, release, or deployment.
