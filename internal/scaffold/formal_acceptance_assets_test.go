package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/quality"
)

func TestInstalledProjectFormalAcceptanceAssets(t *testing.T) {
	for _, host := range []Host{HostClaude, HostDSH} {
		t.Run(string(host), func(t *testing.T) {
			root := t.TempDir()
			if _, err := Init(InitOptions{Root: root, Hosts: []Host{host}}); err != nil {
				t.Fatal(err)
			}
			read := func(relative string) string {
				t.Helper()
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
				if err != nil {
					t.Fatal(err)
				}
				return string(data)
			}
			skills := host.skillsDir()
			for relative, required := range map[string][]string{
				".mulix/templates/constitution-template.md":            {"Acceptance Authority & Evidence", "Evidence levels"},
				".mulix/templates/spec-template.md":                    {"Acceptance Baseline & Scope", "Formal Acceptance Mapping", "mulix-acceptance"},
				".mulix/templates/tasks-template.md":                   {"Acceptance & Release Gates", "Acceptance Contract", "Implementation status", "Acceptance status", "mulix-tests"},
				skills + "/mulix-specify/SKILL.md":                     {"Formal Acceptance Mapping", "mulix-acceptance"},
				skills + "/mulix-tasks/SKILL.md":                       {"Acceptance contract", "mulix-tests", "mulix test run"},
				skills + "/mulix-verify/SKILL.md":                      {"project-scoped", "check-acceptance.py", "mulix verify", "mulix-review"},
				skills + "/mulix-archive/SKILL.md":                     {"project-scoped", "mulix verify", "current verification receipt"},
				skills + "/mulix-verify/references/acceptance-gate.md": {"适用范围", skills + "/mulix-verify/scripts/check-acceptance.py"},
				skills + "/mulix-verify/scripts/check-acceptance.py":   {"def check_acceptance(", "EVIDENCE_CHECK_BLOCKED"},
				skills + "/mulix-verify/tests/test_acceptance_gate.py": {"class AcceptanceGateTest", "test_native_execution_before_scope_review_is_rejected"},
			} {
				content := read(relative)
				for _, expected := range required {
					if !strings.Contains(content, expected) {
						t.Errorf("%s is missing %q", relative, expected)
					}
				}
				if strings.Contains(content, SkillsDirPlaceholder) {
					t.Errorf("%s retains an unresolved host placeholder", relative)
				}
			}

			var criteria, matrix map[string]any
			if err := quality.Block([]byte(read(".mulix/templates/spec-template.md")), "mulix-acceptance", &criteria); err != nil {
				t.Error("native acceptance fence must remain unique:", err)
			}
			if err := quality.Block([]byte(read(".mulix/templates/tasks-template.md")), "mulix-tests", &matrix); err != nil {
				t.Error("native test matrix fence must remain unique:", err)
			}
			verify := read(skills + "/mulix-verify/SKILL.md")
			if strings.Contains(verify, "mulix state set verify_result") {
				t.Error("project evidence supplement must not restore manual native verification results")
			}
			if host == HostDSH && strings.Contains(read(skills+"/mulix-verify/references/acceptance-gate.md"), ".claude/skills/") {
				t.Error("DSH installation must not point to Claude's skills directory")
			}

			var contract struct {
				AcceptanceLevel string            `json:"acceptance_level"`
				BlockingItems   []json.RawMessage `json:"blocking_items"`
			}
			if err := json.Unmarshal([]byte(read(".mulix/templates/acceptance-contract-template.json")), &contract); err != nil {
				t.Fatal(err)
			}
			if contract.AcceptanceLevel != "FORMAL" || len(contract.BlockingItems) == 0 {
				t.Error("unreviewed formal acceptance template must remain blocked")
			}
			var results struct {
				Blockers []json.RawMessage `json:"blockers"`
				Cases    []struct {
					Checkpoints []struct {
						Status string `json:"status"`
					} `json:"checkpoints"`
				} `json:"cases"`
			}
			if err := json.Unmarshal([]byte(read(".mulix/templates/acceptance-results-template.json")), &results); err != nil {
				t.Fatal(err)
			}
			if len(results.Blockers) == 0 || len(results.Cases) == 0 || len(results.Cases[0].Checkpoints) == 0 || results.Cases[0].Checkpoints[0].Status != "NOT_RUN" {
				t.Error("unexecuted results template must retain blockers and NOT_RUN")
			}
		})
	}
}
