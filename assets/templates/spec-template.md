# Feature Specification: [FEATURE NAME]

**Change**: `[YYYY-MM-DD-change-slug]`

**Created**: [DATE]

**Status**: Draft

**Input**: User description: "$ARGUMENTS"

## Acceptance Baseline & Scope *(mandatory)*

- **Product authority**: [产品真相源路径、锁定提交/版本；适用的开发侧裁定及确认记录]
- **Constitution**: [实际项目宪法路径与版本；不能将未填充的 constitution-template 当作已生效约束]
- **Formal case authority**: [正式用例正文路径、版本/哈希及索引路径；保留原始 ID，候选与正式分开；无外部工作台时明确采用本规格 AC/TC，不虚构外部正式准入]
- **Delivery boundary**: [本次是内核切片、后端集成还是完整业务能力；前端、部署及外部依赖分别说明]
- **Acceptance state**: NOT_RUN（范围批准、实现完成、测试设计准入均不代表执行通过）
- **Exclusions and approval**: [逐项列出排除项、理由、影响及明确确认记录；没有排除则写“无”]
- **Blocking gates**: [未闭合依赖、安全/授权/环境门禁及其责任人；用户跳过不计通过，不自动移出全量范围]

验收前锁定被测实现、产品、正式用例和环境四类基线。来源变化须审阅影响并重验受影响项，不把旧通过记录重新标为新版本通过。阶段性子范围结论不得写成整个需求或变更验收通过。

## User Scenarios & Testing *(mandatory)*

<!--
  IMPORTANT: User stories should be PRIORITIZED as user journeys ordered by importance.
  Each user story/journey must be INDEPENDENTLY TESTABLE - meaning if you implement just ONE of them,
  you should still have a viable MVP (Minimum Viable Product) that delivers value.

  Assign priorities (P1, P2, P3, etc.) to each story, where P1 is the most critical.
  Think of each story as a standalone slice of functionality that can be:
  - Developed independently
  - Tested independently
  - Deployed independently
  - Demonstrated to users independently

  For a small bounded change or a spike (see how the change was
  classified during specify), a single User Story 1 is often enough —
  don't invent P2/P3 stories that don't exist just to fill out the
  template.
-->

### User Story 1 - [Brief Title] (Priority: P1)

[Describe this user journey in plain language]

**Why this priority**: [Explain the value and why it has this priority level]

**Independent Test**: [Describe how this can be tested independently - e.g., "Can be fully tested by [specific action] and delivers [specific value]"]

**Acceptance Scenarios**:

1. **Given** [initial state], **When** [action], **Then** [expected outcome]
2. **Given** [initial state], **When** [action], **Then** [expected outcome]

---

### User Story 2 - [Brief Title] (Priority: P2)

[Describe this user journey in plain language]

**Why this priority**: [Explain the value and why it has this priority level]

**Independent Test**: [Describe how this can be tested independently]

**Acceptance Scenarios**:

1. **Given** [initial state], **When** [action], **Then** [expected outcome]

---

### User Story 3 - [Brief Title] (Priority: P3)

[Describe this user journey in plain language]

**Why this priority**: [Explain the value and why it has this priority level]

**Independent Test**: [Describe how this can be tested independently]

**Acceptance Scenarios**:

1. **Given** [initial state], **When** [action], **Then** [expected outcome]

---

[Add more user stories as needed, each with an assigned priority]

### Edge Cases

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right edge cases.
-->

- What happens when [boundary condition]?
- How does system handle [error scenario]?

## Requirements *(mandatory)*

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right functional requirements. Mark every
  ambiguous requirement inline with NEEDS CLARIFICATION rather than
  guessing — the clarify phase exists specifically to resolve these.
-->

### Functional Requirements

- **FR-001**: System MUST [specific capability, e.g., "allow users to create accounts"]
- **FR-002**: System MUST [specific capability, e.g., "validate email addresses"]
- **FR-003**: Users MUST be able to [key interaction, e.g., "reset their password"]
- **FR-004**: System MUST [data requirement, e.g., "persist user preferences"]
- **FR-005**: System MUST [behavior, e.g., "log all security events"]

*Example of marking unclear requirements:*

- **FR-006**: System MUST authenticate users via [NEEDS CLARIFICATION: auth method not specified - email/password, SSO, OAuth?]
- **FR-007**: System MUST retain user data for [NEEDS CLARIFICATION: retention period not specified]

### Key Entities *(include if feature involves data)*

- **[Entity 1]**: [What it represents, key attributes without implementation]
- **[Entity 2]**: [What it represents, relationships to other entities]

## Acceptance Criteria *(mandatory)*

Replace this example with concrete, independently derived expectations. Keep
exactly one `mulix-acceptance` YAML fence. Every FR identifier in this spec must
have at least one AC; each `requirement` is a single existing FR identifier.
Delete unused example requirements and clarification examples before approval.
An expected result describes observable behavior, not “matches implementation”.

```mulix-acceptance
criteria:
  - id: AC-001
    requirement: FR-001
    expected: The requested value is returned unchanged.
```

The tasks phase maps each AC to executable cases; the runner verifies those exact
tests actually ran. Traceability does not prove an assertion is sufficient: review
normal, boundary, error, permission and integration behavior against the spec.

## Success Criteria *(mandatory)*

<!--
  ACTION REQUIRED: Define measurable success criteria.
  These must be technology-agnostic and measurable.
-->

### Measurable Outcomes

- **SC-001**: [Measurable metric, e.g., "Users can complete account creation in under 2 minutes"]
- **SC-002**: [Measurable metric, e.g., "System handles 1000 concurrent users without degradation"]
- **SC-003**: [User satisfaction metric, e.g., "90% of users successfully complete primary task on first attempt"]
- **SC-004**: [Business metric, e.g., "Reduce support tickets related to [X] by 50%"]

## Formal Acceptance Mapping *(mandatory)*

| Requirement / Rule / SC | Formal case ID / admission | Checkpoint ID & observable expected result | Preconditions / business scope | Execution method / owning task | Coverage gap / blocker |
|---|---|---|---|---|---|
| [FR/SC/RULE ID] | [原始正式 ID；未准入则明确 candidate/BLOCKED；无外部工作台时引用本规格 AC/TC] | [逐检查点，含金额/状态/数量及禁止副作用，不只写 HTTP 200] | [身份、工程/单位/对象、初态、受控数据] | [真实产品自动化或人工；由设计/任务细化设施] | [缺口及承接者；不能靠删除行获得通过] |

- 每个范围内 FR/SC 均须承接；一个 case 关联多个需求，不代表所有预期已经有断言。正式用例的前置、步骤、全部预期都纳入受审检查点清单；保留原生 AC → TC 的可执行映射，不以本表代替它。
- 仅允许报告证据实际证明的结论：内核测试通过 ≠ 生产装配通过 ≠ 正式业务验收通过。验收报告逐点记录实际结果、运行 ID/人工记录和来源版本。
- 范围内任一检查点 PARTIAL / FAIL / BLOCKED / NOT_RUN / SKIPPED_BY_USER 或证据不足，不能声明全量 PASS；子范围结论须保留未验范围与已批准门禁。

## Assumptions

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right assumptions based on reasonable defaults
  chosen when the feature description did not specify certain details.
-->

- [Assumption about target users, e.g., "Users have stable internet connectivity"]
- [Assumption about scope boundaries, e.g., "Mobile support is out of scope for v1"]
- [Assumption about data/environment, e.g., "Existing authentication system will be reused"]
- [Dependency on existing system/service, e.g., "Requires access to the existing user profile API"]

## Out of Scope

- ...

## Clarifications

<!-- The clarify phase's guard checks for this exact "## Clarifications"
     heading. Record each question and its answer here as the clarify
     phase resolves NEEDS CLARIFICATION markers above, or set
     clarify_skipped=true in state and explain why here instead. -->
