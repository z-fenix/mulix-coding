# P0 Verifiable Acceptance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The user has requested implementation and full tests; execute the approved P0 scope in this session without commits or pushes.

**Goal:** Replace text-only delivery claims with traceable, actually executed, source-bound test evidence.

**Architecture:** Add a host-independent quality package for contracts, fingerprints, process execution and receipt validation. Store compact receipts in existing CLI-owned state and readable summaries in existing reports; connect new checks to the phase table and expose session-aware test/verify commands.

**Tech Stack:** Go 1.27, Cobra, yaml.v3, standard library subprocess/JSON/SHA-256.

**Spec:** docs/superpowers/specs/2026-11-12-verifiable-acceptance-design.md

## Global Constraints

- Keep the seven phases and runtime specs/sdd layout; no new retained evidence bundle.
- All new checks fail closed; historical state remains readable, not silently exempted.
- Work on feature branch, keep user changes intact, no commits/pushes.
- No test command executes while holding the state lock; revalidate before persisting.
- No CI, platform qualification, real model runs or release publication in P0.

## Review Focus

- Fake success, no tests, skipped/missing cases and malformed event streams must not produce pass receipts (Task 2).
- Dirty/untracked/deleted source, policy/spec/test edits and report replacement must stale evidence; checkbox-only progress must not (Tasks 1, 3).
- Task evidence and final evidence must not be reused across changes/projects or phases (Tasks 2, 3).
- Command timeout/cancellation, input mutation and persistence failure must not leave a successful state (Tasks 2, 3).
- Force recovery and existing archived history must remain explicit, without a normal delivery bypass (Tasks 3, 4).

## Task 1: Contracts and deterministic fingerprints

**Files:** Create internal/quality/{contract,fingerprint}.go and matching tests; reuse the task-heading parser without divergent semantics.

**Interfaces:** Produce Project, Contract, Policy, Task, Case, Snapshot; LoadContract(Project), LoadPolicy(root), Fingerprint(Project, Contract, task), ValidateSpec(Project).

- [X] **Step 1: Write tests** for missing/duplicate fences and IDs, orphan requirements/cases, mismatched task headings, absent mapped files, invalid suite cwd/argv/format, Git dirty/untracked/deleted content, cross-project identity and normalized checkbox status.
- [X] **Step 2: Verify RED** with `go test ./internal/quality -run 'Contract|Policy|Fingerprint' -count=1`.
- [X] **Step 3: Implement** strict YAML blocks, traceability and deterministic snapshots with explicit exclusions and safe paths.
- [X] **Step 4: Verify GREEN** using the same command.

## Task 2: Real execution and task receipts

**Files:** Create internal/quality/{runner,evidence}.go and tests; extend internal/flow/state.go for CLI-owned receipts.

**Interfaces:** Consume Task 1; produce Run(ctx, Project, Contract, Policy, task, stage, scratch), RunRecord, ValidateTaskEvidence and ValidateVerification. Receipts include snapshot, times, exit/count/result and exact test identities; no import cycle between flow and quality.

- [X] **Step 1: Write process-backed tests** for actual failing/passing tests, failure despite PASS, zero/all-skipped/missing/malformed tests, red-before-green, changed tests, timeout/cancellation and command-side source mutation.
- [X] **Step 2: Verify RED** with `go test ./internal/quality -run 'Run|Evidence' -count=1`.
- [X] **Step 3: Implement** argv execution, bounded parsers/output, fail-closed outcomes and source-bound evidence validation.
- [X] **Step 4: Verify GREEN** with the focused command and then all quality tests.

## Task 3: Session-aware commands and phase gates

**Files:** Create internal/cliutil/quality_cmd.go and tests; modify root.go, state_cmd.go, internal/guard/guard.go, internal/flow/flow.go and tests.

**Interfaces:** Consume quality APIs; expose `mulix test run --task N --stage red|green|check` and `mulix verify`; state mutation pins the original selection/phase and saves failures honestly. Existing state set cannot set receipts. Force records audit metadata.

- [X] **Step 1: Write CLI/guard regressions** demonstrating arbitrary RED/GREEN and manual pass no longer advance; real task/full evidence succeeds; stale/report-mismatched evidence, review blockers and cross-change evidence fail; force cannot fabricate a normal archive pass.
- [X] **Step 2: Verify RED** with focused CLI/guard tests.
- [X] **Step 3: Implement** commands, report summaries, receipt persistence, registry checks and recovery audit.
- [X] **Step 4: Verify GREEN** across quality, flow, state, guard, CLI and hook tests; preserve preexisting behavior not superseded by P0.

## Task 4: Installed workflow, smoke fixtures and end-to-end verification

**Files:** Update spec/tasks templates, mulix phase skills, relevant owned execution overrides plus embedded copies, README and scripts/smoke-*.sh. Add current-version process-level P0 acceptance tests. Fix smoke-update interpreter discovery without changing user environments.

**Interfaces:** Installed docs configure quality.yaml and real event-producing suites; smoke uses a real failing/passing fixture rather than fabricated results. Existing template installation/update mechanisms manage template resources only; quality.yaml remains project-owned.

- [X] **Step 1: Write integration regressions** for installed workflow and isolated binary-driven real RED/GREEN/verify/archive, including stale input and missing evidence negatives.
- [X] **Step 2: Verify RED** with focused integration tests.
- [X] **Step 3: Update** templates, skills, scripts and docs to match the actual commands and limits. Preserve archived examples unchanged.
- [X] **Step 4: Verify GREEN** with full root/example tests, vet, race and all three smoke scripts against one current binary.
- [X] **Step 5: Review** the final branch using one fresh read-only reviewer; address verified blocking findings and rerun covering/full tests.

## Verification ledger

Baseline: root `go test -count=1 ./...` exited 0 before implementation. All four tasks and final review/fix steps are complete; changes remain uncommitted on `feat/p0-verifiable-acceptance`, HEAD `2ff5d6c`. No commit steps are authorized.

Final verification on 2026-11-12 (Linux amd64, Go 1.27.3), after all five independently reviewed Important defects were reproduced and fixed:

| Verification | Result |
|---|---|
| Root `go test -json -count=1 -coverprofile=... ./...` | 723 passed, 6 skipped, 0 failed; coverage 81.0% |
| Example `go -C examples/todo-cli test -json -count=1 -coverpkg=./... -coverprofile=... ./...` | 18 passed, 0 skipped, 0 failed; coverage 72.3% |
| Root `go test -race -json -count=1 ./...` | 723 passed, 6 skipped, 0 failed |
| Example `go -C examples/todo-cli test -race -json -count=1 ./...` | 18 passed, 0 skipped, 0 failed |
| Root/example `go vet ./...` | Both exited 0, empty output |
| `go build -o .../mulix-final ./cmd/mulix` | Exited 0 |
| e2e, sessions and update smoke scripts | All exited 0 against that same freshly built binary |
| `bash -n` on all three smoke scripts | Exited 0 |

Counts include subtests. Six root skips are five native-Windows tests on Linux and the optional vendor regeneration test requiring `MULIX_SUPERPOWERS_SOURCE`; they are not passes. The five reviewed defects cover late cancellation, repeated unfinished Go events, final-evidence lifecycle, output capture overflow and TODO-domain placeholder validation. Each has an observed RED→GREEN regression; no second independent approval is claimed.

Detailed ledger, review dispositions, exact counts and full logs are retained in `.superpowers/sdd/2026-11-12-verifiable-acceptance/` (`progress.md`, `final-review.md`, `verification-summary.json`, `review-*-red.log`, `review-*-green.log`, and `*-final` outputs). Preserve this uncommitted evidence; no merge, push, release or workspace deletion was performed. CI/native-platform/host/model qualification remains P1/P2, outside this implementation.
