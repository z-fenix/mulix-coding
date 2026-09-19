# Specification Analysis Report

**Change**: `[###-change-slug]`

<!-- Read-only cross-artifact consistency check across spec.md / plan.md /
     tasks.md (and docs/constitution.md, if one exists). List findings; do
     not edit the other artifacts from here — report them and let the
     human decide whether to go back a phase. -->

## Findings

<!-- One row per finding, most severe first. Generate stable IDs prefixed
     by category initial (e.g. D1 = Duplication #1, A1 = Ambiguity #1).
     Categories: Duplication, Ambiguity, Underspecification, Constitution
     Alignment, Coverage Gap, Inconsistency. Severities: CRITICAL (violates
     a constitution MUST, or a requirement with zero coverage that blocks
     baseline functionality), HIGH (conflicting requirement, untestable
     acceptance criterion), MEDIUM (terminology drift, missing
     non-functional task coverage), LOW (wording/style only). -->

| ID | Category | Severity | Location(s) | Summary | Recommendation |
|----|----------|----------|--------------|---------|-----------------|
| ... | ... | ... | spec.md:L### | ... | ... |

## Coverage Summary

<!-- Map each functional/success-criteria requirement (FR-###/SC-###) to
     the task(s) that implement it. A requirement with no task is a
     coverage gap; a task with no requirement is drift. -->

| Requirement | Has Task? | Task IDs | Notes |
|-------------|-----------|----------|-------|

**Constitution alignment issues**: [none, or list]

**Unmapped tasks**: [none, or list]

**Metrics**:

- Total requirements: [N]
- Total tasks: [N]
- Coverage %: [requirements with ≥1 task]
- Critical issues: [N]

## Verdict

<!-- e.g. "No blocking inconsistencies found." or a list of what must be
     fixed in spec/plan/tasks before build. If CRITICAL findings exist,
     recommend resolving them before advancing to build. -->
