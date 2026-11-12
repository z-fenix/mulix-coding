# P0: verifiable acceptance

## Intent and authorization

Implement the P0 scope approved in conversation: acceptance traceability, a real test runner, source-bound evidence and fail-closed delivery guards. Keep the existing seven phases. Do not implement CI, new platform support, installation/release qualification or billed model evaluations (P1/P2). Do not commit or push.

## Builder decisions

- Reuse spec.md, tasks.md, task-N-report.md, report.md and CLI-owned state.yaml. Do not create another retained runtime directory.
- spec.md contains exactly one `mulix-acceptance` YAML fence: `criteria`, each with `id`, `requirement` (FR-NNN), and independently observable `expected` text. Every FR identifier in the spec needs a criterion.
- tasks.md contains exactly one `mulix-tests` YAML fence: `tasks` entries (`number`, `kind`: code|verification|documentation, `reason` for non-code), and `cases` entries (`id`, `criterion`, `task`, `point`, `given`, `when`, `then`, `suite`, `test`, `file`). Every criterion and task needs a case. The declared tasks must exactly match numbered Task headings. Step checkbox-only edits do not change plan identity.
- .mulix/quality.yaml is project-owned configuration, schema mulix.quality.v1. Suites declare unique `id`, argv `command`, project-relative `cwd`, `format` (go-json|json) and a bounded timeout in seconds. Every declared suite is mandatory; no implicit skip or exit-code-only test adapter. Generic JSON lines are objects with `name` and `status` (pass|fail|skip). Go identities are `package#TestName`.
- `mulix test run --task N --stage red|green|check` runs suites relevant to that task in build. Code tasks require an actually failing mapped test followed by passing mapped tests with unchanged test-file/contract fingerprints. Non-code tasks require a reason and passing check evidence instead of fake RED/GREEN.
- `mulix verify` in verify runs all configured suites, checks all mapped cases, a structured `mulix-review` YAML fence in report.md (`reviewer`, `result: approved`, `blocking: []`), and writes a summary plus evidence into report.md. It records pass/fail itself. It may also revalidate in not-yet-archived archive phase after finishing changes; failed revalidation permits the corrective `verify-fail` return to build. Already archived changes cannot reopen through that event. This retains seven phases and adds no acceptance bypass. Review identity is an explicit local claim, not authenticated approval.
- Receipts are CLI-generated records in state.yaml, not settable with `state set`. Reports retain readable run summaries. Guards consume receipts, not arbitrary RED/GREEN text. Final evidence binds project location, change, spec, normalized plan, policy, source files, test files and final report bytes. Source/report changes invalidate verify and archive. A corrective verify-fail or new build-complete cycle invalidates prior final verification, requiring a fresh run even with unchanged source. Task evidence is historical TDD provenance; later unrelated task implementation does not require redoing earlier RED runs.
- Fingerprint tracked and non-ignored untracked files in Git projects, including dirty contents and deletions. Non-Git projects use deterministic filesystem enumeration with only explicitly identified tooling/runtime exclusions. Exclude state, context/locks, installed baselines, the current final report and transient data, not arbitrary source paths. Source symlink inputs fail with an actionable error rather than silently ignoring target changes.
- Execute argv without a shell; capture stdout/stderr separately in context scratch; retain counts, exit codes, identities and summaries in existing artifacts. Fail on malformed output, no tests, all skipped, failures, missing mapped tests, cancellation, timeout, changed inputs during execution or unusable artifacts. State locks are not held while test commands execute. Revalidate pinned change/phase/input identity under the mutation lock before saving a receipt.
- Recovery `--force` records the bypass and reason. It never creates successful test evidence; a normal archive still requires fresh verification. Direct filesystem edits and force remain explicit local recovery capabilities, not a security sandbox.
- Existing v2 state remains readable; new guards apply when continuing old work. No opt-out silently retains text-only acceptance. Templates, smoke fixtures, embedded skills and owned overrides must agree.

## Acceptance

1. Arbitrary RED/GREEN and manual verify_result=pass cannot satisfy build/verify/archive guards.
2. Exit nonzero plus PASS text, no tests, all skipped, absent mapped test, malformed output and mismatched change/project are rejected.
3. Real red->green task evidence and full verification on a current fingerprint pass.
4. Source, test, spec, plan-definition, quality policy or final report edits stale final evidence; permitted checkbox edits do not.
5. Each mandatory requirement is traceable to a concrete passing case; duplicate/orphan IDs and broken references fail.
6. Docs/verification tasks use honest checks rather than invented RED runs.
7. Important review blockers prevent final verification; review authenticity limits are documented.
8. Real CLI tests cover session selection, command failure, mutation races, report persistence and force auditing.
9. Root/example full tests, vet, race and all three smoke scripts are run. Skips/environment failures are disclosed.
