# [FEATURE NAME] Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development (recommended) or executing-plans to implement this plan task-by-task. Steps start as `- [ ] **Step N: ...**`; mark only actually completed steps `- [X] **Step N: ...**`.

<!-- Written by the tasks phase via the writing-plans skill; executed by
     the build phase. mulix requires numbered task sections AND the
     mulix-tests acceptance matrix below. Every task is a "## Task N: <name>"
     section, numbered 1..n in execution order with no gaps. Executors extract
     task briefs by that heading; guards match task declarations and receipts
     to it. -->

**Goal:** [One sentence describing what this builds]

**Architecture:** [2-3 sentences about approach. For a bounded change,
the approach approved in chat during the design phase.]

**Tech Stack:** [Key technologies/libraries]

**Spec:** `docs/specs/[YYYY-MM-DD-change-slug]/spec.md`

**Design:** `.mulix/.runtime/[YYYY-MM-DD-change-slug]/specs/[YYYY-MM-DD-topic]-design.md`
[or: approved in chat (bounded) — summarized under Architecture]

## Global Constraints

[The spec's project-wide requirements — version floors, dependency
limits, naming and copy rules, platform requirements — one line each,
exact values copied verbatim from the spec, plus any MUST principle from
`.mulix/memory/constitution.md` that binds this change. Every task's
requirements implicitly include this section.]

## Review Focus

[The five input classes or failure modes the spec implies but no task's
tests exercise that are most likely to bite a person using this software
— one line each, most likely first. Each line gets its pinning test added
to the task that owns the code.]

## Acceptance Test Matrix

Keep exactly one `mulix-tests` YAML fence. Replace the sample mappings with
reviewed cases: declare every numbered task, cover every AC and every task,
and use unique TC identifiers. The `test` is an exact emitted test identity
(`go-json`: module/package#TestName); `file` is its project-relative source file,
not a runtime output. Expected values come from the specification.
Read `.mulix/templates/quality-template.md` and configure project-owned
`.mulix/quality.yaml` before `tasks-complete`. Every case's suite must exist.

```mulix-tests
tasks:
  - number: 1
    kind: code
cases:
  - id: TC-001
    criterion: AC-001
    task: 1
    point: exact value preservation
    given: a literal input
    when: the public operation runs
    then: the returned value equals the literal input
    suite: unit
    test: fixture#TestValue
    file: value_test.go
```

For an existing-behavior verification or documentation-only task, declare
`kind: verification` or `kind: documentation` and a concrete `reason`. Its steps
run a meaningful executable check with `mulix test run --task N --stage check`,
not an invented failure. Code tasks use actual runner RED then GREEN; a build
error or missing command is not a behavioral RED. Commit only when authorized.

---

## Task 1: [Component Name]

**Files:**
- Create: `exact/path/to/file`
- Modify: `exact/path/to/existing:123-145`
- Test: `tests/exact/path/to/test`

**Interfaces:**
- Consumes: [what this task uses from earlier tasks — exact signatures]
- Produces: [what later tasks rely on — exact names, parameter and return types]

- [ ] **Step 1: Write the failing test**

```[language]
[complete test code]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `mulix test run --task 1 --stage red`
Expected: Successful RED receipt for the mapped behavioral failure; not a build or environment failure.

- [ ] **Step 3: Write minimal implementation**

```[language]
[complete implementation code]
```

- [ ] **Step 4: Run test to verify it passes**

Run: `mulix test run --task 1 --stage green`
Expected: All selected suites and mapped tests pass; a source-bound GREEN receipt is recorded.

- [ ] **Step 5: Commit**

```bash
git add [files]
git commit -m "[message]"
```

## Task 2: [Component Name]

[Same structure. Repeat the code a task needs rather than writing
"similar to Task 1" — the implementer may read tasks out of order and
only ever sees its own task's brief.]
