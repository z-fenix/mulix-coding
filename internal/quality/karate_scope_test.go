package quality

import (
	"fmt"
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
