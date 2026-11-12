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
the change's records in `docs/specs/<change>/`, `docs/changes/<change>/`, and the
state/design documents in `.mulix/.runtime/<change>/`. Only commit/push when
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
current verification receipt. It checks the recorded consent value, not its
authenticity. `--force --reason "explicit recovery reason"` bypasses failed
readiness only for a legal transition and records an audit entry; it creates no
test evidence or release qualification and must not conceal missing consent.
Archiving clears global active only if it
still points to that archived change. Session bindings remain and report an
archived target until explicitly selected again or unbound; they never
silently follow another global change. Apply the
[session context rules](../mulix-using/SKILL.md#session-context).
