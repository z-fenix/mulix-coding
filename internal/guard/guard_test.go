package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mulix-dev/mulix-coding/internal/flow"
)

func TestRun_SpecCompleteFailsWhenArtifactMissing(t *testing.T) {
	root := t.TempDir()
	s := flow.New("x", "")
	s.SpecPath = "docs/specs/001-x/spec.md" // not created on disk

	report := Run(root, s, flow.EventSpecComplete)
	if report.Passed() {
		t.Fatal("expected guard to fail when spec artifact is missing")
	}
	if len(report.Failures()) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(report.Failures()))
	}
}

func TestRun_SpecCompletePassesWhenArtifactPresent(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "docs", "specs", "001-x")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "spec.md"), []byte("# Spec\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("x", "")
	s.SpecPath = "docs/specs/001-x/spec.md"

	report := Run(root, s, flow.EventSpecComplete)
	if !report.Passed() {
		t.Fatalf("expected guard to pass, failures: %+v", report.Failures())
	}
}

func TestRun_BuildCompleteFailsOnUncheckedTasks(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("- [x] task one\n- [ ] task two\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if report.Passed() {
		t.Fatal("expected guard to fail with an unchecked task remaining")
	}
}

func TestRun_BuildCompletePassesWhenAllTasksChecked(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("- [x] task one\n  - tests: a_test.go TestA\n- [x] task two\n  - tests: b_test.go TestB\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if !report.Passed() {
		t.Fatalf("expected guard to pass, failures: %+v", report.Failures())
	}
}

func TestRun_BuildCompleteFailsWhenCheckedTaskHasNoTestEvidence(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("- [x] T001 [US1] Add parser in src/parser.go\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if report.Passed() {
		t.Fatal("expected guard to fail when a checked task has no test evidence")
	}
	var evidenceFailure *Result
	for i, res := range report.Failures() {
		if res.Name == "tdd-evidence-present" {
			evidenceFailure = &report.Failures()[i]
		}
	}
	if evidenceFailure == nil {
		t.Fatalf("expected a tdd-evidence-present failure, got: %+v", report.Failures())
	}
	if !strings.Contains(evidenceFailure.Next, "T001") {
		t.Fatalf("expected failure hint to name the offending task, got: %q", evidenceFailure.Next)
	}
}

func TestRun_BuildCompletePassesWhenTasksAreMarkedNoTest(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	// A documentation task legitimately has no test; [no-test] on the
	// task line is the explicit opt-out from the evidence requirement.
	if err := os.WriteFile(tasksPath, []byte("- [x] T001 [Setup] Update README in README.md [no-test]\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if !report.Passed() {
		t.Fatalf("expected guard to pass for [no-test] tasks, failures: %+v", report.Failures())
	}
}

func TestRun_BuildCompleteFailsWhenTestEvidenceIsDetachedFromTask(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	// The evidence line exists, but prose intervenes between the task and
	// its evidence — the association is broken, so the task counts as
	// unevidenced.
	if err := os.WriteFile(tasksPath, []byte("- [x] T001 [US1] Add parser in src/parser.go\nSome prose line\n  - tests: parser_test.go TestParser\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if report.Passed() {
		t.Fatal("expected guard to fail when evidence is detached from its task")
	}
}

func TestRun_BuildCompleteUncheckedTaskNeedsNoTestEvidence(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("- [ ] T001 pending task\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if report.Passed() {
		t.Fatal("expected guard to fail on the unchecked task itself")
	}
	for _, res := range report.Failures() {
		if res.Name == "tdd-evidence-present" {
			t.Fatalf("unchecked tasks must not require test evidence, got: %+v", report.Failures())
		}
	}
}

func TestRun_BuildCompletePassesWhenOnlyProseMentionsCheckboxSyntax(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	// The literal substring "- [ ]" appears here only inside an
	// explanatory comment, not as an actual task line - this must not
	// be mistaken for an unchecked task (see tasks-template.md's own
	// guidance comment for the real-world case this mirrors).
	content := "<!-- keep the exact \"- [ ]\" checkbox syntax -->\n- [x] task one\n  - tests: a_test.go TestA\n- [x] task two\n  - tests: b_test.go TestB\n"
	if err := os.WriteFile(tasksPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if !report.Passed() {
		t.Fatalf("expected guard to pass when \"- [ ]\" only appears in prose, failures: %+v", report.Failures())
	}
}

func TestRun_VerifyPassRequiresRecordedPassResult(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	reportPath := filepath.Join(changeDir, "report.md")
	if err := os.WriteFile(reportPath, []byte("all green\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("x", "")
	s.ReportPath = "docs/changes/001-x/report.md"
	s.VerifyResult = flow.VerifyFail // report exists, but result says fail

	report := Run(root, s, flow.EventVerifyPass)
	if report.Passed() {
		t.Fatal("expected guard to fail when verify_result is not pass")
	}
}

func TestRun_ArchiveRequiresExplicitConfirmation(t *testing.T) {
	root := t.TempDir()
	s := flow.New("x", "")
	s.ArchiveConfirmation = flow.ArchivePending

	report := Run(root, s, flow.EventArchived)
	if report.Passed() {
		t.Fatal("expected guard to fail without explicit archive confirmation")
	}
}
