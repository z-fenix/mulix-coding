---
name: mulix-build
description: Use when the active change is in the build phase, before writing any production code or test for it.
---

# Build phase

Read `mulix state show`: `spec_path` and, for `runtime-issues-v1`, the
numbered files in `tasks_dir` are the approved inputs. A legacy change uses
its single `tasks_path` plan. Read the spec and plan before implementation.

## Choose the executor

Recommend a method based on dependencies, task count and the cost of a
mistake. Get the human's choice, then record `mulix state set execution_method
subagent-driven` or `inline`. On a return from Verify, keep the recorded
method unless the human asks to change it.

- **subagent-driven:** `subagent-driven-development` implements and reviews
  each task before a whole-change review.
- **inline:** `executing-plans` implements tasks in this session, followed
  by a fresh whole-change review.

**REQUIRED SUB-SKILL:** invoke the chosen executor and use
`test-driven-development` before the first code task. For reasoned non-code
tasks use CHECK as defined by their matrix. Use a worktree only with the
human's permission; commands and hook context must refer to the same checkout.

## Execute and record

1. Use `sdd-workspace` with the selected plan path from state, from the
   project root with the current session identity. Native work uses the
   owning `sdd/issues/` workspace; legacy uses its plan-file workspace.
   Keep its `plan-path`, `progress.md`, briefs, reports and review packages
   for recovery and guards. Use `progress-init` and the executor's ledger
   format, preserving existing history.
2. For code, write an executable behavior test. Run
   `mulix test run --task N --stage red` and observe the mapped failure.
   Make the implementation change, then run `mulix test run --task N --stage green`.
   Keep the approved inputs, policy and test stable across the RED/GREEN pair.
   An infrastructure failure is not RED. For a reasoned documentation or
   verification task run `mulix test run --task N --stage check`.
3. Record verified step outcomes and command/result evidence in the task's
   `### Task N` ledger section. **Native task files are not progress
   files:** preserve their prose and inputs, including all step markers.
   **Legacy only:** mark only verified step checkboxes `[X]` in `tasks.md`
   and refresh that task's brief via `task-brief`. Skipped/future steps and
   fenced examples stay unchanged. Commit only with authorization.
4. Check the actual current GREEN/CHECK with `mulix test receipt --task N`
   after execution and review. For inline pass that read-only command to
   `task-done`, rather than running GREEN/CHECK again. For native completion,
   the owning ledger must contain `- Task N: complete (receipt <id>)` with
   the exact current receipt ID. For legacy retain `- Task N: complete`
   (old plain lines remain valid). A missing or stale receipt is not
   completion. Use the runner summaries in the existing task report and
   record relevant evidence there without overwriting them.

### Optional task Issue closure (GitHub/GitLab only)

While still in Build with that task's current receipt and ledger entry,
run `mulix tracker close task-N --dry-run`. Show the identified Issue and
eligibility. Only with separate explicit authorization, run
`mulix tracker close task-N --expect TOKEN` for that view. Otherwise leave
it open. This changes tracker status only; it neither reruns tests nor
establishes review or integration. A later change can stale the receipt.

## Advance

Resolve ad-hoc logs and build output under the current context's scratch
`verification/`, not under the retained plan. See `mulix-using` for the
runtime artifact policy. Keep the task progress ledger and report summaries.

Finish the executor's review and report rulings and deferred findings, but
leave branch integration to Archive. Run `mulix guard build-complete`, then
`mulix state transition build-complete`. The gate checks the complete owning
ledger and runner evidence; a ruling, checkbox, or remote close is not proof
of execution. For GitHub/GitLab all bound snapshots must still be current.
If the plan needs a substantive change, stop and raise it with the human
instead of editing approved inputs or forcing past drift.
