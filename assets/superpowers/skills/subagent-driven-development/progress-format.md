# Runtime progress format

Both executors use `progress-template.md` in this directory. From either
executor's skill directory, run
`bash ../subagent-driven-development/scripts/progress-init PLAN_FILE` at setup.
It creates only a missing ledger; existing files are left byte-for-byte intact.
The first line of a new file is `# SDD progress — plan: <plan path>`, followed
by a blank line. For a native plan, `<plan path>` is the Mulix-root-relative
`issues/` directory identity from `plan-path`, without a trailing slash.
Keep it unformatted for the phase guard. Legacy ledger identities remain
valid and unchanged.

## Sections, in order

1. **Execution context** — separate list items for execution method,
   authorization boundaries, environment constraints, and global rulings.
   Record only known facts; an empty section does not imply authorization.
2. **Pre-flight** — the executor's required interface/conflict table. Put
   rulings in the relevant row or separate list items beside the table.
   If the inline scan finds no shared interfaces, use
   `- Pre-flight: no shared interfaces` instead.
3. **Task progress** — one `### Task N` section per task, created when that
   task starts. Each status, fix round, ruling, and deferred minor is its
   own list item. Preserve event order; append follow-up task sections when
   revisiting a task rather than moving or rewriting previous events.
4. **Final review** — append this section only when whole-plan review
   begins. Record review evidence, final rulings, deferred issues, and
   unverified gates separately. If execution resumes after final review,
   append `## Task progress (continued)` before the new task sections.

## Task record

For native plans, record step progress here as `- Step N: verified —
<observed result or receipt>`. Record a skipped or blocked step separately;
never mark it verified. Do not edit the task document. Legacy `tasks.md`
keeps its historical checked-box step status.

Keep the machine-readable completion marker on its own line. Native plans
use `- Task N: complete (receipt <id>)`, where `<id>` is the exact 32-digit
lowercase hexadecimal ID returned by `mulix test receipt --task N --id-only`
for that task. Legacy plans keep `- Task N: complete`. Supporting facts go in
indented child items:

```markdown
### Task 1

- Task 1: Ruling: <finding> — <decision and reason> — <cost if wrong>
- Task 1: complete (receipt <id>)
  - Commits: <actual range, or explicitly uncommitted>
  - Tests: <command actually run>
  - Result: <observed result>
  - Evidence: [test log](task-1-tests.log), [report](task-1-report.md)
```

This is a format example, not evidence that Task 1 is complete. Apply the
executor's completion contract before writing that marker. Inline
`task-done` appends to the current task section or creates one, then writes
the completion marker, command/result, and log link after the receipt check
and brief refresh; do not duplicate its entry. It records the actual BASE..HEAD
range, not proof of an authorized commit or merge. If commits were skipped,
record that separately and leave legacy commit steps unchecked. Link only
files that exist.

Use inline code for commands, paths, ranges, and short results; relative
Markdown links for evidence; fenced code blocks for long output in the
report or log, not a giant ledger paragraph. Give headings and tables a
blank line on either side. Each execution constraint or pre-flight note
is a separate item/row, not multiple events on one prose line.

## Runner evidence in mulix projects

Completion markers and prose are structural records, not test receipts.
Use `mulix test run --task N --stage red`, implement, then run `--stage green`;
reasoned non-code tasks use `--stage check`. The CLI appends summaries to task
reports and owns source-bound receipts in `state.yaml`. Preserve those
summaries when adding review or implementation notes. For native plans,
`task-done` checks a current receipt without rerunning the suite. Final
`mulix verify` runs every configured mandatory suite; neither recording an
execution nor recording review is evidence of integration.

## Resume and historical compatibility

Native `build-complete` checks each closure against the latest valid GREEN or
CHECK receipt. If that task records a new receipt, append a new closure for
it; an older entry cannot close the retry. Earlier tasks can keep their
closure while later tasks legitimately advance the source. Legacy list
markers and plain `Task N: complete ...` lines still count for legacy plans.
Keep historical text, first-line identity, and event order unchanged; no
bulk formatting migration. In an existing ledger, append
new task sections after a blank line without retrofitting its headings.
For a file with no final newline, append two newline characters before
the next section; preserve every existing byte.
A fresh template contains no completion examples or pre-checked statuses.
