package guard

import (
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
)

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

func TestRun_BuildCompletePassesWithMarkdownLedger(t *testing.T) {
	root := t.TempDir()
	s, ws := buildFixture(t, root, twoTasks)
	writeFile(t, root, ws+"/progress.md", "# SDD ledger — plan: docs/changes/001-x/tasks.md\n\n- Task 1: complete (commits a..b, review clean)\n- Task 2: Ruling: kept the flag name\n- Task 2: complete (commits b..c, review clean)\n")
	writeFile(t, root, ws+"/task-1-report.md", goodReport)
	writeFile(t, root, ws+"/task-2-report.md", goodReport)
	if report := Run(root, s, flow.EventBuildComplete); !report.Passed() {
		t.Fatalf("expected list ledger to pass, failures: %+v", report.Failures())
	}
}
