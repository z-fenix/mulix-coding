---
name: mulix-verify
description: Use when the active change is in the verify phase, or archive evidence is stale before archiving.
---

# Verify phase

**REQUIRED SUB-SKILL:** `verification-before-completion` governs claims.

1. Review each AC/TC against the spec and approved design, including boundaries,
   errors and integration behavior. Read the build's rulings and deferred minors
   in `.mulix/.runtime/<change>/sdd/tasks/progress.md`; record whether each ruling
   holds against the spec in `docs/changes/<change>/report.md`.
2. Obtain a whole-change review and address blocking findings. Retain the actual
   review reference and outcome in the report using exactly one fence:

   ```mulix-review
   reviewer: actual reviewer identity or review session reference
   result: approved
   blocking: []
   ```

   This is an explicit local claim, not authenticated reviewer consent. Only
   record approved/no blockers when that review actually happened.
3. Inspect `.mulix/quality.yaml` against the project's real test surface. Every
   required unit, integration and final-artifact suite must be declared. Read
   `.mulix/templates/quality-template.md` for adapters and limitations. Run:

   ```sh
   mulix verify
   ```

   The CLI executes all configured suites, checks exact mapped cases, records
   pass/fail itself, and appends summaries to the existing report. Missing,
   all-skipped or malformed results fail closed. Review any unmapped skips and
   undeclared/manual checks explicitly; do not represent a skip as acceptance.
4. On success, `mulix state transition verify-pass`. This enters archive with
   `archive_confirmation=pending`; integration still needs an explicit human
   decision. On failure, explain which tasks own it, then
   `mulix state transition verify-fail` to return to build. Fix with real task
   execution evidence; append rather than erase ledger history. Report repeated
   failures instead of silently looping.

Manual `verify_result=pass`, old logs and handwritten RED/GREEN are not test
receipts. A failed/incomplete retry invalidates prior success. Source, tests,
policy, spec, plan definitions, Git revision or final-report edits stale the
final evidence. Verified step-checkbox status changes are normalized.

## Temporary output and revalidation

The runner keeps bounded stdout/stderr in the current context's system scratch
`verification/` directory (`mulix context show --field scratch-dir`) and retains
stream hashes, counts and test identities in state.yaml. Keep relevant behavior
and requirement checks in the final report. Existing SDD records stay read-only
in verify; see the [runtime artifact policy](../mulix-using/SKILL.md#runtime-artifact-policy).

If finishing changes inputs after verify-pass, `mulix verify` can revalidate in
not-yet-archived archive phase, without changing phases or bypassing readiness.
Correct a failing implementation in build using `mulix state transition
verify-fail` after failed archive revalidation, not through archive-phase writers.
An already archived change cannot run tests or mutate evidence through this
command. Recovery `--force --reason` logs a bypass, not release qualification.
