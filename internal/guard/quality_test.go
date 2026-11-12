package guard

import (
	"github.com/z-fenix/mulix-coding/internal/flow"
	"testing"
)

func TestQualityGateRejectsHandwrittenTDD(t *testing.T) {
	root := t.TempDir()
	s, ws := buildFixture(t, root, "## Task 1: Nothing executed\n")
	writeFile(t, root, ws+"/progress.md", "# SDD progress — plan: docs/changes/001-x/tasks.md\n- Task 1: complete\n")
	writeFile(t, root, ws+"/task-1-report.md", "RED: not executed\nGREEN: not executed\n")
	if Run(root, s, flow.EventBuildComplete).Passed() {
		t.Fatal("handwritten RED/GREEN accepted without actual execution")
	}
}
func TestQualityGateRejectsManualVerifyPassAndUnverifiedArchive(t *testing.T) {
	root := t.TempDir()
	s := flow.New("001-x", "")
	s.Phase = flow.PhaseVerify
	s.VerifyResult = flow.VerifyPass
	s.ReportPath = "docs/changes/001-x/report.md"
	writeFile(t, root, s.ReportPath, "All tests passed (not actually executed).\n")
	if Run(root, s, flow.EventVerifyPass).Passed() {
		t.Fatal("manual pass accepted without runner evidence")
	}
	s.Phase = flow.PhaseArchive
	s.ArchiveConfirmation = flow.ArchiveConfirmed
	if Run(root, s, flow.EventArchived).Passed() {
		t.Fatal("unverified change normally archived")
	}
}
func TestQualityGateRejectsSpecAndPlanWithoutAcceptance(t *testing.T) {
	root := t.TempDir()
	s := flow.New("001-x", "")
	s.SpecPath = "docs/specs/001-x/spec.md"
	writeFile(t, root, s.SpecPath, "# Placeholder spec\n")
	if Run(root, s, flow.EventSpecComplete).Passed() {
		t.Fatal("spec without testable acceptance accepted")
	}
	s.Phase = flow.PhaseTasks
	s.TasksPath = "docs/changes/001-x/tasks.md"
	writeFile(t, root, s.TasksPath, "## Task 1: Unmapped task\n")
	if Run(root, s, flow.EventTasksComplete).Passed() {
		t.Fatal("plan without acceptance mapping accepted")
	}
}
