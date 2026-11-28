package scaffold

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/assets"
	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/guard"
	"github.com/z-fenix/mulix-coding/internal/quality"
	"github.com/z-fenix/mulix-coding/internal/state"
	"github.com/z-fenix/mulix-coding/internal/tracker"
)

const ledgerPlan = "docs/changes/001-ledger/tasks.md"
const ledgerWorkspace = ".mulix/.runtime/001-ledger/sdd/tasks"

func ledgerFixture(t *testing.T, host Host) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("ledger scripts require bash")
	}
	root := filepath.Join(t.TempDir(), "ledger project")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(InitOptions{Root: root, Hosts: []Host{host}}); err != nil {
		t.Fatal(err)
	}
	ledgerCommand(t, root, "git", "init", "-q")
	ledgerCommand(t, root, "git", "-c", "user.name=ledger", "-c", "user.email=ledger@example.com", "commit", "-q", "--allow-empty", "-m", "init")
	for rel, text := range map[string]string{
		".mulix/active": "001-ledger\n",
		ledgerPlan:      "# Plan\n\n## Task 1: First\n\n- [X] Verified step\n\n## Task 2: Second\n",
	} {
		file := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := flow.New("001-ledger", "")
	s.Phase = flow.PhaseBuild
	if err := state.Save(root, s); err != nil {
		t.Fatal(err)
	}
	skills := ".claude/skills"
	if host == HostDSH {
		skills = ".dsh/skills"
	}
	return root, skills
}

func ledgerCommand(t *testing.T, root, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Env = scriptEnv("")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestLedgerInit_SectionedMarkdown(t *testing.T) {
	for _, host := range []Host{HostClaude, HostDSH} {
		t.Run(string(host), func(t *testing.T) {
			root, skills := ledgerFixture(t, host)
			init := skills + "/subagent-driven-development/scripts/progress-init"
			ledgerCommand(t, root, "bash", init, ledgerPlan)
			got := readProjectFile(t, root, ledgerWorkspace+"/progress.md")
			if !strings.HasPrefix(got, "# SDD progress — plan: "+ledgerPlan+"\n\n") {
				t.Fatalf("missing ledger identity and blank line:\n%s", got)
			}
			previous := -1
			for _, heading := range []string{"## Execution context", "## Pre-flight", "## Task progress"} {
				i := strings.Index(got, "\n"+heading+"\n\n")
				if i <= previous {
					t.Fatalf("missing or unordered section %q:\n%s", heading, got)
				}
				previous = i
			}
			if done := guard.CompletedTasks(got); len(done) != 0 {
				t.Fatalf("empty ledger claimed completed tasks: %v", done)
			}
			ledgerCommand(t, root, "bash", init, ledgerPlan)
			if again := readProjectFile(t, root, ledgerWorkspace+"/progress.md"); again != got {
				t.Fatal("reinitialization changed the existing ledger")
			}
		})
	}
}

func TestLedgerInit_PreservesExistingHistory(t *testing.T) {
	for _, history := range []string{
		"# SDD ledger — plan: " + ledgerPlan + "\r\nTask 1: complete (legacy)\r\nRuling: keep history",
		"# SDD ledger — plan: " + ledgerPlan + "\n\n## Task progress\n\n- Task 1: complete\n",
	} {
		root, skills := ledgerFixture(t, HostClaude)
		ledgerCommand(t, root, "bash", skills+"/subagent-driven-development/scripts/sdd-workspace", ledgerPlan)
		file := filepath.Join(root, ledgerWorkspace, "progress.md")
		if err := os.WriteFile(file, []byte(history), 0o644); err != nil {
			t.Fatal(err)
		}
		ledgerCommand(t, root, "bash", skills+"/subagent-driven-development/scripts/progress-init", ledgerPlan)
		if got := readProjectFile(t, root, ledgerWorkspace+"/progress.md"); got != history {
			t.Fatalf("existing ledger was rewritten:\n%q\nwant:\n%q", got, history)
		}
	}
}

func TestTaskDone_SectionedCompletionAndEvidence(t *testing.T) {
	root, skills := ledgerFixture(t, HostClaude)
	base := ledgerCommand(t, root, "git", "rev-parse", "HEAD")
	done := skills + "/executing-plans/scripts/task-done"
	ledgerCommand(t, root, "bash", done, ledgerPlan, "1", base, "--", "printf", "ok `quoted` | <pass>\n")
	got := readProjectFile(t, root, ledgerWorkspace+"/progress.md")
	for _, text := range []string{
		"\n## Task progress\n\n",
		"\n### Task 1\n\n- Task 1: complete\n",
		"  - Commits: ",
		"  - Tests: ",
		"  - Result: `` ok `quoted` | <pass> ``\n",
		"  - Evidence: [test log](task-1-tests.log)\n",
	} {
		if !strings.Contains(got, text) {
			t.Errorf("ledger missing %q:\n%s", text, got)
		}
	}
	if done := guard.CompletedTasks(got); !done[1] || done[2] || len(done) != 1 {
		t.Fatalf("completion parser got %v, want only Task 1", done)
	}
	if log := readProjectFile(t, root, ledgerWorkspace+"/task-1-tests.log"); log != "ok `quoted` | <pass>\n" {
		t.Fatalf("test log changed: %q", log)
	}
	if brief := readProjectFile(t, root, ledgerWorkspace+"/task-1-brief.md"); !strings.Contains(brief, "- [X] Verified step") {
		t.Fatalf("brief was not refreshed:\n%s", brief)
	}
}

func TestTaskDone_AppendsWithoutRewritingLegacyLedger(t *testing.T) {
	root, skills := ledgerFixture(t, HostClaude)
	base := ledgerCommand(t, root, "git", "rev-parse", "HEAD")
	ledgerCommand(t, root, "bash", skills+"/subagent-driven-development/scripts/sdd-workspace", ledgerPlan)
	history := "# SDD ledger — plan: " + ledgerPlan + "\r\nTask 1: complete (legacy)\r\nRuling: leave unchanged"
	if err := os.WriteFile(filepath.Join(root, ledgerWorkspace, "progress.md"), []byte(history), 0o644); err != nil {
		t.Fatal(err)
	}
	ledgerCommand(t, root, "bash", skills+"/executing-plans/scripts/task-done", ledgerPlan, "2", base, "--", "true")
	got := readProjectFile(t, root, ledgerWorkspace+"/progress.md")
	if !strings.HasPrefix(got, history+"\n\n### Task 2\n\n- Task 2: complete\n") {
		t.Fatalf("legacy history changed or new section not separated:\n%q", got)
	}
	if done := guard.CompletedTasks(got); !done[1] || !done[2] {
		t.Fatalf("mixed-format ledger lost completions: %v", done)
	}
	if !strings.Contains(got, "  - Result: ` (no output) `\n") {
		t.Fatalf("missing no-output result:\n%s", got)
	}
}

func TestTaskDone_AppendsToCurrentTaskSection(t *testing.T) {
	root, skills := ledgerFixture(t, HostClaude)
	base := ledgerCommand(t, root, "git", "rev-parse", "HEAD")
	ledgerCommand(t, root, "bash", skills+"/subagent-driven-development/scripts/progress-init", ledgerPlan)
	file := filepath.Join(root, ledgerWorkspace, "progress.md")
	history := readProjectFile(t, root, ledgerWorkspace+"/progress.md") + "\n### Task 1\n\n- Task 1: Ruling: commit not authorized.\n"
	if err := os.WriteFile(file, []byte(history), 0o644); err != nil {
		t.Fatal(err)
	}
	ledgerCommand(t, root, "bash", skills+"/executing-plans/scripts/task-done", ledgerPlan, "1", base, "--", "true")
	got := readProjectFile(t, root, ledgerWorkspace+"/progress.md")
	if !strings.HasPrefix(got, history) || strings.Count(got, "\n### Task 1\n") != 1 {
		t.Fatalf("existing task section was changed or duplicated:\n%s", got)
	}
	if !guard.CompletedTasks(got)[1] {
		t.Fatal("completion was not recorded under the existing task")
	}
}

func TestTaskDone_ResumesAfterFinalReview(t *testing.T) {
	root, skills := ledgerFixture(t, HostClaude)
	base := ledgerCommand(t, root, "git", "rev-parse", "HEAD")
	ledgerCommand(t, root, "bash", skills+"/subagent-driven-development/scripts/progress-init", ledgerPlan)
	history := readProjectFile(t, root, ledgerWorkspace+"/progress.md") + "\n## Final review\n\n### Task 2\n\n- Final: Task 2 needs a fix.\n"
	if err := os.WriteFile(filepath.Join(root, ledgerWorkspace, "progress.md"), []byte(history), 0o644); err != nil {
		t.Fatal(err)
	}
	ledgerCommand(t, root, "bash", skills+"/executing-plans/scripts/task-done", ledgerPlan, "2", base, "--", "true")
	got := readProjectFile(t, root, ledgerWorkspace+"/progress.md")
	if !strings.HasPrefix(got, history) || !strings.Contains(got, "\n## Task progress (continued)\n\n### Task 2\n\n- Task 2: complete\n") {
		t.Fatalf("resumed completion ended up inside final review:\n%s", got)
	}
}

func TestLedgerOverlay_EmbeddedCopiesMatch(t *testing.T) {
	overlay := os.DirFS(filepath.Join("..", "..", "scripts", "superpowers-overrides"))
	if err := fs.WalkDir(overlay, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		source, err := fs.ReadFile(overlay, path)
		if err != nil {
			return err
		}
		bundled, err := assets.Superpowers.ReadFile("superpowers/skills/" + path)
		if err != nil {
			return err
		}
		if !bytes.Equal(source, bundled) {
			t.Errorf("embedded %s is stale; refresh it from scripts/superpowers-overrides", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTaskDone_FailureDoesNotCreateLedger(t *testing.T) {
	root, skills := ledgerFixture(t, HostClaude)
	base := ledgerCommand(t, root, "git", "rev-parse", "HEAD")
	cmd := exec.Command("bash", skills+"/executing-plans/scripts/task-done", ledgerPlan, "1", base, "--", "bash", "-c", "printf 'expected failure\\n'; exit 7")
	cmd.Dir = root
	cmd.Env = scriptEnv("")
	out, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 {
		t.Fatalf("got %v, want exit 7:\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, ledgerWorkspace, "progress.md")); !os.IsNotExist(err) {
		t.Fatalf("failed tests must not create a ledger: %v", err)
	}
	if log := readProjectFile(t, root, ledgerWorkspace+"/task-1-tests.log"); log != "expected failure\n" {
		t.Fatalf("failed test evidence not retained: %q", log)
	}
}

func TestIssueLayoutLedger(t *testing.T) {
	repo, skills := ledgerFixture(t, HostClaude)
	root := filepath.Join(repo, "nested project")
	s := flow.New("native", "")
	s.Phase = flow.PhaseBuild
	s.ArtifactLayout = flow.RuntimeIssuesLayout
	s.Tracker = &tracker.ChangeBinding{Target: tracker.Target{Kind: "markdown"}}
	const runtime = ".mulix/.runtime/native"
	s.SpecPath, s.TasksDir, s.ReportPath = runtime+"/spec.md", runtime+"/issues", runtime+"/report.md"
	task := func(n, tc, title, name string) string {
		return "## Task " + n + ": " + title + "\n\n" + "```mulix-tests\n" + `tasks:
  - number: ` + n + `
    kind: documentation
    reason: Confirm the documented behavior already works.
cases:
  - id: ` + tc + `
    criterion: AC-001
    task: ` + n + `
    point: Literal result
    given: A fixture module
    when: The function runs
    then: The result is two
    suite: unit
    test: fixture#` + name + `
    file: value_test.go
` + "```\n"
	}
	first := task("1", "TC-001", "Document Value", "TestValue") + "\n- [ ] **Step 1: Inspect Value**\n"
	second := task("2", "TC-002", "Document Caller", "TestCaller") + "\n~~~~markdown\n## Task 55: Example only\n~~~\n~~~~\n"
	for rel, data := range map[string]string{
		s.SpecPath:                   "# Spec\nFR-001: Return two.\n```mulix-acceptance\ncriteria:\n  - id: AC-001\n    requirement: FR-001\n    expected: Value returns two.\n```\n",
		s.TasksDir + "/01-value.md":  first,
		s.TasksDir + "/02-caller.md": second,
		".mulix/quality.yaml":        "schema: mulix.quality.v1\nsuites:\n  - id: unit\n    format: go-json\n    cwd: .\n    command: [go, test, -json, -count=1, .]\n",
		"go.mod":                     "module fixture\n\ngo 1.27.0\n",
		"value.go":                   "package fixture\nfunc Value() int { return 2 }\n",
		"value_test.go":              "package fixture\nimport \"testing\"\nfunc TestValue(t *testing.T) { if Value()!=2 { t.Fatal(\"wrong value\") } }\nfunc TestCaller(t *testing.T) { if Value()+1!=3 { t.Fatal(\"wrong total\") } }\n",
		"app/source.go":              "package app\n",
	} {
		writeProjectFile(t, root, rel, data)
	}
	if err := state.Save(root, s); err != nil {
		t.Fatal(err)
	}
	if err := state.SetActive(root, s.Change); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(root, "app")
	sdd := filepath.ToSlash(filepath.Join(repo, skills, "subagent-driven-development", "scripts"))
	inline := filepath.ToSlash(filepath.Join(repo, skills, "executing-plans", "scripts"))
	plan := "../" + s.TasksDir
	const ws = runtime + "/sdd/issues"
	out := ledgerCommand(t, cwd, "bash", sdd+"/sdd-workspace", plan)
	gotDir, gotErr := os.Stat(out)
	wantDir, wantErr := os.Stat(filepath.Join(root, ws))
	if gotErr != nil || wantErr != nil || !os.SameFile(gotDir, wantDir) {
		t.Fatalf("workspace is not in the effective Mulix root: %s (%v; %v)", out, gotErr, wantErr)
	}
	if marker := readProjectFile(t, root, ws+"/plan-path"); marker != s.TasksDir+"\n" {
		t.Fatalf("wrong workspace marker: %q", marker)
	}
	ledgerCommand(t, cwd, "bash", inline+"/task-start", plan, "2")
	if brief := readProjectFile(t, root, ws+"/task-2-brief.md"); brief != second {
		t.Fatalf("brief is not exactly task 2:\n%s", brief)
	}
	fail := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir, cmd.Env = cwd, scriptEnv("")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("unexpected success: %v\n%s", args, out)
		}
		return string(out)
	}
	base := ledgerCommand(t, cwd, "git", "rev-parse", "HEAD")
	// With no additional authorized fixture commit yet, a directory plan
	// must reach the usual range guard instead of being rejected as a file.
	if out := fail("bash", sdd+"/review-package", plan, base, base); !strings.Contains(out, "empty commit range") {
		t.Fatalf("native plan did not reach review range checks: %s", out)
	}
	done := []string{"bash", inline + "/task-done", plan, "1", base, "--"}
	receipt := []string{"mulix", "test", "receipt", "--task", "1"}
	fail(append(done, "true")...)
	if out := fail(append(done, receipt...)...); !strings.Contains(out, "lacks runner-generated check") {
		t.Fatalf("missing evidence did not block completion: %s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ws, "progress.md")); !os.IsNotExist(err) {
		t.Fatalf("failed receipt check created ledger completion: %v", err)
	}
	ledgerCommand(t, cwd, sessionTestBin, "test", "run", "--task", "1", "--stage", "check")
	if report := readProjectFile(t, root, ws+"/task-1-report.md"); !strings.Contains(report, "Stage: check") {
		t.Fatal("runner did not use executor's plan workspace")
	}
	writeProjectFile(t, root, "value.go", "package fixture\n// changed after check\nfunc Value() int { return 2 }\n")
	if out := fail(append(done, receipt...)...); !strings.Contains(out, "receipt is stale") {
		t.Fatalf("stale receipt did not block completion: %s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ws, "progress.md")); !os.IsNotExist(err) {
		t.Fatalf("stale receipt created a ledger: %v", err)
	}
	ledgerCommand(t, cwd, sessionTestBin, "test", "run", "--task", "1", "--stage", "check")

	// Force an otherwise valid definition edit during brief refresh. Closure
	// must use the receipt after that refresh, rather than an earlier check.
	drift := `binary=$1
input=$2
shift 2
export binary input
mulix() {
  if [ "$1" = _task-input ]; then printf '\nChanged during brief refresh.\n' >> "$input"; fi
  "$binary" "$@"
}
export -f mulix
exec bash "$@"`
	firstPath := filepath.ToSlash(filepath.Join(root, s.TasksDir, "01-value.md"))
	args := append([]string{"-c", drift, "receipt-fixture", filepath.ToSlash(sessionTestBin), firstPath}, done[1:]...)
	cmd := exec.Command("bash", append(args, receipt...)...)
	cmd.Dir, cmd.Env = cwd, scriptEnv("")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "contract inputs") {
		t.Fatalf("accepted a receipt from before the brief changed: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, ws, "progress.md")); !os.IsNotExist(err) {
		t.Fatalf("drift during brief refresh created a ledger: %v", err)
	}
	writeProjectFile(t, root, s.TasksDir+"/01-value.md", first)
	before := readProjectFile(t, root, runtime+"/state.yaml")
	ledgerCommand(t, cwd, done[0], append(done[1:], receipt...)...)
	if after := readProjectFile(t, root, runtime+"/state.yaml"); after != before {
		t.Fatal("task closure reran suites or mutated receipts")
	}
	ledger := readProjectFile(t, root, ws+"/progress.md")
	saved, err := state.Load(root, s.Change)
	if err != nil {
		t.Fatal(err)
	}
	check, ok := quality.Latest(saved.QualityRuns, 1, "check")
	if !ok || !strings.Contains(ledger, "\n- Task 1: complete (receipt "+check.ID+")\n") {
		t.Fatalf("native executor did not write current CHECK ID %s:\n%s", check.ID, ledger)
	}
	if !strings.HasPrefix(ledger, "# SDD progress — plan: "+s.TasksDir+"\n") || !guard.CompletedTasks(ledger)[1] || guard.CompletedTasks(ledger)[2] {
		t.Fatalf("wrong plan or task closed:\n%s", ledger)
	}
	if readProjectFile(t, root, s.TasksDir+"/01-value.md") != first || readProjectFile(t, root, s.TasksDir+"/02-caller.md") != second {
		t.Fatal("closing a task rewrote approved prose")
	}
	if got := readProjectFile(t, root, ws+"/task-1-brief.md"); got != first {
		t.Fatalf("task completion did not refresh the correct brief: %q", got)
	}

	// The marker is an exact identity, not a shell string with NULs stripped.
	writeProjectFile(t, root, ws+"/plan-path", s.TasksDir+"\x00\n")
	fail("bash", sdd+"/sdd-workspace", plan)
	fail(sessionTestBin, "test", "run", "--task", "1", "--stage", "check")

	// Legacy fallback must not adopt native history without an exact marker.
	if err := os.Remove(filepath.Join(root, ws, "plan-path")); err != nil {
		t.Fatal(err)
	}
	fail("bash", sdd+"/sdd-workspace", plan)
	fail(sessionTestBin, "test", "run", "--task", "1", "--stage", "check")
	if after := readProjectFile(t, root, ws+"/progress.md"); after != ledger {
		t.Fatal("unbound workspace history was overwritten")
	}
	if _, err := os.Stat(filepath.Join(root, ws, "plan-path")); !os.IsNotExist(err) {
		t.Fatalf("native plan adopted a markerless legacy workspace: %v", err)
	}

	// Neither an existing legacy plan nor a same-suffix directory elsewhere
	// in the repository can be used as the effective native plan.
	legacy := filepath.ToSlash(filepath.Join(repo, ledgerPlan))
	fail("bash", sdd+"/sdd-workspace", legacy)
	fail("bash", sdd+"/task-brief", legacy, "1", "unapproved.md")
	borrowed := filepath.ToSlash(filepath.Join(repo, s.TasksDir))
	writeProjectFile(t, repo, s.TasksDir+"/01-value.md", first)
	fail("bash", sdd+"/sdd-workspace", borrowed)
}

func issueExecutorFixture(t *testing.T) (string, string, flow.State) {
	t.Helper()
	root, skills := ledgerFixture(t, HostClaude)
	s := flow.New("native", "")
	s.Phase = flow.PhaseBuild
	s.ArtifactLayout = flow.RuntimeIssuesLayout
	s.Tracker = &tracker.ChangeBinding{Target: tracker.Target{Kind: "markdown"}}
	const dir = ".mulix/.runtime/native"
	s.SpecPath, s.TasksDir, s.ReportPath = dir+"/spec.md", dir+"/issues", dir+"/report.md"
	if err := os.MkdirAll(filepath.Join(root, s.TasksDir), 0700); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(root, s); err != nil {
		t.Fatal(err)
	}
	if err := state.SetActive(root, s.Change); err != nil {
		t.Fatal(err)
	}
	return root, skills + "/subagent-driven-development/scripts", s
}

func TestIssueLayoutWorkspaceUsesSingleContext(t *testing.T) {
	root, scripts, s := issueExecutorFixture(t)
	writeProjectFile(t, root, "issues.md", "# Legacy plan with a colliding workspace slug\n")
	// Switch selection immediately after the first context projection. A
	// native snapshot must still refuse the legacy file, regardless of the
	// active change by the time workspace setup gets the line record.
	switchContext := `binary=$1
shift
export binary
mulix() {
  "$binary" "$@" || return
  if [ "$1" = context ] && { [ "$4" = runtime-dir ] || [ "$4" = sdd-context ]; }; then
    "$binary" state select 001-ledger > /dev/null
  fi
}
export -f mulix
exec bash "$@"`
	cmd := exec.Command("bash", "-c", switchContext, "context-fixture", filepath.ToSlash(sessionTestBin), scripts+"/sdd-workspace", "issues.md")
	cmd.Dir, cmd.Env = root, scriptEnv("")
	out, err := cmd.CombinedOutput()
	if active, e := state.Active(root); e != nil || active != "001-ledger" {
		t.Fatalf("fixture did not change selection after lookup: %q %v, %s", active, e, out)
	}
	if err == nil {
		t.Fatalf("legacy file claimed native workspace across a selection switch: %s", out)
	}
	if _, err := os.Lstat(filepath.Join(root, ".mulix/.runtime/native/sdd/issues")); !os.IsNotExist(err) {
		t.Fatalf("selection switch created a native workspace: %v", err)
	}
	if err := state.SetActive(root, s.Change); err != nil {
		t.Fatal(err)
	}
	ledgerCommand(t, root, "bash", scripts+"/sdd-workspace", s.TasksDir)
}

func TestIssueLayoutWorkspaceUsesPortableMarkerInput(t *testing.T) {
	root, scripts, s := issueExecutorFixture(t)
	// Run the real installed helper and comparator. The external command is
	// given an environment that cannot open Bash's process-substitution paths.
	portable := `cmp() {
  for arg in "$@"; do
    case "$arg" in
      /dev/fd/*|/proc/*/fd/*) return 2 ;;
    esac
  done
  command cmp "$@"
}
export -f cmp
exec bash "$@"`
	ledgerCommand(t, root, "bash", "-c", portable, "marker-fixture", scripts+"/sdd-workspace", s.TasksDir)
	if got := readProjectFile(t, root, ".mulix/.runtime/native/sdd/issues/plan-path"); got != s.TasksDir+"\n" {
		t.Fatalf("workspace has the wrong ownership marker: %q", got)
	}
}

func TestIssueLayoutReviewRefusesLinkedOutput(t *testing.T) {
	root, scripts, s := issueExecutorFixture(t)
	base := ledgerCommand(t, root, "git", "rev-parse", "--short", "HEAD")
	ledgerCommand(t, root, "git", "-c", "user.name=ledger", "-c", "user.email=ledger@example.com", "commit", "-q", "--allow-empty", "-m", "review fixture candidate")
	head := ledgerCommand(t, root, "git", "rev-parse", "--short", "HEAD")
	ws := ledgerCommand(t, root, "bash", scripts+"/sdd-workspace", s.TasksDir)
	out := filepath.Join(ws, "review-"+base+".."+head+".diff")
	const reviewed = "# Approved spec bytes\n"
	writeProjectFile(t, root, s.SpecPath, reviewed)
	if err := os.Symlink(filepath.Join(root, s.SpecPath), out); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	cmd := exec.Command("bash", scripts+"/review-package", s.TasksDir, base, head)
	cmd.Dir, cmd.Env = root, scriptEnv("")
	log, err := cmd.CombinedOutput()
	if actual := readProjectFile(t, root, s.SpecPath); err == nil || actual != reviewed {
		t.Fatalf("linked native review output was not refused (spec intact=%v): %v\n%s", actual == reviewed, err, log)
	}
	if err := os.Remove(out); err != nil {
		t.Fatal(err)
	}
	ledgerCommand(t, root, "bash", scripts+"/review-package", s.TasksDir, base, head)
	if data, err := os.ReadFile(out); err != nil || !strings.Contains(string(data), "review fixture candidate") {
		t.Fatalf("ordinary review output was not generated: %v\n%s", err, data)
	}
	if err := state.SetActive(root, "001-ledger"); err != nil {
		t.Fatal(err)
	}
	ledgerCommand(t, root, "bash", scripts+"/review-package", ledgerPlan, base, head, "legacy-review.diff")
	if got := readProjectFile(t, root, "legacy-review.diff"); !strings.Contains(got, "review fixture candidate") {
		t.Fatalf("legacy explicit output changed:\n%s", got)
	}
}
