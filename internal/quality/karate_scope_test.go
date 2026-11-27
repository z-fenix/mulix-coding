package quality

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	karateA      = ".mulix/.runtime/001-x/tests/features/task-1/First check.feature"
	karateB      = ".mulix/.runtime/001-x/tests/features/task-2/second.feature"
	karateFinal  = ".mulix/.runtime/001-x/tests/features/final/integration.feature"
	karateHelper = ".mulix/.runtime/001-x/tests/helpers/math.js"
)

func karateFixture(t *testing.T) Project {
	t.Helper()
	p := fixture(t)
	plan := `# Plan
` + "```mulix-tests\n" + `tasks:
  - number: 1
    kind: code
  - number: 2
    kind: verification
    reason: check the existing endpoint without implementing it
cases:
  - id: TC-001
    criterion: AC-001
    task: 1
    point: value preservation
    given: a public input
    when: the operation executes
    then: the original value is preserved
    suite: karate
    test: '%s#保存成功'
    file: '%s'
  - id: TC-002
    criterion: AC-001
    task: 2
    point: another public input
    given: an existing result
    when: the result is fetched
    then: the same result is returned
    suite: karate
    test: '%s#second'
    file: '%s'
verify_cases:
  - id: TC-003
    criterion: AC-001
    suite: karate
    test: '%s#joined'
    file: '%s'
` + "```\n\n## Task 1: Implement\n\n## Task 2: Check\n"
	put(t, p.Root, p.TasksPath, fmt.Sprintf(plan, karateA, karateA, karateB, karateB, karateFinal, karateFinal))
	put(t, p.Root, PolicyPath, "schema: mulix.quality.v1\nsuites:\n  - id: karate\n    format: karate-jsonl\n    karate:\n      version: 2.1.5.RC1\n      sha256: "+strings.Repeat("a", 64)+"\n")
	for _, rel := range []string{karateA, karateB, karateFinal, karateHelper} {
		put(t, p.Root, rel, "fixture\n")
	}
	return p
}

func TestKarateContractScopesByTaskAndFinal(t *testing.T) {
	p := karateFixture(t)
	c, err := LoadContract(p)
	if err != nil {
		t.Fatal(err)
	}
	pol, err := LoadPolicy(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateMapping(c, pol); err != nil {
		t.Fatal(err)
	}
	if got := c.SelectedCases(1); len(got) != 1 || got[0].File != karateA {
		t.Fatalf("task selection includes unrelated cases: %+v", got)
	}
	if got := c.SelectedCases(0); len(got) != 3 || got[2].File != karateFinal {
		t.Fatalf("final-only case missing: %+v", got)
	}
}

func TestKarateFingerprintsKeepTaskInputsSeparate(t *testing.T) {
	p := karateFixture(t)
	c, err := LoadContract(p)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func(task int) Snapshot {
		t.Helper()
		s, err := Fingerprint(p, c, task)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	oldTask, oldFinal := snapshot(1), snapshot(0)
	put(t, p.Root, karateB, "task 2 changed\n")
	put(t, p.Root, karateFinal, "final changed\n")
	if snapshot(1) != oldTask || snapshot(0).Tests == oldFinal.Tests {
		t.Fatal("task scope changed or final scope failed to bind current tests")
	}
	put(t, p.Root, karateHelper, "shared code changed\n")
	if after := snapshot(1); after.Tests == oldTask.Tests || after.Source != oldTask.Source {
		t.Fatal("helper change was not bound separately from implementation")
	}
}

func TestKarateContractRejectsInvalidOwnership(t *testing.T) {
	for name, repl := range map[string]string{
		"other change":         ".mulix/.runtime/other/tests/features/task-1/case.feature",
		"another task":         karateB,
		"final entry":          karateFinal,
		"other runtime output": ".mulix/.runtime/001-x/sdd/error.feature",
		"helper feature":       ".mulix/.runtime/001-x/tests/helpers/data.feature",
	} {
		t.Run(name, func(t *testing.T) {
			p := karateFixture(t)
			plan, err := ReadProjectFile(p.Root, p.TasksPath)
			if err != nil {
				t.Fatal(err)
			}
			put(t, p.Root, p.TasksPath, strings.ReplaceAll(string(plan), karateA, repl))
			if _, err := LoadContract(p); err == nil {
				t.Fatalf("accepted a task case outside its ownership: %s", repl)
			}
		})
	}
}

func TestIssueTaskDocumentsAndKarateScopes(t *testing.T) {
	p := Project{Root: t.TempDir(), Change: "001-x", SpecPath: ".mulix/.runtime/001-x/spec.md", TasksDir: ".mulix/.runtime/001-x/issues"}
	spec := strings.TrimSuffix(specFixture, "```\n") + "  - id: AC-002\n    requirement: FR-001\n    expected: The persisted value can still be retrieved.\n```\n"
	put(t, p.Root, p.SpecPath, spec)
	put(t, p.Root, PolicyPath, "schema: mulix.quality.v1\nsuites:\n  - id: karate\n    format: karate-jsonl\n    karate:\n      version: 2.1.5.RC1\n      sha256: "+strings.Repeat("a", 64)+"\n")
	for _, rel := range []string{karateA, karateB, karateFinal, karateHelper} {
		put(t, p.Root, rel, "fixture\n")
	}
	file1, file2 := p.TasksDir+"/01-value.md", p.TasksDir+"/002-read.md"
	doc := func(n int, feature, scenario string) string {
		return fmt.Sprintf("## Task %d: A real check\n\n", n) + "```mulix-tests\n" + fmt.Sprintf(`tasks:
  - number: %[1]d
    kind: code
cases:
  - id: TC-%03[1]d
    criterion: AC-%03[1]d
    task: %[1]d
    point: literal value
    given: a configured fixture
    when: the action runs
    then: the expected result is returned
    suite: karate
    test: '%[2]s#%[3]s'
    file: '%[2]s'
`, n, feature, scenario)
	}
	body1 := doc(1, karateA, "保存成功") + "```\n"
	body2 := doc(2, karateB, "second") + fmt.Sprintf(`verify_cases:
  - id: TC-003
    criterion: AC-001
    task: 0
    suite: karate
    test: '%[1]s#joined'
    file: '%[1]s'
`, karateFinal) + "```\n"
	put(t, p.Root, file1, body1)
	put(t, p.Root, file2, body2)

	docs, err := ReadTaskDocuments(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || docs[0].Number != 1 || docs[0].Path != file1 || string(docs[0].Markdown) != body1 || docs[1].Number != 2 || docs[1].Path != file2 || string(docs[1].Markdown) != body2 {
		t.Fatalf("expected complete task documents in numeric, not lexical, order: %+v", docs)
	}
	c, err := LoadContract(p)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := LoadPolicy(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	for n, want := range map[int][]string{1: {karateA}, 2: {karateB}, 0: {karateFinal, karateA, karateB}} {
		sel, err := karateInputs(p, c, policy.Suites[0], n)
		if err != nil || !slices.Equal(sel.Features, want) || !slices.Equal(sel.Helpers, []string{karateHelper}) {
			t.Fatalf("task %d scope: %+v %v", n, sel, err)
		}
	}
	if final := c.SelectedCases(0); len(final) != 3 || final[2].Task != 0 || final[2].File != karateFinal {
		t.Fatalf("wrong final-only mapping: %+v", final)
	}

	t.Run("another change directory", func(t *testing.T) {
		borrowed := p
		borrowed.TasksDir = ".mulix/.runtime/other/issues"
		put(t, p.Root, borrowed.TasksDir+"/01-value.md", body1)
		put(t, p.Root, borrowed.TasksDir+"/002-read.md", body2)
		if docs, err := ReadTaskDocuments(borrowed); err == nil || len(docs) != 0 {
			t.Fatalf("accepted another change's task documents: %+v %v", docs, err)
		}
	})
	t.Run("linked task", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "outside.md")
		if err := os.WriteFile(outside, []byte(body2), 0600); err != nil {
			t.Fatal(err)
		}
		linked := filepath.Join(p.Root, file2)
		if err := os.Remove(linked); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, linked); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if docs, err := ReadTaskDocuments(p); err == nil || len(docs) != 0 {
			t.Fatalf("accepted linked task input: %+v %v", docs, err)
		}
	})
}
