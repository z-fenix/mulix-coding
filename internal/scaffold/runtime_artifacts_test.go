package scaffold

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRuntimeArtifactPolicy_StandardSddFilesOnly(t *testing.T) {
	for _, host := range []Host{HostClaude, HostDSH} {
		t.Run(string(host), func(t *testing.T) {
			root, skills := ledgerFixture(t, host)
			stateBefore := readProjectFile(t, root, ".mulix/.runtime/001-ledger/state.yaml")
			base := ledgerCommand(t, root, "git", "rev-parse", "HEAD")
			ledgerCommand(t, root, "bash", skills+"/executing-plans/scripts/task-start", ledgerPlan, "1")
			if err := os.WriteFile(filepath.Join(root, "implementation.txt"), []byte("implementation\n"), 0600); err != nil {
				t.Fatal(err)
			}
			ledgerCommand(t, root, "git", "add", "implementation.txt")
			ledgerCommand(t, root, "git", "-c", "user.name=ledger", "-c", "user.email=ledger@example.com", "commit", "-q", "-m", "implement fixture")
			ledgerCommand(t, root, "bash", skills+"/executing-plans/scripts/task-done", ledgerPlan, "1", base, "--", "printf", "pass\n")
			if err := os.WriteFile(filepath.Join(root, ledgerWorkspace, "task-1-report.md"), []byte("RED: fixture failing test before implementation\nGREEN: printf pass -> pass\n"), 0600); err != nil {
				t.Fatal(err)
			}
			out := ledgerCommand(t, root, "bash", skills+"/subagent-driven-development/scripts/review-package", ledgerPlan, base, "HEAD")
			diffs, err := filepath.Glob(filepath.Join(root, ledgerWorkspace, "review-*.diff"))
			if err != nil || len(diffs) != 1 || !strings.Contains(filepath.ToSlash(out), "/sdd/tasks/"+filepath.Base(diffs[0])) {
				t.Fatalf("review package was not produced at its reported path: %v %v %s", diffs, err, out)
			}
			if !strings.Contains(readProjectFile(t, root, filepath.Join(ledgerWorkspace, filepath.Base(diffs[0]))), "implementation.txt") {
				t.Fatal("review package omitted the actual change")
			}
			entries, err := os.ReadDir(filepath.Join(root, ledgerWorkspace))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, entry := range entries {
				got = append(got, entry.Name())
			}
			want := []string{"plan-path", "progress.md", "task-1-brief.md", "task-1-report.md", "task-1-tests.log", filepath.Base(diffs[0])}
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("nonstandard SDD artifacts: got %v, want %v", got, want)
			}
			if readProjectFile(t, root, ".mulix/.runtime/001-ledger/sdd/.gitignore") != "*\n" {
				t.Fatal("SDD workspace lost its native ignore file")
			}
			if after := readProjectFile(t, root, ".mulix/.runtime/001-ledger/state.yaml"); after != stateBefore {
				t.Fatal("execution helpers changed CLI-owned state")
			}
			entries, err = os.ReadDir(filepath.Join(root, ".mulix", ".runtime", "001-ledger"))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.IsDir() && entry.Name() != "sdd" && entry.Name() != "specs" {
					t.Fatalf("unexpected retained runtime directory: %s", entry.Name())
				}
			}
		})
	}
}

func TestRuntimeArtifactPolicy_UpdatePreservesLegacyRuntime(t *testing.T) {
	for _, host := range []Host{HostClaude, HostDSH} {
		t.Run(string(host), func(t *testing.T) {
			root, skills := ledgerFixture(t, host)
			ledgerCommand(t, root, "bash", skills+"/subagent-driven-development/scripts/sdd-workspace", ledgerPlan)
			history := map[string]string{
				".mulix/.runtime/.gitignore":                                       "# historical ignore file\r\n*/brainstorm/\r\n/_shared/\r\n",
				".mulix/.runtime/001-ledger/brainstorm/old/.last-token":            "private-old-token\n",
				".mulix/.runtime/001-ledger/verification/old.log":                  "historical verification\n",
				".mulix/.runtime/001-ledger/migrations/manifest.json":              "{\"historical\":true}\n",
				".mulix/.runtime/_shared/diagnosing-superpowers/session/report.md": "historical diagnosis\n",
				ledgerWorkspace + "/progress.md":                                   "# SDD ledger — plan: " + ledgerPlan + "\r\nTask 1: complete (legacy)\r\nRuling: retain original bytes",
			}
			for rel, text := range history {
				file := filepath.Join(root, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Init(InitOptions{Root: root, Hosts: []Host{host}}); err != nil {
				t.Fatal(err)
			}
			result, err := Update(UpdateOptions{Root: root})
			if err != nil || result.NeedsAttention() {
				t.Fatalf("update: %+v %v", result, err)
			}
			ledgerCommand(t, root, "bash", skills+"/subagent-driven-development/scripts/progress-init", ledgerPlan)
			for rel, want := range history {
				if got := readProjectFile(t, root, rel); got != want {
					t.Errorf("history changed in %s: %q != %q", rel, got, want)
				}
			}
			ledgerCommand(t, root, "git", "check-ignore", "-q", ".mulix/.runtime/001-ledger/brainstorm/old/.last-token")
			ledgerCommand(t, root, "git", "check-ignore", "-q", ".mulix/.runtime/_shared/diagnosing-superpowers/session/report.md")
		})
	}
}
