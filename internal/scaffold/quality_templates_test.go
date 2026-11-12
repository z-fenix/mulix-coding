package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledVerifiableAcceptanceContract(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	for rel, required := range map[string][]string{
		".mulix/templates/spec-template.md":     {"mulix-acceptance", "AC-001", "requirement: FR-001"},
		".mulix/templates/tasks-template.md":    {"mulix-tests", "kind: code", "criterion: AC-001", "suite: unit", "file:"},
		".mulix/templates/quality-template.md":  {"mulix.quality.v1", "go-json", "timeout_seconds", ".mulix/quality.yaml", "mandatory"},
		".claude/skills/mulix-build/SKILL.md":   {"mulix test run --task N --stage red", "mulix test run --task N --stage green", "mulix test run --task N --stage check"},
		".claude/skills/mulix-verify/SKILL.md":  {"mulix verify", "mulix-review", "blocking: []"},
		".claude/skills/mulix-archive/SKILL.md": {"mulix verify", "--reason", "current"},
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Error(err)
			continue
		}
		for _, needle := range required {
			if !strings.Contains(string(data), needle) {
				t.Errorf("%s is missing %q", rel, needle)
			}
		}
	}
	// The runner policy belongs to the project, never to init/update's vendor
	// defaults. Resource refresh must not silently change what is mandatory.
	policy := filepath.Join(root, ".mulix/quality.yaml")
	custom := "project-owned quality policy\n"
	if err := os.WriteFile(policy, []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(UpdateOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(policy)
	if err != nil || string(data) != custom {
		t.Fatal("project policy overwritten", err)
	}
}
