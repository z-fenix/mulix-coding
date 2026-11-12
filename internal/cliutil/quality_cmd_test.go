package cliutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func qualityCLIFile(t *testing.T, root, rel, text string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}
func setupQualityCLI(t *testing.T, root, change string) flow.State {
	t.Helper()
	s := flow.New(change, "")
	s.Phase = flow.PhaseBuild
	s.ExecutionMethod = flow.ExecInline
	s.SpecPath = "docs/specs/" + change + "/spec.md"
	s.TasksPath = "docs/changes/" + change + "/tasks.md"
	qualityCLIFile(t, root, s.SpecPath, "# Spec\nFR-001: Return the specified value.\n\n```mulix-acceptance\ncriteria:\n  - id: AC-001\n    requirement: FR-001\n    expected: Value returns exactly two.\n```\n")
	qualityCLIFile(t, root, s.TasksPath, "# Plan\n```mulix-tests\ntasks:\n  - number: 1\n    kind: code\ncases:\n  - id: TC-001\n    criterion: AC-001\n    task: 1\n    point: literal return value\n    given: an initialized module\n    when: Value is called\n    then: the result equals two\n    suite: unit\n    test: fixture#TestValue\n    file: value_test.go\n```\n\n## Task 1: Implement Value\n- [ ] **Step 1: Test**\n")
	qualityCLIFile(t, root, ".mulix/quality.yaml", "schema: mulix.quality.v1\nsuites:\n  - id: unit\n    command: [go, test, -json, -count=1, ./...]\n    cwd: .\n    format: go-json\n    timeout_seconds: 30\n")
	qualityCLIFile(t, root, "go.mod", "module fixture\n\ngo 1.27.0\n")
	qualityCLIFile(t, root, "value.go", "package fixture\nfunc Value() int {return 0}\n")
	qualityCLIFile(t, root, "value_test.go", "package fixture\nimport \"testing\"\nfunc TestValue(t *testing.T){if Value()!=2 {t.Fatal(\"Value must equal two\")}}\n")
	ws := ".mulix/.runtime/" + change + "/sdd/tasks"
	qualityCLIFile(t, root, ws+"/plan-path", s.TasksPath+"\n")
	qualityCLIFile(t, root, ws+"/progress.md", "# SDD progress — plan: "+s.TasksPath+"\n- Task 1: complete\n")
	saveStateForTest(t, root, s)
	if err := state.SetActive(root, change); err != nil {
		t.Fatal(err)
	}
	return s
}
func qualityCommand(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runPresetCLI(t, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return out
}
func realTaskEvidence(t *testing.T, root string) {
	t.Helper()
	qualityCommand(t, "test", "run", "--task", "1", "--stage", "red")
	qualityCLIFile(t, root, "value.go", "package fixture\nfunc Value() int {return 2}\n")
	qualityCommand(t, "test", "run", "--task", "1", "--stage", "green")
}
func TestQualityCLIRealRedGreenVerifyAndStaleEvidence(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	s := setupQualityCLI(t, root, "quality")
	if _, err := runPresetCLI(t, "state", "transition", "build-complete"); err == nil {
		t.Fatal("unexecuted build accepted")
	}
	realTaskEvidence(t, root)
	qualityCommand(t, "state", "transition", "build-complete")
	report := "docs/changes/quality/report.md"
	qualityCLIFile(t, root, report, "# Delivery review\n```mulix-review\nreviewer: independent test reviewer\nresult: approved\nblocking: []\n```\n")
	qualityCommand(t, "state", "set", "report_path", report)
	qualityCommand(t, "state", "set", "verify_result", "pass")
	if _, err := runPresetCLI(t, "state", "transition", "verify-pass"); err == nil {
		t.Fatal("manual verification flag accepted")
	}
	qualityCommand(t, "verify")
	loaded, err := state.Load(root, s.Change)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.VerifyResult != flow.VerifyPass || len(loaded.QualityRuns) != 3 {
		t.Fatalf("missing execution evidence: %+v", loaded)
	}
	qualityCommand(t, "state", "transition", "verify-pass")
	qualityCommand(t, "state", "set", "archive_confirmation", "confirmed")
	qualityCLIFile(t, root, "untracked.go", "package fixture\n// changed after verification\n")
	if _, err := runPresetCLI(t, "state", "transition", "archived"); err == nil {
		t.Fatal("changed source accepted for archive")
	}
	// Revalidation in not-yet-archived archive phase avoids an unrecoverable
	// stale receipt after authorized finishing changes or a new Git revision.
	qualityCommand(t, "verify")
	qualityCommand(t, "state", "transition", "archived")
	final, err := state.Load(root, s.Change)
	if err != nil || !final.Archived {
		t.Fatalf("archive: %+v %v", final, err)
	}
}
func TestQualityCLIArchiveFailureReturnsToBuild(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	setupQualityCLI(t, root, "quality")
	realTaskEvidence(t, root)
	qualityCommand(t, "state", "transition", "build-complete")
	qualityCLIFile(t, root, "docs/changes/quality/report.md", "```mulix-review\nreviewer: independent test reviewer\nresult: approved\nblocking: []\n```\n")
	qualityCommand(t, "verify")
	qualityCommand(t, "state", "transition", "verify-pass")
	qualityCLIFile(t, root, "value.go", "package fixture\nfunc Value() int { return 9 }\n")
	if _, err := runPresetCLI(t, "verify"); err == nil {
		t.Fatal("broken finishing change accepted")
	}
	qualityCommand(t, "state", "transition", "verify-fail")
	loaded, err := state.Load(root, "quality")
	if err != nil || loaded.Phase != flow.PhaseBuild || loaded.VerifyFailures != 1 {
		t.Fatalf("failed archive revalidation cannot recover: %+v %v", loaded, err)
	}
}

func TestQualityCLIFailedRetryInvalidatesPreviousGreen(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	s := setupQualityCLI(t, root, "quality")
	realTaskEvidence(t, root)
	qualityCLIFile(t, root, "value.go", "package fixture\nfunc Value() int {return 9}\n")
	if _, err := runPresetCLI(t, "test", "run", "--task", "1", "--stage", "green"); err == nil {
		t.Fatal("failed retry accepted")
	}
	loaded, err := state.Load(root, s.Change)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.QualityRuns[len(loaded.QualityRuns)-1].Success {
		t.Fatal("failure left a passing latest receipt")
	}
	if _, err := runPresetCLI(t, "state", "transition", "build-complete"); err == nil {
		t.Fatal("older green reused after failed retry")
	}
}
func TestQualityCLIReviewBlockerAndReceiptFields(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	setupQualityCLI(t, root, "quality")
	realTaskEvidence(t, root)
	qualityCommand(t, "state", "transition", "build-complete")
	report := "docs/changes/quality/report.md"
	qualityCLIFile(t, root, report, "```mulix-review\nreviewer: independent reviewer\nresult: approved\nblocking: [data loss]\n```\n")
	if _, err := runPresetCLI(t, "verify"); err == nil {
		t.Fatal("blocking finding accepted")
	}
	for _, field := range []string{"quality_runs", "guard_overrides"} {
		if _, err := runPresetCLI(t, "state", "set", field, "pass"); err == nil {
			t.Fatal("CLI allowed fabricated receipt field", field)
		}
	}
}
func TestQualityCLIForceIsAuditedNotEvidence(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	s := setupQualityCLI(t, root, "quality")
	if _, err := runPresetCLI(t, "state", "transition", "build-complete", "--force"); err == nil {
		t.Fatal("force without an explicit reason accepted")
	}
	qualityCommand(t, "state", "transition", "build-complete", "--force", "--reason", "recover interrupted fixture")
	loaded, err := state.Load(root, s.Change)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.GuardOverrides) != 1 || !strings.Contains(loaded.GuardOverrides[0].Reason, "recover") || len(loaded.QualityRuns) != 0 {
		t.Fatalf("force manufactured evidence or lost audit: %+v", loaded)
	}
	if out := qualityCommand(t, "state", "show"); !strings.Contains(out, "guard_overrides: 1") {
		t.Fatalf("recovery audit is hidden from state inspection: %s", out)
	}
	if _, err := runPresetCLI(t, "state", "transition", "verify-pass"); err == nil {
		t.Fatal("force granted normal delivery eligibility")
	}
}
func TestQualityCLIRespectsSessionSelection(t *testing.T) {
	root, store := sessionProject(t)
	setupQualityCLI(t, root, "X")
	setupQualityCLI(t, root, "Y")
	selectForSession(t, store, "A", "X")
	qualityCommand(t, "test", "run", "--task", "1", "--stage", "red", "--session-id", "A")
	x, _ := state.Load(root, "X")
	y, _ := state.Load(root, "Y")
	if len(x.QualityRuns) != 1 || len(y.QualityRuns) != 0 {
		t.Fatal("runner mutated another session's change")
	}
	if _, err := runPresetCLI(t, "test", "run", "--task", "1", "--stage", "red", "--change", "Y", "--session-id", "A"); err == nil {
		t.Fatal("runner bypassed session binding")
	}
}
