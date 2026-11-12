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

- 继承规格的验收基线、范围和阻断项；列出每个业务闭环的验收承接 Task，不把全部取证推迟到最后一步。
- **实现状态与验收状态分列**：Task complete 只表示其批准交付层级完成；正式验收仍使用 NOT_RUN / PARTIAL / PASS / FAIL / BLOCKED / SKIPPED_BY_USER，不把跳过或候选算通过。
- 纯模型/Mock 只证明限定契约；持久化、事务、并发、生产装配与业务旅程分别使用能证明该行为的真实设施。基础任务可由下游集成任务承接正式验收，但须明确 Task 与门禁。
- 正式用例正文和受审检查点清单是分母；FR/SC → 正式 case/checkpoint → feature/人工步骤 → 原始执行证据必须可追踪，不以标题关联代替覆盖。
- 保留原生 mulix-acceptance / mulix-tests / quality.yaml 和 CLI 真实运行回执；项目宪法要求外部正式工作台时，另先审核 acceptance-contract.json、真实执行并生成 acceptance-results.json，运行 verify 技能的附加证据校验。工具自测、dry-run 和报告导出不是业务验收。
- 明确真实测试环境、受控写入授权和工作台产物 producer；缺环境/契约/准入保持 BLOCKED，不造固定结果或默认成功数据。
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

**Acceptance Contract:**
- Requirement / rule / success criterion: [精确 ID]
- Delivery level: [纯内核 / 持久化集成 / 生产装配 / 业务闭环；与本任务批准范围一致]
- Formal case / checkpoint: [原始 ID；缺口或候选保持显式阻断；基础任务写明下游验收承接 Task；无外部工作台时承接原生 AC/TC]
- Given / When / Then: [真实初态、动作、状态/金额/数量/关联/事件及禁止副作用；预期来自产品或批准裁定]
- Facilities and doubles: [真实设施、受控数据与写入授权、替身边界；不以替身调用证明事务/并发]
- Evidence boundary: [本任务能证明什么 / 不能证明什么；实际 producer、执行命令及原始证据位置]
- Downstream gate: [接口可消费条件、生产启用条件、验收承接 Task 和未闭合门禁]
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
