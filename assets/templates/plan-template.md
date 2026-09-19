# Implementation Plan: [FEATURE NAME]

**Change**: `[###-change-slug]` | **Date**: [DATE] | **Spec**: [SPEC_PATH]

**Input**: Feature specification from `docs/specs/[###-change-slug]/spec.md`

**Note**: This template is filled in by the plan phase (see the
`mulix-plan` skill); keep it proportional to how the change was
classified during specify — a spike's plan is a few sentences per
section, a bounded change's plan is a short paragraph each, an
architectural change's plan can point back at the design doc written
during specify rather than repeating it.

## Summary

[Extract from feature spec: primary requirement + technical approach]

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical
  details for the change. The structure here is presented in advisory
  capacity to guide the planning process.
-->

**Language/Version**: [e.g., Go 1.22, Python 3.11, Rust 1.75 or NEEDS CLARIFICATION]

**Primary Dependencies**: [e.g., cobra, FastAPI, LLVM or NEEDS CLARIFICATION]

**Storage**: [if applicable, e.g., PostgreSQL, files or N/A]

**Testing**: [e.g., go test, pytest, cargo test or NEEDS CLARIFICATION]

**Target Platform**: [e.g., Linux server, CLI, WASM or NEEDS CLARIFICATION]

**Project Type**: [e.g., library/cli/web-service/mobile-app or NEEDS CLARIFICATION]

**Performance Goals**: [domain-specific, e.g., 1000 req/s, 60 fps or NEEDS CLARIFICATION]

**Constraints**: [domain-specific, e.g., <200ms p95, offline-capable or NEEDS CLARIFICATION]

**Scale/Scope**: [domain-specific, e.g., 10k users, 50 screens or NEEDS CLARIFICATION]

## Constitution Check

*GATE: Must pass before starting design work. Re-check before the tasks phase.*

[Gates determined based on docs/constitution.md, if one exists for this project]

## Approach

<!-- The chosen approach and why, in a few sentences. If exploring the
     options during specify surfaced alternatives, note what was
     rejected and why. -->

## Design Notes

<!-- Data model, contracts, interfaces touched. Keep it proportional to
     how the change was classified during specify: skip entirely for a
     spike; a short paragraph for a bounded change; for an architectural
     change, link out to the full design doc under docs/ if one was
     written rather than duplicating it here. -->

## Project Structure

### Documentation (this change)

```text
docs/specs/[###-change-slug]/
└── spec.md               # Feature specification (specify phase)

docs/changes/[###-change-slug]/
├── plan.md               # This file (plan phase)
├── tasks.md              # Dependency-ordered checklist (tasks phase)
├── analyze.md            # Cross-artifact consistency findings (analyze phase)
└── report.md             # Verification results (verify phase)
```

### Source Code (repository root)

<!--
  ACTION REQUIRED: Replace the placeholder tree below with the concrete
  layout for this change. Delete unused options and expand the chosen
  structure with real paths. The delivered plan must not include Option
  labels.
-->

```text
# [REMOVE IF UNUSED] Option 1: Single project (DEFAULT)
src/
├── models/
├── services/
├── cli/
└── lib/

tests/
├── contract/
├── integration/
└── unit/

# [REMOVE IF UNUSED] Option 2: Web application (when "frontend" + "backend" detected)
backend/
├── src/
│   ├── models/
│   ├── services/
│   └── api/
└── tests/

frontend/
├── src/
│   ├── components/
│   ├── pages/
│   └── services/
└── tests/
```

**Structure Decision**: [Document the selected structure and reference the real
directories captured above]

## Risks

- ...

## Test Strategy

<!-- What will be tested and how, before any implementation code is
     written (this feeds directly into the build phase's TDD discipline).
     Be concrete about what gets tested and in what order — this section
     matters most, since it's what the build phase actually follows. -->

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|---------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct access insufficient] |
