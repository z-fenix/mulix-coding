package guard

import (
	"os"
	"path/filepath"
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
	if err := os.WriteFile(tasksPath, []byte("- [x] task one\n- [x] task two\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := flow.New("x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	if !report.Passed() {
		t.Fatalf("expected guard to pass, failures: %+v", report.Failures())
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
	content := "<!-- keep the exact \"- [ ]\" checkbox syntax -->\n- [x] task one\n- [x] task two\n"
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
