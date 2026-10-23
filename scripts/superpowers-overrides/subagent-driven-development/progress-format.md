# Runtime progress format

Both executors use `progress-template.md` in this directory. From either
executor's skill directory, run
`bash ../subagent-driven-development/scripts/progress-init PLAN_FILE` at setup.
It creates only a missing ledger; existing files are left byte-for-byte intact.
The first line of a new file is `# SDD progress — plan: <plan file path>`, followed
by a blank line. Keep that identity line unformatted for the phase guard;
legacy `# SDD ledger — plan: ...` identities remain valid and unchanged.

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

Keep the machine-readable completion marker on its own line, exactly
`- Task N: complete`. Supporting facts go in indented child items:

```markdown
### Task 1

- Task 1: Ruling: <finding> — <decision and reason> — <cost if wrong>
- Task 1: complete
  - Commits: <actual range, or explicitly uncommitted>
  - Tests: <command actually run>
  - Result: <observed result>
  - Evidence: [test log](task-1-tests.log), [report](task-1-report.md)
```

This is a format example, not evidence that Task 1 is complete. Apply the
executor's completion contract before writing that marker. Inline
`task-done` appends to the current task section or creates one, then writes
the completion marker, test command/result, and log link after successful
tests and brief refresh; do not duplicate its entry.
It records the actual BASE..HEAD range, not proof of an authorized commit.
If commits were skipped, record that fact separately and leave commit
steps unchecked. Link only files that exist; inline reports remain the
executor's responsibility.

Use inline code for commands, paths, ranges, and short results; relative
Markdown links for evidence; fenced code blocks for long output in the
report or log, not a giant ledger paragraph. Give headings and tables a
blank line on either side. Each execution constraint or pre-flight note
is a separate item/row, not multiple events on one prose line.

## Resume and historical compatibility

List completion markers and legacy plain `Task N: complete ...` lines
both count. Keep historical text, first-line identity, and event order
unchanged; no bulk formatting migration. In an existing ledger, append
new task sections after a blank line without retrofitting its headings.
For a file with no final newline, append two newline characters before
the next section; preserve every existing byte.
A fresh template contains no completion examples or pre-checked statuses.
