---
name: mulix-tasks
description: Use when the active change is in the tasks phase, after its design was approved and before any code is written.
---

# Tasks phase

**REQUIRED SUB-SKILL:** use `writing-plans` for decomposition, executable
steps, and self-review. Use the destinations and phase exit below instead
of its default save path and handoff.

## Inputs

Run `mulix state show`. Read `spec_path`, the approved design (`design_path`
for architectural, the approved in-chat approach for bounded), and the
project constitution. Keep the plan within the approved design and scope.

## Write the approved inputs

- **Native (`runtime-issues-v1`):** Use `.mulix/templates/issue-task-template.md`
  to write one direct `NN-short-name.md` in the pinned `tasks_dir` per task.
  Number tasks `1..n` without gaps; match the filename, the single `## Task N:`
  heading, and the one task in its `mulix-tests` fence. Include the exact
  `spec_path`, requirement/AC IDs, design constraints, dependencies, interfaces
  and review cases that task owns. Use this per-task format instead of the
  combined plan header and checkbox tracking; native progress goes in the
  Build ledger, not these files.
- **Legacy:** Use `.mulix/templates/tasks-template.md` (or the project's
  `mulix preset resolve tasks-template` override), with the combined plan
  header, Spec/Design references, Global Constraints, Review Focus and
  ordered `## Task N:` sections. Use `tasks_path` or, if unset,
  `docs/changes/<change>/tasks.md`. Keep step checkboxes initially unchecked.

For either layout:

- The matrix covers every AC and every task with concrete Given/When/Then,
  an actual suite, emitted test identity and project-relative test file.
  Native task files declare only their own cases, with unique TC IDs across
  the plan. Put final-only Karate coverage in its owning file's `verify_cases`
  (`task: 0`). Check `.mulix/quality.yaml` against the installed
  `quality-template.md`; have the project owner resolve missing suite policy.
- Code tasks plan `mulix test run --task N --stage red`, an implementation
  change, then `--stage green`, with expected observations. Non-code tasks
  declare a reason and an actual `--stage check`. Name remaining gates and
  who owns integration evidence; a task GREEN alone is not that evidence.
- Self-review the complete plan for scope, mapping coverage and executable
  steps. Present it for the existing human plan review and revise the drafts
  on feedback. Execution method and implementation belong to Build.

## Advance

For `github` or `gitlab`, run `mulix tracker publish tasks --dry-run` in
this phase. Show the exact batch and target. Only after explicit publication
approval, run `mulix tracker publish tasks --expect TOKEN`. A pending intent
needs a fresh reviewed dry run for reconciliation, not another create or an
alternate exporter. Bound task snapshots are read-only: use the confirmed
sync workflow for authorized Issue edits. Markdown and legacy skip publication.

For legacy only, record the plan path with `mulix state set tasks_path <path>`.
Then run `mulix guard tasks-complete` and `mulix state transition tasks-complete`.
The remote gate requires a verified current spec and one exact current binding
per task. Stop on drift or missing mappings; do not force the gate.
