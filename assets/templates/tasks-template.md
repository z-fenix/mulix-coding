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

## Acceptance & Release Gates

- 继承规格的验收基线、范围和阻断项；注明每项要求由哪个任务验证，以及哪些回归必须等全部任务集成后执行。
- **实现状态与验收状态分列**：任务完成只证明该任务范围内的行为；未运行、部分覆盖、失败、阻断或跳过均不能算通过。
- 替身仅证明所覆盖的契约；照实际断言选择能证明集成、持久化、并发及部署行为的设施。明确前置条件和不可替代的真实检查。
- 保留原生 `mulix-acceptance` / `mulix-tests` / `quality.yaml` 和 CLI 实际运行回执。若项目宪法声明附加门禁，按其步骤取证并列出未闭合项。
- 明确实际环境、数据、授权与证据来源。工具自测、dry-run 或仅生成报告都不是行为验收。
- 提交步骤仅在用户授权且实际提交成功后勾选；本模板不自动授权 commit/push/部署或更改其他变更状态。

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
not generated output. Expected values come from the specification. For Karate,
group owned features by `tests/features/task-N/`. Required final-only cases
use `verify_cases` in this same fence; see the quality template for its fields.
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

**Acceptance Contract:**
- Requirement / rule / success criterion: [精确 ID]
- Delivery level: [component / integration / deployment; state what this task proves]
- Cases and checkpoints: [AC/TC IDs; preserve project-supplied IDs when applicable; name the owner of deferred integration evidence]
- Given / When / Then: [initial state, action, observable result and forbidden effects; derive expectations from the approved inputs]
- Facilities and doubles: [actual facilities, controlled inputs and authorization; explain limits of substitutes]
- Evidence boundary: [what the task does and does not establish; execution command and evidence reference]
- Downstream gate: [what enables dependent tasks and who owns unresolved requirements]
- Implementation status: NOT_STARTED
- Acceptance status: NOT_RUN

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
