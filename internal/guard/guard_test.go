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

const twoTasks = "# Tasks\n\n## Task 1: Parser\n\nsteps\n\n## Task 2: CLI\n\nsteps\n"

// writeFile writes rel (slash-separated) under root.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// buildFixture lays out a change whose tasks.md holds tasksMD and whose
// plan workspace (as sdd-workspace creates it) is .mulix/.runtime/001-x/
// sdd/tasks/, returning the state and the workspace's rel path.
func buildFixture(t *testing.T, root, tasksMD string) (flow.State, string) {
	t.Helper()
	writeFile(t, root, "docs/changes/001-x/tasks.md", tasksMD)
	ws := ".mulix/.runtime/001-x/sdd/tasks"
	writeFile(t, root, ws+"/plan-path", "docs/changes/001-x/tasks.md\n")
	s := flow.New("001-x", "")
	s.Phase = flow.PhaseBuild
	s.TasksPath = "docs/changes/001-x/tasks.md"
	s.ExecutionMethod = flow.ExecInline
	return s, ws
}

const goodReport = "## Report\n**TDD Evidence**\n- RED: `go test ./...` → FAIL: undefined: Parse (expected, not built yet)\n- GREEN: `go test ./...` → ok\n"

func failureNamed(report Report, name string) *Result {
	for _, f := range report.Failures() {
		if f.Name == name {
			return &f
		}
	}
	return nil
}

func TestTaskNumbers_IgnoresFencedHeadings(t *testing.T) {
	md := "## Task 1: A\n```markdown\n### Task 9: example inside a fence\n```\n### Task 2: B\n## Tasks overview\n"
	got := TaskNumbers(md)
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("TaskNumbers = %v, want [1 2]", got)
	}
}

func TestRun_TasksCompleteRequiresTaskSections(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/changes/001-x/tasks.md", "- [ ] T001 old checkbox style\n")
	s := flow.New("001-x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventTasksComplete)
	if failureNamed(report, "tasks-have-task-sections") == nil {
		t.Fatalf("expected tasks-have-task-sections to fail on a checkbox-only tasks.md, got %+v", report.Results)
	}
}

func TestRun_TasksCompleteRejectsGappedNumbering(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/changes/001-x/tasks.md", "## Task 1: A\n## Task 3: C\n")
	s := flow.New("001-x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	if Run(root, s, flow.EventTasksComplete).Passed() {
		t.Fatal("expected Task 1, Task 3 numbering to be rejected")
	}
}

func TestRun_TasksCompletePassesWithNumberedSections(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/changes/001-x/tasks.md", twoTasks)
	s := flow.New("001-x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	if report := Run(root, s, flow.EventTasksComplete); !report.Passed() {
		t.Fatalf("expected pass, failures: %+v", report.Failures())
	}
}

func designFixture() flow.State {
	s := flow.New("001-x", "")
	s.Phase = flow.PhaseDesign
	s.DesignTrack = flow.TrackBounded
	s.DesignApproved = true
	return s
}

func TestRun_DesignApprovedPassesForApprovedBoundedDesign(t *testing.T) {
	root := t.TempDir()
	if report := Run(root, designFixture(), flow.EventDesignApproved); !report.Passed() {
		t.Fatalf("expected pass, failures: %+v", report.Failures())
	}
}

func TestRun_DesignApprovedRequiresTrack(t *testing.T) {
	root := t.TempDir()
	s := designFixture()
	s.DesignTrack = ""
	if failureNamed(Run(root, s, flow.EventDesignApproved), "design-approved") == nil {
		t.Fatal("expected design-approved to fail without a classified track")
	}
}

func TestRun_DesignApprovedRequiresExplicitApproval(t *testing.T) {
	root := t.TempDir()
	s := designFixture()
	s.DesignApproved = false
	if failureNamed(Run(root, s, flow.EventDesignApproved), "design-approved") == nil {
		t.Fatal("expected design-approved to fail without an explicit approval")
	}
}

func TestRun_DesignApprovedArchitecturalNeedsDesignDocInRuntime(t *testing.T) {
	root := t.TempDir()
	s := designFixture()
	s.DesignTrack = flow.TrackArchitectural

	if failureNamed(Run(root, s, flow.EventDesignApproved), "design-approved") == nil {
		t.Fatal("expected the architectural track to require a design doc")
	}

	// A design doc outside the change's runtime specs dir doesn't count.
	writeFile(t, root, "docs/login-design.md", "# Design\n")
	s.DesignPath = "docs/login-design.md"
	if failureNamed(Run(root, s, flow.EventDesignApproved), "design-approved") == nil {
		t.Fatal("expected a design doc outside .mulix/.runtime/<change>/specs/ to be rejected")
	}

	writeFile(t, root, ".mulix/.runtime/001-x/specs/2026-01-01-login-design.md", "# Design\n")
	s.DesignPath = ".mulix/.runtime/001-x/specs/2026-01-01-login-design.md"
	if report := Run(root, s, flow.EventDesignApproved); !report.Passed() {
		t.Fatalf("expected pass with the design doc in place, failures: %+v", report.Failures())
	}
}

func TestRun_TasksCompleteRejectsTasksOutsideChangeDir(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/plans/tasks.md", twoTasks)
	s := flow.New("001-x", "")
	s.TasksPath = "docs/plans/tasks.md"

	if failureNamed(Run(root, s, flow.EventTasksComplete), "tasks-artifact-present") == nil {
		t.Fatal("expected tasks.md outside docs/changes/<change>/ to be rejected")
	}
}

func TestRun_BuildCompleteRequiresExecutionMethod(t *testing.T) {
	root := t.TempDir()
	s, _ := buildFixture(t, root, twoTasks)
	s.ExecutionMethod = ""

	if failureNamed(Run(root, s, flow.EventBuildComplete), "execution-method-chosen") == nil {
		t.Fatal("expected execution-method-chosen to fail when no method was recorded")
	}
}

func TestRun_BuildCompleteFailsWithoutWorkspace(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "docs/changes/001-x/tasks.md", twoTasks)
	s := flow.New("001-x", "")
	s.TasksPath = "docs/changes/001-x/tasks.md"

	report := Run(root, s, flow.EventBuildComplete)
	f := failureNamed(report, "plan-tasks-complete")
	if f == nil {
		t.Fatalf("expected plan-tasks-complete to fail without a workspace, got %+v", report.Results)
	}
	if !strings.Contains(f.Next, ".mulix/.runtime/001-x/sdd") {
		t.Fatalf("expected the hint to name the workspace location, got %q", f.Next)
	}
}

func TestRun_BuildCompleteFailsOnIncompleteLedger(t *testing.T) {
	root := t.TempDir()
	s, ws := buildFixture(t, root, twoTasks)
	writeFile(t, root, ws+"/progress.md", "# SDD ledger — plan: docs/changes/001-x/tasks.md\nTask 1: complete (commits a..b, review clean)\nTask 2: fix round 1/5 (1 addressed, 1 open)\n")
	writeFile(t, root, ws+"/task-1-report.md", goodReport)
	writeFile(t, root, ws+"/task-2-report.md", goodReport)

	f := failureNamed(Run(root, s, flow.EventBuildComplete), "plan-tasks-complete")
	if f == nil {
		t.Fatal("expected plan-tasks-complete to fail while Task 2 is mid fix-loop")
	}
	if !strings.Contains(f.Next, "[2]") {
		t.Fatalf("expected the hint to name Task 2, got %q", f.Next)
	}
}

func TestRun_BuildCompleteRejectsAnotherPlansLedger(t *testing.T) {
	root := t.TempDir()
	s, ws := buildFixture(t, root, twoTasks)
	writeFile(t, root, ws+"/progress.md", "# SDD ledger — plan: docs/other/plan.md\nTask 1: complete (x)\nTask 2: complete (x)\n")
	writeFile(t, root, ws+"/task-1-report.md", goodReport)
	writeFile(t, root, ws+"/task-2-report.md", goodReport)

	if failureNamed(Run(root, s, flow.EventBuildComplete), "plan-tasks-complete") == nil {
		t.Fatal("expected a ledger naming a different plan to be rejected")
	}
}

func TestRun_BuildCompleteRequiresRedAndGreenEvidence(t *testing.T) {
	root := t.TempDir()
	s, ws := buildFixture(t, root, twoTasks)
	writeFile(t, root, ws+"/progress.md", "# SDD ledger — plan: docs/changes/001-x/tasks.md\nTask 1: complete (x)\nTask 2: complete (x)\n")
	writeFile(t, root, ws+"/task-1-report.md", goodReport)
	writeFile(t, root, ws+"/task-2-report.md", "- GREEN: go test ./... → ok\n")

	f := failureNamed(Run(root, s, flow.EventBuildComplete), "tdd-evidence-present")
	if f == nil {
		t.Fatal("expected tdd-evidence-present to fail when Task 2 has no RED evidence")
	}
	if !strings.Contains(f.Next, "Task 2 (no RED evidence)") {
		t.Fatalf("expected the hint to name Task 2's missing RED, got %q", f.Next)
	}
}

func TestRun_BuildCompleteRejectsEmptyPhaseLines(t *testing.T) {
	if got := tddEvidenceProblem("- RED:\n- GREEN:\n"); got == "" {
		t.Fatal("expected bare RED:/GREEN: labels with no evidence to be rejected")
	}
	if got := tddEvidenceProblem("  - **RED:** go test → FAIL\n  - **GREEN:** go test → ok\n"); got != "" {
		t.Fatalf("expected bolded implementer-template labels to count, got %q", got)
	}
}

func TestRun_BuildCompletePassesWithLedgerAndReports(t *testing.T) {
	root := t.TempDir()
	s, ws := buildFixture(t, root, twoTasks)
	writeFile(t, root, ws+"/progress.md", "# SDD ledger — plan: docs/changes/001-x/tasks.md\nTask 1: complete (commits a..b, review clean)\nTask 2: Ruling: kept the flag name — spec says so — rename later\nTask 2: complete (commits b..c, review clean)\n")
	writeFile(t, root, ws+"/task-1-report.md", goodReport)
	writeFile(t, root, ws+"/task-2-report.md", goodReport)

	if report := Run(root, s, flow.EventBuildComplete); !report.Passed() {
		t.Fatalf("expected pass, failures: %+v", report.Failures())
	}
}

func TestFindWorkspace_MatchesMarkerBySuffix(t *testing.T) {
	root := t.TempDir()
	s, _ := buildFixture(t, root, twoTasks)
	// The mulix root is a subdirectory of the git root: the marker is
	// git-root-relative, so it carries an extra leading segment.
	writeFile(t, root, ".mulix/.runtime/001-x/sdd/tasks/plan-path", "app/docs/changes/001-x/tasks.md\n")

	if _, ok := FindWorkspace(root, s); !ok {
		t.Fatal("expected a git-root-relative marker to match the root-relative tasks path")
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
