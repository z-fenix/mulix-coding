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
	"github.com/z-fenix/mulix-coding/internal/guard"
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
