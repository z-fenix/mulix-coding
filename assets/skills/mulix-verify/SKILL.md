---
name: mulix-verify
description: Use when the active change is in the verify phase, or archive evidence is stale before archiving.
---

# Verify phase

**REQUIRED SUB-SKILL:** `verification-before-completion` governs claims.

Run `mulix state show`. Read `spec_path`, the approved task inputs, and the
project constitution. For `runtime-issues-v1` use the pinned `report_path`
and the owning `sdd/issues/` ledger. For legacy use the plan-file workspace
and `docs/changes/<change>/report.md` (the runner records its path). Fix the
implementation, requirements, tests and environment being verified. When
the constitution names an additional gate, run its specified procedure and
record input identities and actual results in the report; unresolved required
gates belong in `mulix-review.blocking`.

1. Review each AC/TC against the spec's Acceptance Baseline & Scope and the
   approved design, including boundaries, errors and integration behavior.
   Read the Build ledger's rulings and deferred findings; record whether each
   ruling holds against the spec in the final report.
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

   The CLI executes all configured suites against the current inputs, checks
   exact mapped cases, records pass/fail, and appends summaries to the existing
   report. Karate runs all task entries and final-only entries, even when the
   task checks already passed. Missing, all-skipped or malformed results fail
   closed. Review unmapped skips and undeclared/manual checks explicitly; a
   skip does not establish acceptance.
4. On success, run `mulix guard verify-pass` and `mulix state transition
   verify-pass` with the current receipt and unchanged report. GitHub/GitLab
   also require current bound spec and task bodies. A sync failure is not
   a test result; resolve drift using confirmed sync, then reassess evidence.
   Successful transition enters archive with `archive_confirmation=pending`.
   If verification fails, explain the failing behavior and its task, then
   `mulix state transition verify-fail` for local recovery in Build. Keep
   ledger history; report repeated failures instead of silently looping.

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
