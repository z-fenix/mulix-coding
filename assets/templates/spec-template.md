# Feature Specification: [FEATURE NAME]

**Change**: `[YYYY-MM-DD-change-slug]`

**Created**: [DATE]

**Status**: Draft

**Input**: User description: "$ARGUMENTS"

## Acceptance Baseline & Scope *(mandatory)*

- **Product authority**: [产品真相源路径、锁定提交/版本；适用的开发侧裁定及确认记录]
- **Constitution**: [实际项目宪法路径与版本；不能将未填充的 constitution-template 当作已生效约束]
- **Test authority**: [本变更 AC/TC；若项目声明额外验收来源，记录其路径、版本和原始 ID，不推断已批准]
- **Delivery boundary**: [本次是内核切片、后端集成还是完整业务能力；前端、部署及外部依赖分别说明]
- **Acceptance state**: NOT_RUN（范围批准、实现完成、测试设计准入均不代表执行通过）
- **Exclusions and approval**: [逐项列出排除项、理由、影响及明确确认记录；没有排除则写“无”]
- **Blocking gates**: [未闭合依赖、安全/授权/环境门禁及其责任人；用户跳过不计通过，不自动移出全量范围]

验收前锁定被测实现、需求、用例和环境基线。来源变化须审阅影响并重验受影响项，不把旧通过记录重新标为新版本通过。阶段性子范围结论不得写成整个需求或变更验收通过。

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

## Acceptance Mapping *(mandatory)*

| Requirement / Rule / SC | AC or project-supplied source ID | Observable expected result | Preconditions | Execution method / owning task | Coverage gap / blocker |
|---|---|---|---|---|---|
| [FR/SC/RULE ID] | [AC ID; original external source ID if the project defines one] | [Observable result, boundary and any forbidden effect] | [Relevant initial state, actor, environment and controlled inputs] | [Executable or manual check; identify its owner] | [Outstanding work and its owner] |

- Cover every in-scope requirement and success criterion. A linked ID is not proof that its expected behavior was checked. Keep native executable AC → TC mappings; use a separate source only when the project declares it.
- Report only what the actual evidence proves, identifying the executed checks and the source/environment baselines. A component result does not prove the integrated behavior.
- Partial, failed, blocked, skipped or unexecuted required checks do not permit full acceptance; retain the exclusions, their approvals and outstanding obligations.

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
