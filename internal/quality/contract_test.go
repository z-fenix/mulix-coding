package quality

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const specFixture = "# Example\n\n- FR-001: Preserve the requested value.\n\n```mulix-acceptance\ncriteria:\n  - id: AC-001\n    requirement: FR-001\n    expected: The requested value is returned unchanged.\n```\n"
const planFixture = "# Plan\n\n```mulix-tests\ntasks:\n  - number: 1\n    kind: code\ncases:\n  - id: TC-001\n    criterion: AC-001\n    task: 1\n    point: exact value preservation\n    given: a literal input\n    when: the public operation runs\n    then: the returned value equals the literal input\n    suite: unit\n    test: fixture#TestValue\n    file: value_test.go\n```\n\n## Task 1: Preserve value\n- [ ] **Step 1: Write a test**\n"
const policyFixture = "schema: mulix.quality.v1\nsuites:\n  - id: unit\n    command: [go, test, -json, -count=1, ./...]\n    cwd: .\n    format: go-json\n    timeout_seconds: 30\n"

func put(t *testing.T, root, rel, data string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) Project {
	t.Helper()
	p := Project{Root: t.TempDir(), Change: "001-x", SpecPath: "docs/specs/001-x/spec.md", TasksPath: "docs/changes/001-x/tasks.md"}
	put(t, p.Root, p.SpecPath, specFixture)
	put(t, p.Root, p.TasksPath, planFixture)
	put(t, p.Root, ".mulix/quality.yaml", policyFixture)
	put(t, p.Root, "value_test.go", "package fixture\n// independently derived test fixture\n")
	put(t, p.Root, "value.go", "package fixture\n")
	return p
}
func TestContractTraceability(t *testing.T) {
	p := fixture(t)
	c, err := LoadContract(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Criteria) != 1 || len(c.Tasks) != 1 || len(c.Cases) != 1 || c.Cases[0].Then != "the returned value equals the literal input" {
		t.Fatalf("contract = %+v", c)
	}
}
func TestContractRejectsIncompleteAndAmbiguousMappings(t *testing.T) {
	tests := []struct{ name, spec, plan string }{
		{"acceptance only in long fence", "# Spec\n````mulix-acceptance\ncriteria:\n  - id: AC-001\n    requirement: FR-001\n    expected: Return the literal value.\n````\n", planFixture},
		{"acceptance only in tilde fence", "# Spec\n~~~mulix-acceptance\ncriteria:\n  - id: AC-001\n    requirement: FR-001\n    expected: Return the literal value.\n~~~\n", planFixture},
		{"excluded mapped file", specFixture, strings.Replace(planFixture, "file: value_test.go", "file: .mulix/.runtime/001-x/report.md", 1)},
		{"missing acceptance", "# Spec\nFR-001: Value\n", planFixture},
		{"duplicate fence", specFixture + "\n```mulix-acceptance\ncriteria: []\n```\n", planFixture},
		{"unmapped requirement", specFixture + "\nFR-002: Also preserve errors.\n", planFixture},
		{"unknown criterion", specFixture, strings.Replace(planFixture, "criterion: AC-001", "criterion: AC-999", 1)},
		{"unknown task", specFixture, strings.Replace(planFixture, "task: 1", "task: 2", 1)},
		{"heading mismatch", specFixture, strings.Replace(planFixture, "## Task 1:", "## Task 2:", 1)},
		{"missing expected result", strings.Replace(specFixture, "The requested value is returned unchanged.", "", 1), planFixture},
		{"missing independent expectation", specFixture, strings.Replace(planFixture, "then: the returned value equals the literal input", "then: TBD", 1)},
		{"unreasoned exemption", specFixture, strings.Replace(planFixture, "kind: code", "kind: documentation", 1)},
		{"unknown field", strings.Replace(specFixture, "criteria:", "unknown: true\ncriteria:", 1), planFixture},
		{"trailing YAML document", strings.Replace(specFixture, "```\n", "---\ncriteria: []\n```\n", 1), planFixture},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := fixture(t)
			put(t, p.Root, p.SpecPath, tt.spec)
			put(t, p.Root, p.TasksPath, tt.plan)
			if _, err := LoadContract(p); err == nil {
				t.Fatal("invalid contract accepted")
			}
		})
	}
}
func TestPolicyRejectsInvalidExecutionDefinitions(t *testing.T) {
	for name, policy := range map[string]string{
		"empty suites":    "schema: mulix.quality.v1\nsuites: []\n",
		"empty argv":      strings.Replace(policyFixture, "[go, test, -json, -count=1, ./...]", "[]", 1),
		"unknown parser":  strings.Replace(policyFixture, "go-json", "exit-only", 1),
		"outside cwd":     strings.Replace(policyFixture, "cwd: .", "cwd: ../", 1),
		"unknown field":   policyFixture + "allow_skips: true\n",
		"invalid timeout": strings.Replace(policyFixture, "timeout_seconds: 30", "timeout_seconds: -1", 1),
	} {
		t.Run(name, func(t *testing.T) {
			p := fixture(t)
			put(t, p.Root, ".mulix/quality.yaml", policy)
			if _, err := LoadPolicy(p.Root); err == nil {
				t.Fatal("invalid policy accepted")
			}
		})
	}
}
func TestFingerprintChangesAndCheckboxNormalization(t *testing.T) {
	p := fixture(t)
	c, err := LoadContract(p)
	if err != nil {
		t.Fatal(err)
	}
	before, err := Fingerprint(p, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	put(t, p.Root, p.TasksPath, strings.Replace(planFixture, "[ ] **Step", "[X] **Step", 1))
	after, err := Fingerprint(p, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("status-only checkbox edit invalidated the snapshot")
	}
	for _, tt := range []struct{ name, rel, data string }{
		{"source", "value.go", "package fixture\n// changed\n"},
		{"test", "value_test.go", "package fixture\n// changed assertion\n"},
		{"policy", ".mulix/quality.yaml", policyFixture + "# reviewed policy change\n"},
		{"spec", p.SpecPath, specFixture + "\nAdditional business constraint.\n"},
		{"plan", p.TasksPath, planFixture + "\nAdditional task constraint.\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			old, err := os.ReadFile(filepath.Join(p.Root, tt.rel))
			if err != nil {
				t.Fatal(err)
			}
			put(t, p.Root, tt.rel, tt.data)
			changed, err := Fingerprint(p, c, 1)
			if err != nil {
				t.Fatal(err)
			}
			if changed == before {
				t.Fatal("input mutation did not invalidate snapshot")
			}
			put(t, p.Root, tt.rel, string(old))
		})
	}
}
func TestFingerprintExcludesOnlyWorkflowOutputAndBindsProject(t *testing.T) {
	p := fixture(t)
	c, _ := LoadContract(p)
	before, err := Fingerprint(p, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".mulix/.runtime/001-x/state.yaml", ".mulix/.locks/change.lock", ".mulix/sessions/x.json", ".mulix/.installed/template.md", "docs/changes/001-x/report.md"} {
		put(t, p.Root, rel, "runtime output")
	}
	after, err := Fingerprint(p, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("workflow output changed source identity")
	}
	other := fixture(t)
	otherContract, _ := LoadContract(other)
	different, err := Fingerprint(other, otherContract, 1)
	if err != nil {
		t.Fatal(err)
	}
	if before.Project == different.Project {
		t.Fatal("receipt can be reused in another project")
	}
}
func TestFingerprintGitIncludesDirtyUntrackedAndDeletedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	p := fixture(t)
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = p.Root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	c, _ := LoadContract(p)
	before, err := Fingerprint(p, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	put(t, p.Root, "new.go", "package fixture\n")
	untracked, err := Fingerprint(p, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	if before.Source == untracked.Source {
		t.Fatal("untracked source ignored")
	}
	if err := os.Remove(filepath.Join(p.Root, "new.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(p.Root, "value.go")); err != nil {
		t.Fatal(err)
	}
	deleted, err := Fingerprint(p, c, 1)
	if err != nil {
		t.Fatal(err)
	}
	if before.Source == deleted.Source {
		t.Fatal("tracked deletion ignored")
	}
}
func TestFingerprintRejectsMissingOrLinkedTestInput(t *testing.T) {
	p := fixture(t)
	c, _ := LoadContract(p)
	if err := os.Remove(filepath.Join(p.Root, "value_test.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := Fingerprint(p, c, 1); err == nil {
		t.Fatal("missing test file accepted")
	}
	outside := filepath.Join(t.TempDir(), "test.go")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(p.Root, "value_test.go")); err != nil {
		t.Skip("symlink unavailable")
	}
	if _, err := Fingerprint(p, c, 1); err == nil {
		t.Fatal("linked test input accepted")
	}
}
