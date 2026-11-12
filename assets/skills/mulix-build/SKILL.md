---
name: mulix-build
description: Use when the active change is in the build phase, before writing any production code or test for it.
---

# Build phase

tasks.md is the implementation plan. Code tasks execute under
`test-driven-development`; reasoned verification/documentation tasks run CHECK.
This skill picks the executor and connects its records to actual CLI execution.

## 1. Choose the execution method

Read tasks.md once (and the spec it names). Then recommend one method in
one sentence — from how much the tasks depend on each other's
interfaces, how many there are, what a shipped mistake would cost — and
ask the human to choose:

- **subagent-driven** — a fresh implementer subagent per task, a task
  review (spec compliance + quality) after each, a whole-branch review at
  the end. Most thorough; a fresh context per task and per review.
- **inline** — you implement every task in this session, then one fresh
  reviewer checks the whole branch. Cheapest; no independent review until
  the end.

Wait for the answer, then record it:

```
mulix state set execution_method subagent-driven   # or: inline
```

On a return from verify (`verify_failures` > 0), keep the recorded
method unless the human asks to change it.

## 2. Execute

**REQUIRED SUB-SKILL:** load `test-driven-development` before the first code
 task. For a declared, reasoned non-code task, the CHECK contract below replaces
 the executor's generic RED/GREEN steps. Then, by method:

- subagent-driven → **REQUIRED SUB-SKILL:** `subagent-driven-development`
- inline → **REQUIRED SUB-SKILL:** `executing-plans`

The plan file is `docs/changes/<change>/tasks.md`. Follow the executor
skill exactly, including its setup, ledger, reviews, and finish, with
these mulix specifics:

- **Workspace.** `sdd-workspace` resolves to
  `.mulix/.runtime/<change>/sdd/tasks/` using `mulix context show --field
  runtime-dir` with the current session identity. Follow the
  [session context rules](../mulix-using/SKILL.md#session-context), not a direct
  read of global active. The ledger (`progress.md`), task briefs, reports, test
  logs, and review packages all live there. Run the scripts from the
  project root.
- **Ledger layout.** Both executors initialize `progress.md` with the
  shared `subagent-driven-development/scripts/progress-init` helper and
  follow `subagent-driven-development/progress-format.md`: execution
  context, pre-flight, task progress (`### Task N`), then final review.
  Keep `- Task N: complete` on its own line, with command/result/evidence
  as child items. Existing plain-line ledgers remain valid and unchanged;
  append new sections rather than reformatting history.
- **Worktree.** `using-git-worktrees` asks before creating one. If you
  do work in a new worktree, run mulix commands from it and keep
  `.mulix/` there — the hook and guards read state relative to the
  directory the agent session runs in.
- **Runner evidence, both methods.** Read the matrix in tasks.md and
  `.mulix/templates/quality-template.md`. For a `code` task, write an executable
  behavioral test, then `mulix test run --task N --stage red`; keep test files,
  policy and approved definitions stable, implement the fix, then
  `mulix test run --task N --stage green`. RED succeeds only when a mapped test
  actually fails; compilation/missing-command failures are not RED. For a
  reasoned `verification` or `documentation` task, run its real executable
  check with `mulix test run --task N --stage check`. The CLI owns receipts in
  state.yaml and appends summaries to `task-N-report.md`; explain relevant
  behavior/output in that same report without overwriting runner summaries.
  `task-done` records structural ledger completion; it does not create runner
  evidence. For inline completion, pass the applicable `mulix test run` command
  as its test command; the prior RED still must have run before implementation.
- **Step status.** After each step actually runs and its expected result
  is verified, change only its `- [ ]` marker in approved tasks.md to
  `- [X]` (capital X). This status-only edit is allowed. Keep task numbers,
  step definitions, code, commands, and all approved content unchanged.
  Leave future or unperformed steps `[ ]`: passing GREEN does not mean
  other steps ran. A commit step becomes `[X]` only after an authorized
  commit actually succeeds; a skipped or unauthorized commit stays `[ ]`
  and is reported, never bulk-checked. Checkboxes record progress; they
  do not replace the ledger completion line or RED/GREEN reports.
  tasks.md is the step-status source of truth; briefs are derived snapshots.
  After each verified status edit, refresh that task's brief with
  `bash <skills-dir>/subagent-driven-development/scripts/task-brief PLAN_FILE N`.
  Leave fenced checkbox examples and other tasks unchanged. For subagents,
  pass PLAN_FILE and N explicitly and include this status-edit contract in
  the dispatch; they may locate their own task without reading the whole plan.
- **Keep the workspace.** The executor skills' finish keeps it (the
  build-complete guard reads it); never `rm -rf` it or `git clean` it
  away.
- **Stop before finishing the branch.** Where the executor says "use
  finishing-a-development-branch", stop instead: post the "Rulings I made"
  list (and "Deferred minors" under inline), then advance to verify.
  Integrating the branch happens in the archive phase.

## Runtime artifact policy

Before creating execution artifacts, follow the
[Runtime artifact policy](../mulix-using/SKILL.md#runtime-artifact-policy).
The SDD workspace holds the native ledger, ownership marker, briefs, reports,
standard task test logs and review diffs. Query `mulix context show --field
scratch-dir` with the current identity for ad-hoc RED/rerun/debug output and
build binaries; create its private `verification/` directory for those files.
Keep the command, relevant failing/passing output and result in the existing
report/ledger. Temporary files may disappear; they are not extra retained
runtime evidence bundles.

## Advancing out of this phase

```
mulix state transition build-complete
```

The guard checks that `execution_method` is recorded, the ledger at
`.mulix/.runtime/<change>/sdd/tasks/progress.md` names tasks.md on its
first line and has a `- Task N: complete` list item for every `## Task N`
section (legacy plain completion lines also count), and every task has valid
runner receipts for its declared kind. Handwritten RED/GREEN prose is not an
execution receipt. A fix round or a Ruling line is not a completion line.

If a task can't be completed as planned, keep its definitions unchanged;
only the verified step-status edits above are allowed. The guard matches
the ledger against the approved tasks. Rule on deviations in the ledger
as the executor skill says, or, if the plan is broken past ruling, tell
the human.
