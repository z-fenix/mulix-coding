# SDD ledger — tasks: docs/changes/001-add-task-priority-levels/tasks.md

T001: complete (review clean)
T002: complete (review clean)
T003: complete (review clean)
T004: complete (verification only)
T005: complete (review clean)
T006: complete (review clean)
T007: complete (review clean)
T008: fix round 1/5 (1 addressed, 0 open — stability-only tie-break was
      input-order-dependent; explicit CreatedAt comparison added; diff
      in review-T008-fix1.diff)
T008: complete (review clean)
T009: complete (review clean)
T010: complete (review clean; two test-side corrections recorded in
      task_T010_report.md)
T011: complete (full suite green)
Ruling: SortByPriority's tie-break is part of its contract, not an
implementation detail — made input-order-independent via an explicit
CreatedAt comparison instead of relying on stability — costs if wrong:
none (strictly more deterministic than the stability-only version)
