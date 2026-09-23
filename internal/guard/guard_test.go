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

	s := flow.New("001-x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if report.Passed() {
		t.Fatal("expected guard to fail when a checked task has no task report")
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
	if !strings.Contains(evidenceFailure.Next, filepath.ToSlash(filepath.Join(".runtime", "sdd", "task_T001_report.md"))) {
		t.Fatalf("expected failure hint to name the report path, got: %q", evidenceFailure.Next)
	}
}

// writeReport creates a task report in the mandated checklist shape
// (### Task + RED/GREEN(/REFACTOR) checkboxes) under the change's
// .runtime/sdd/ directory.
func writeReport(t *testing.T, root, change, id string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "changes", change, ".runtime", "sdd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "### Task\n- [x] RED: failing test written — pkg_test.go Test" + id + "\n- [x] GREEN: minimal implementation — makes the RED pass\n- [ ] REFACTOR: none needed\n"
	if err := os.WriteFile(filepath.Join(dir, "task_"+id+"_report.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestRun_BuildCompletePassesWhenAllTasksChecked(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("- [x] T001 task one\n- [x] T002 task two\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	writeReport(t, root, "001-x", "T001")
	writeReport(t, root, "001-x", "T002")

	s := flow.New("001-x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if !report.Passed() {
		t.Fatalf("expected guard to pass, failures: %+v", report.Failures())
	}
}

func TestRun_BuildCompleteEvidenceHintMentionsDelegationDoesNotExempt(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("- [x] T001 [US1] Add parser in src/parser.go\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("001-x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"
	s.DelegatedToSubagents = true

	report := Run(root, s, flow.EventBuildComplete)
	if report.Passed() {
		t.Fatal("expected guard to fail when a checked task has no task report, delegated or not")
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
	if !strings.Contains(strings.ToLower(evidenceFailure.Next), "delegat") {
		t.Fatalf("expected failure hint to note delegation doesn't exempt evidence, got: %q", evidenceFailure.Next)
	}
}

func TestRun_BuildCompleteFailsWhenBriefIsEmpty(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("- [x] T001 task one\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	writeReport(t, root, "001-x", "T001")
	briefPath := filepath.Join(changeDir, ".runtime", "sdd", "task_T001_report.md")
	if err := os.WriteFile(briefPath, nil, 0o644); err != nil { // overwrite with empty
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("001-x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if report.Passed() {
		t.Fatal("expected guard to fail when the task report exists but is empty")
	}
}

func TestRun_BuildCompleteFailsWhenBriefHasNoTestReference(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("- [x] T001 task one\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	briefPath := filepath.Join(changeDir, ".runtime", "sdd", "task_T001_report.md")
	if err := os.MkdirAll(filepath.Dir(briefPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// The RED phase is ticked, but GREEN is still an open checkbox — a
	// checked task cannot have an unticked RED/GREEN phase.
	content := "### Task\n- [x] RED: failing test written — pkg_test.go TestOne\n- [ ] GREEN: minimal implementation — pending\n- [ ] REFACTOR: none needed\n"
	if err := os.WriteFile(briefPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("001-x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if report.Passed() {
		t.Fatal("expected guard to fail when the report has an unticked RED/GREEN phase")
	}
}

func TestRun_BuildCompletePassesWhenReportHasOnlyTickedRed(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("- [x] T001 task one\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// A test-writing task's report legitimately ticks only its RED
	// phase — the GREEN phase belongs to the implementation task.
	briefPath := filepath.Join(changeDir, ".runtime", "sdd", "task_T001_report.md")
	if err := os.MkdirAll(filepath.Dir(briefPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "### Task\n- [x] RED: failing tests written — pkg_test.go TestOne\n- [ ] REFACTOR: none needed\n"
	if err := os.WriteFile(briefPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("001-x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if !report.Passed() {
		t.Fatalf("expected guard to pass when the report's RED phase is ticked, failures: %+v", report.Failures())
	}
}

func TestRun_BuildCompleteFailsWhenReportHasNoCheckboxes(t *testing.T) {
	root := t.TempDir()
	changeDir := filepath.Join(root, "docs", "changes", "001-x")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	tasksPath := filepath.Join(changeDir, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte("- [x] T001 task one\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	briefPath := filepath.Join(changeDir, ".runtime", "sdd", "task_T001_report.md")
	if err := os.MkdirAll(filepath.Dir(briefPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(briefPath, []byte("prose only, no Task checklist, no ticked phases\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("001-x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if report.Passed() {
		t.Fatal("expected guard to fail when the report has no ticked RED/GREEN checkbox")
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
	content := "<!-- keep the exact \"- [ ]\" checkbox syntax -->\n- [x] T001 task one\n- [x] T002 task two\n"
	if err := os.WriteFile(tasksPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	writeReport(t, root, "001-x", "T001")
	writeReport(t, root, "001-x", "T002")

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
