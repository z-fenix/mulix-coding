package cliutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/quality"
	"github.com/z-fenix/mulix-coding/internal/state"
	"github.com/z-fenix/mulix-coding/internal/tracker"
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
func TestQualityCLIReceiptIsReadOnlyAndRequiresCurrentInputs(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	setupQualityCLI(t, root, "quality")
	if _, err := runPresetCLI(t, "test", "receipt", "--task", "1"); err == nil {
		t.Fatal("task without a run returned a receipt")
	}
	realTaskEvidence(t, root)
	file := filepath.Join(root, ".mulix", ".runtime", "quality", "state.yaml")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	out := qualityCommand(t, "test", "receipt", "--task", "1")
	if !strings.Contains(out, "green") || !strings.Contains(out, "receipt") {
		t.Fatalf("closure did not identify the verified run: %s", out)
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(before) {
		t.Fatalf("receipt lookup modified state: %v", err)
	}
	qualityCLIFile(t, root, "value.go", "package fixture\nfunc Value() int {return 5}\n")
	if _, err := runPresetCLI(t, "test", "receipt", "--task", "1"); err == nil {
		t.Fatal("stale task receipt closed a changed implementation")
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

const issueTaskPolicy = `schema: mulix.quality.v1
suites:
  - id: unit
    command: [go, test, -json, -count=1, .]
    cwd: .
    format: go-json
    timeout_seconds: 60
  - id: final
    command: [go, test, -json, -count=1, -run, '^TestFinal$', .]
    cwd: .
    format: go-json
    timeout_seconds: 60
`

func issueTaskText(n int, title, kind, test string) string {
	reason := ""
	if kind != "code" {
		reason = "    reason: Check the already implemented behavior.\n"
	}
	return fmt.Sprintf("## Task %d: %s\n\n", n, title) + "```mulix-tests\n" + fmt.Sprintf(`tasks:
  - number: %d
    kind: %s
%scases:
  - id: TC-%03d
    criterion: AC-001
    task: %d
    point: exact result
    given: an initialized fixture
    when: the behavior is exercised
    then: the required result is observed
    suite: unit
    test: fixture#%s
    file: value_test.go
`, n, kind, reason, n, n, test) + "```\n"
}

func setupIssueQualityCLI(t *testing.T, root, change string, phase flow.Phase) flow.State {
	t.Helper()
	s := flow.New(change, "")
	s.Phase, s.ExecutionMethod = phase, flow.ExecInline
	s.ArtifactLayout = flow.RuntimeIssuesLayout
	s.Tracker = &tracker.ChangeBinding{Target: tracker.Target{Kind: "markdown"}}
	base := ".mulix/.runtime/" + change
	s.SpecPath, s.TasksDir, s.ReportPath = base+"/spec.md", base+"/issues", base+"/report.md"
	qualityCLIFile(t, root, s.SpecPath, "# Spec\nFR-001: Return the specified value.\n\n```mulix-acceptance\ncriteria:\n  - id: AC-001\n    requirement: FR-001\n    expected: Value returns exactly two.\n```\n")
	qualityCLIFile(t, root, s.TasksDir+"/01-value.md", issueTaskText(1, "Implement Value", "code", "TestValue"))
	qualityCLIFile(t, root, s.TasksDir+"/02-existing.md", issueTaskText(2, "Check Existing", "verification", "TestExisting"))
	qualityCLIFile(t, root, quality.PolicyPath, issueTaskPolicy)
	qualityCLIFile(t, root, "go.mod", "module fixture\n\ngo 1.27.0\n")
	qualityCLIFile(t, root, "value.go", "package fixture\nfunc Value() int {return 0}\n")
	qualityCLIFile(t, root, "value_test.go", `package fixture
import "testing"
func TestValue(t *testing.T) { if Value()!=2 { t.Fatal("Value must equal two") } }
func TestExisting(t *testing.T) { if Value()!=2 { t.Fatal("Existing caller requires two") } }
func TestFinal(t *testing.T) { if Value()+1!=3 { t.Fatal("Composed result must be three") } }
`)
	saveStateForTest(t, root, s)
	if err := state.SetActive(root, change); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIssueTaskContract(t *testing.T) {
	t.Run("two tasks and final-only case", func(t *testing.T) {
		root := chdirTemp(t)
		initMulixRoot(t, root)
		s := setupIssueQualityCLI(t, root, "issue", flow.PhaseTasks)
		final := ".mulix/.runtime/issue/tests/features/final/delivery.feature"
		qualityCLIFile(t, root, final, "Feature: Delivery\nScenario: Join\n  * match 2 + 1 == 3\n")
		qualityCLIFile(t, root, quality.PolicyPath, issueTaskPolicy+"  - id: karate\n    format: karate-jsonl\n    karate:\n      version: 2.1.5.RC1\n      sha256: "+strings.Repeat("a", 64)+"\n")
		second := strings.TrimSuffix(issueTaskText(2, "Check Existing", "verification", "TestExisting"), "```\n") +
			"verify_cases:\n  - id: TC-003\n    criterion: AC-001\n    task: 0\n    suite: karate\n    test: '" + final + "#Join'\n    file: '" + final + "'\n```\n"
		qualityCLIFile(t, root, s.TasksDir+"/02-existing.md", second)
		qualityCommand(t, "state", "transition", "tasks-complete")
		if out := qualityCommand(t, "_task-input", "2", "--plan-path", s.TasksDir); out != second {
			t.Fatalf("expected exactly task 2 Markdown, got %q", out)
		}
	})

	for _, tc := range []struct{ name, file, old, replacement, diagnostic string }{
		{"missing file number", "existing.md", "", "", "task filename"},
		{"numbering gap", "03-existing.md", "2", "3", "numbered 1..n"},
		{"duplicate number", "01-existing.md", "", "", "duplicate task number"},
		{"heading mismatch", "02-existing.md", "## Task 2:", "## Task 1:", "Task 2 heading"},
		{"extra heading", "02-existing.md", "", "\n### Task 99: Different task\n", "Task 2 heading"},
		{"duplicate fence", "02-existing.md", "", "\n```mulix-tests\ntasks: []\n```\n", "exactly one"},
		{"wrong declaration", "02-existing.md", "number: 2", "number: 1", "declare exactly task 2"},
		{"case owned by another task", "02-existing.md", "task: 2", "task: 1", "ownership"},
		{"case file from another change", "02-existing.md", "file: value_test.go", "file: .mulix/.runtime/sibling/tests/features/task-2/value.feature", "owned"},
		{"global duplicate case", "02-existing.md", "TC-002", "TC-001", "duplicate test case"},
		{"unknown criterion", "02-existing.md", "AC-001", "AC-999", "unknown criterion"},
		{"inexact Go identity", "02-existing.md", "fixture#TestExisting", "TestExisting", "exact Package#Test"},
		{"task without any case", "02-existing.md", "", "", "no executable case"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := chdirTemp(t)
			initMulixRoot(t, root)
			s := setupIssueQualityCLI(t, root, "issue", flow.PhaseTasks)
			body := issueTaskText(2, "Check Existing", "verification", "TestExisting")
			switch tc.name {
			case "duplicate number":
				body = issueTaskText(1, "Duplicate task", "verification", "TestExisting")
			case "numbering gap":
				body = issueTaskText(3, "Check Existing", "verification", "TestExisting")
			case "task without any case":
				prefix, _, _ := strings.Cut(body, "\ncases:")
				body = prefix + "\ncases: []\n```\n"
			default:
				if tc.old == "" {
					body += tc.replacement
				} else {
					body = strings.Replace(body, tc.old, tc.replacement, 1)
				}
			}
			if err := os.Remove(filepath.Join(root, s.TasksDir, "02-existing.md")); err != nil {
				t.Fatal(err)
			}
			qualityCLIFile(t, root, s.TasksDir+"/"+tc.file, body)
			if out, err := runPresetCLI(t, "state", "transition", "tasks-complete"); err == nil || !strings.Contains(out, tc.diagnostic) {
				t.Fatalf("wanted %q diagnostic, got %v:\n%s", tc.diagnostic, err, out)
			}
			s.Phase = flow.PhaseBuild // exercise the reader, not just its phase check
			saveStateForTest(t, root, s)
			if out, err := runPresetCLI(t, "_task-input", "1"); err == nil || !strings.Contains(err.Error(), tc.diagnostic) || out != "" {
				t.Fatalf("reader returned task 1 despite invalid task set: %v %q", err, out)
			}
		})
	}
}

func TestIssueTaskInputUsesSessionAndNestedRoot(t *testing.T) {
	outer, _ := sessionProject(t)
	outside := setupIssueQualityCLI(t, outer, "selected", flow.PhaseBuild)
	inner := filepath.Join(outer, "nested project")
	initMulixRoot(t, inner)
	t.Chdir(inner)
	qualityCommand(t, "context", "mode", "session")
	s := setupIssueQualityCLI(t, inner, "selected", flow.PhaseBuild)
	other := setupIssueQualityCLI(t, inner, "global", flow.PhaseBuild)
	qualityCommand(t, "state", "select", s.Change, "--session-id", "A")
	want := issueTaskText(1, "Nested and selected", "code", "TestValue")
	qualityCLIFile(t, inner, s.TasksDir+"/01-value.md", want)
	before, err := os.ReadFile(state.PathFor(inner, s.Change))
	if err != nil {
		t.Fatal(err)
	}
	qualityCLIFile(t, inner, "app/main.go", "package app\n")
	t.Chdir(filepath.Join(inner, "app"))
	if got := qualityCommand(t, "_task-input", "1", "--session-id", "A", "--plan-path", "../"+s.TasksDir); got != want {
		t.Fatalf("wrong effective task: %q", got)
	}
	for _, plan := range []string{filepath.Join(outer, outside.TasksDir), filepath.Join(inner, other.TasksDir)} {
		if out, err := runPresetCLI(t, "_task-input", "1", "--session-id", "A", "--plan-path", plan); err == nil || out != "" {
			t.Fatalf("accepted another plan %q: %v %q", plan, err, out)
		}
	}
	after, err := os.ReadFile(state.PathFor(inner, s.Change))
	if err != nil || string(before) != string(after) {
		t.Fatalf("task input changed state: %v", err)
	}
}

func TestIssueTaskInputAcceptsRelativePlanFromAliasedCWD(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	s := setupIssueQualityCLI(t, root, "issue", flow.PhaseBuild)
	want := issueTaskText(1, "Review from an aliased directory", "code", "TestValue")
	qualityCLIFile(t, root, s.TasksDir+"/01-value.md", want)

	alias := filepath.Join(t.TempDir(), "project-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	t.Chdir(alias)
	// On Unix Getwd can return the shell's logical spelling even though the
	// selected Mulix root was already resolved to its physical location.
	t.Setenv("PWD", alias)
	if got := qualityCommand(t, "_task-input", "1", "--plan-path", s.TasksDir); got != want {
		t.Fatalf("wrong task document through alias: %q", got)
	}
}

func setupSingleIssueCheck(t *testing.T, root string) flow.State {
	t.Helper()
	initMulixRoot(t, root)
	s := setupIssueQualityCLI(t, root, "issue", flow.PhaseBuild)
	if err := os.Remove(filepath.Join(root, s.TasksDir, "02-existing.md")); err != nil {
		t.Fatal(err)
	}
	qualityCLIFile(t, root, s.TasksDir+"/01-value.md", issueTaskText(1, "Check Value", "verification", "TestValue"))
	qualityCLIFile(t, root, "value.go", "package fixture\nfunc Value() int {return 2}\n")
	return s
}

func latestIssueCheck(t *testing.T, root string, s flow.State) quality.RunRecord {
	t.Helper()
	loaded, err := state.Load(root, s.Change)
	if err != nil {
		t.Fatal(err)
	}
	run, ok := quality.Latest(loaded.QualityRuns, 1, "check")
	if !ok || !run.Success {
		t.Fatalf("fixture lacks a successful CHECK: %+v", run)
	}
	return run
}

func TestIssueTaskClosureRequiresLatestReceipt(t *testing.T) {
	root := chdirTemp(t)
	s := setupSingleIssueCheck(t, root)
	const ledger = ".mulix/.runtime/issue/sdd/issues/progress.md"
	writeClosure := func(line string) {
		t.Helper()
		qualityCLIFile(t, root, ledger, "# SDD progress — plan: "+s.TasksDir+"\n"+line+"\n")
	}
	qualityCommand(t, "test", "run", "--task", "1", "--stage", "check")
	old := latestIssueCheck(t, root, s)
	writeClosure("- Task 1: complete (receipt " + old.ID + ")")

	// A valid revised plan with fresh execution evidence is still unclosed.
	qualityCLIFile(t, root, s.TasksDir+"/01-value.md", issueTaskText(1, "Revised Check", "verification", "TestValue"))
	qualityCommand(t, "test", "run", "--task", "1", "--stage", "check")
	fresh := latestIssueCheck(t, root, s)
	if out, err := runPresetCLI(t, "state", "transition", "build-complete"); err == nil || !strings.Contains(out, "plan-tasks-complete") || strings.Contains(out, "tdd-evidence-present") {
		t.Fatalf("fresh CHECK %s reused closure for %s: %v\n%s", fresh.ID, old.ID, err, out)
	}
	writeClosure("- Task 1: complete")
	if out, err := runPresetCLI(t, "state", "transition", "build-complete"); err == nil || !strings.Contains(out, "plan-tasks-complete") {
		t.Fatalf("unversioned native closure accepted: %v\n%s", err, out)
	}
	if out := qualityCommand(t, "test", "receipt", "--task", "1", "--plan-path", s.TasksDir, "--id-only"); out != fresh.ID+"\n" {
		t.Fatalf("receipt for selected plan = %q, want %s", out, fresh.ID)
	}
	otherPlan := t.TempDir()
	if out, err := runPresetCLI(t, "test", "receipt", "--task", "1", "--plan-path", otherPlan, "--id-only"); err == nil || !strings.Contains(err.Error(), "not the current change's task directory") || out != "" {
		t.Fatalf("receipt request for a foreign plan was not rejected: %q (%v)", out, err)
	}
	writeClosure("- Task 1: complete (receipt " + fresh.ID + ")")
	// Even another successful attempt at the same version needs closure.
	qualityCommand(t, "test", "run", "--task", "1", "--stage", "check")
	if out, err := runPresetCLI(t, "state", "transition", "build-complete"); err == nil || !strings.Contains(out, "plan-tasks-complete") {
		t.Fatalf("retry inherited the previous attempt's closure: %v\n%s", err, out)
	}
	current := latestIssueCheck(t, root, s)
	writeClosure("- Task 1: complete (receipt " + current.ID + ")")
	qualityCommand(t, "state", "transition", "build-complete")
}

func TestIssueTaskSetInvalidatesReceipt(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	s := setupIssueQualityCLI(t, root, "issue", flow.PhaseBuild)
	realTaskEvidence(t, root)
	qualityCommand(t, "test", "run", "--task", "2", "--stage", "check")
	green := strings.TrimSpace(qualityCommand(t, "test", "receipt", "--task", "1", "--id-only"))
	check := strings.TrimSpace(qualityCommand(t, "test", "receipt", "--task", "2", "--id-only"))
	closures := fmt.Sprintf("\n- Task 1: complete (receipt %s)\n- Task 2: complete (receipt %s)\n", green, check)
	workspace := ".mulix/.runtime/issue/sdd/issues"
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if got := read(workspace + "/plan-path"); got != s.TasksDir+"\n" {
		t.Fatalf("task reports and ledger disagree on plan identity: %q", got)
	}
	if !strings.Contains(read(workspace+"/task-1-report.md"), "Stage: green") {
		t.Fatal("native runner did not report the observed GREEN in its workspace")
	}
	ledger := workspace + "/progress.md"
	wrongPlan := filepath.Join(root, "outside", s.TasksDir)
	qualityCLIFile(t, root, ledger, "# SDD progress — plan: "+filepath.ToSlash(wrongPlan)+closures)
	if out, err := runPresetCLI(t, "state", "transition", "build-complete"); err == nil || !strings.Contains(out, "plan-tasks-complete") {
		t.Fatalf("another directory's ledger was accepted: %v %s", err, out)
	}
	qualityCLIFile(t, root, ledger, "# SDD progress — plan: "+s.TasksDir+closures)
	qualityCLIFile(t, root, s.TasksDir+"/.draft.md", "temporary draft\n")
	qualityCLIFile(t, root, s.TasksDir+"/01-value.md.bak", "editor backup\n")
	qualityCLIFile(t, root, workspace+"/task-1-tests.log", "temporary execution noise\n")
	qualityCommand(t, "test", "receipt", "--task", "1")

	first, second, third := s.TasksDir+"/01-value.md", s.TasksDir+"/02-existing.md", s.TasksDir+"/03-final.md"
	firstBody := read(first)
	secondBody := read(second)
	remove := func(rel string) {
		t.Helper()
		if err := os.Remove(filepath.Join(root, rel)); err != nil {
			t.Fatal(err)
		}
	}
	rename := func(from, to string) {
		t.Helper()
		if err := os.Rename(filepath.Join(root, from), filepath.Join(root, to)); err != nil {
			t.Fatal(err)
		}
	}
	mutations := []struct {
		name          string
		mutate, reset func()
	}{
		{"add", func() {
			qualityCLIFile(t, root, third, issueTaskText(3, "Add another task", "verification", "TestFinal"))
		}, func() { remove(third) }},
		{"delete", func() { remove(second) }, func() { qualityCLIFile(t, root, second, secondBody) }},
		{"rename", func() { rename(second, s.TasksDir+"/02-renamed.md") }, func() { rename(s.TasksDir+"/02-renamed.md", second) }},
		{"edit", func() { qualityCLIFile(t, root, second, secondBody+"\nA different approved input.\n") }, func() { qualityCLIFile(t, root, second, secondBody) }},
	}
	for _, m := range mutations {
		t.Run("task receipt "+m.name, func(t *testing.T) {
			m.mutate()
			defer m.reset()
			if _, err := runPresetCLI(t, "test", "receipt", "--task", "1"); err == nil || !strings.Contains(err.Error(), "contract") {
				t.Fatalf("old GREEN after %s: %v", m.name, err)
			}
			if _, err := runPresetCLI(t, "test", "receipt", "--task", "2"); err == nil {
				t.Fatalf("old CHECK after %s", m.name)
			}
		})
	}
	qualityCommand(t, "state", "transition", "build-complete")
	qualityCLIFile(t, root, s.ReportPath, "```mulix-review\nreviewer: review fixture reference\nresult: approved\nblocking: [remaining defect]\n```\n")
	if _, err := runPresetCLI(t, "verify"); err == nil || !strings.Contains(err.Error(), "no blocking findings") {
		t.Fatalf("verification without review approval: %v", err)
	}
	qualityCLIFile(t, root, s.ReportPath, "```mulix-review\nreviewer: review fixture reference\nresult: approved\nblocking: []\n```\n")
	qualityCommand(t, "verify")
	loaded, err := state.Load(root, s.Change)
	if err != nil {
		t.Fatal(err)
	}
	run, ok := quality.Latest(loaded.QualityRuns, 0, "verify")
	if !ok || !run.Success || loaded.ReportPath != s.ReportPath || len(run.Suites) != 2 || run.Suites[1].ID != "final" || run.Suites[1].Passed != 1 {
		t.Fatalf("missing final suite or pinned report: %+v, report %s", run, loaded.ReportPath)
	}
	report := read(s.ReportPath)
	for _, input := range []struct{ path, body string }{{s.SpecPath, read(s.SpecPath)}, {first, firstBody}, {second, secondBody}} {
		if !strings.Contains(report, input.path) || !strings.Contains(report, quality.Digest([]byte(input.body))) {
			t.Errorf("report lacks verified input path or digest for %s:\n%s", input.path, report)
		}
	}
	for _, receipt := range loaded.QualityRuns {
		if !strings.Contains(report, receipt.ID) {
			t.Errorf("report lacks receipt %s", receipt.ID)
		}
	}
	qualityCommand(t, "state", "transition", "verify-pass")
	qualityCommand(t, "state", "set", "archive_confirmation", "confirmed")
	p := loaded.QualityProject(root)
	for _, m := range mutations {
		t.Run("final receipt "+m.name, func(t *testing.T) {
			m.mutate()
			defer m.reset()
			c, err := quality.LoadContract(p)
			if err != nil {
				t.Fatalf("fixture mutation must leave a valid contract: %v", err)
			}
			current, err := quality.Fingerprint(p, c, 0)
			if err != nil || current.Contract == run.Snapshot.Contract || current.Source != run.Snapshot.Source {
				t.Fatalf("task mutation did not change only the contract inputs: %+v %v", current, err)
			}
			if err := quality.ValidateVerification(p, c, loaded.QualityRuns, s.ReportPath); err == nil {
				t.Fatal("old final receipt accepted independently of task receipt checks")
			}
			if out, err := runPresetCLI(t, "state", "transition", "archived"); err == nil || !strings.Contains(out, "verification-evidence-current") {
				t.Fatalf("old final report after %s: %v\n%s", m.name, err, out)
			}
		})
	}
	qualityCLIFile(t, root, s.ReportPath, report+"\nModified after execution.\n")
	if out, err := runPresetCLI(t, "state", "transition", "archived"); err == nil || !strings.Contains(out, "verification report changed after execution") {
		t.Fatalf("edited report accepted: %v\n%s", err, out)
	}
	qualityCLIFile(t, root, s.ReportPath, report)
	qualityCLIFile(t, root, ledger, read(ledger)+"\nA progress note unrelated to the verified inputs.\n")
	qualityCommand(t, "state", "transition", "archived")
	for _, rel := range []string{"docs/changes/issue", ".mulix/.runtime/issue/tasks.md", s.TasksDir + "/tasks.md"} {
		if _, err := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(err) {
			t.Errorf("native execution created a legacy artifact %s: %v", rel, err)
		}
	}
}
