package guard

import "testing"

func TestCompletedTasks_LedgerFormats(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		want bool
	}{
		{"legacy", "Task 3: complete (commits a..b, review clean)", true},
		{"list", "- Task 3: complete (commits a..b, review clean)", true},
		{"CRLF list", "- Task 3: complete (tests: ok)\r", true},
		{"fix round", "- Task 3: fix round 1/5 (1 addressed, 1 open)", false},
		{"ruling", "- Task 3: Ruling: complete after review", false},
		{"incomplete", "- Task 3: incomplete", false},
		{"completion suffix", "- Task 3: completed", false},
		{"quoted example", "> - Task 3: complete (example)", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CompletedTasks(tc.line)[3]; got != tc.want {
				t.Fatalf("CompletedTasks(%q)[3] = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

func TestPlanTasksCompletePassesWithSectionedLedger(t *testing.T) {
	root := t.TempDir()
	s, ws := buildFixture(t, root, twoTasks)
	writeFile(t, root, ws+"/progress.md", `# SDD progress — plan: docs/changes/001-x/tasks.md

## Execution context

- Execution: inline; no commit or push authorized.

## Pre-flight

| Tasks | Interface | Finding |
|---|---|---|
| 1 → 2 | parser result | signatures match |

## Task progress

### Task 1

- Task 1: complete
  - Commits: uncommitted
  - Tests: `+"`go test ./...`"+`
  - Result: PASS
  - Evidence: [report](task-1-report.md)

### Task 2

- Task 2: Ruling: retained the approved interface.
- Task 2: complete
  - Evidence: [report](task-2-report.md)

## Final review

- Final: review clean; runtime validation remains a separate gate.
`)
	writeFile(t, root, ws+"/task-1-report.md", goodReport)
	writeFile(t, root, ws+"/task-2-report.md", goodReport)
	if report := checkPlanTasksComplete(root, s); !report.Pass {
		t.Fatalf("expected sectioned ledger to pass, failures: %+v", report.Next)
	}
}

func TestPlanTasksCompletePassesWithMarkdownLedger(t *testing.T) {
	root := t.TempDir()
	s, ws := buildFixture(t, root, twoTasks)
	writeFile(t, root, ws+"/progress.md", "# SDD ledger — plan: docs/changes/001-x/tasks.md\n\n- Task 1: complete (commits a..b, review clean)\n- Task 2: Ruling: kept the flag name\n- Task 2: complete (commits b..c, review clean)\n")
	writeFile(t, root, ws+"/task-1-report.md", goodReport)
	writeFile(t, root, ws+"/task-2-report.md", goodReport)
	if report := checkPlanTasksComplete(root, s); !report.Pass {
		t.Fatalf("expected list ledger to pass, failures: %+v", report.Next)
	}
}
