package scaffold

import (
	"io/fs"
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
			skills := host.skillsDir()
			for _, rel := range []string{
				skills + "/mulix-verify/references/acceptance-gate.md",
				skills + "/mulix-verify/scripts/check-acceptance.py",
				skills + "/mulix-verify/tests/test_acceptance_gate.py",
				".mulix/templates/acceptance-contract-template.json",
				".mulix/templates/acceptance-results-template.json",
			} {
				if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
					t.Errorf("default install must not ship %s: %v", rel, err)
				}
			}
			for _, dir := range []string{skills, ".mulix/templates"} {
				err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(dir)), func(full string, d fs.DirEntry, err error) error {
					if err != nil || d.IsDir() || filepath.Ext(full) != ".md" {
						return err
					}
					data, err := os.ReadFile(full)
					if err != nil {
						return err
					}
					for _, forbidden := range []string{"../pm/Product-Spec.md", "完整测试点与测试用例.md", "requirements/case/", "cases.json", "--workbench-root ../test", "reports/case-live", "check-acceptance.py", SkillsDirPlaceholder} {
						if strings.Contains(string(data), forbidden) {
							t.Errorf("%s carries project-specific input %q", full, forbidden)
						}
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			for path, fence := range map[string]string{
				".mulix/templates/spec-template.md":  "mulix-acceptance",
				".mulix/templates/tasks-template.md": "mulix-tests",
				skills + "/mulix-verify/SKILL.md":    "mulix-review",
			} {
				var block map[string]any
				if err := quality.Block([]byte(readProjectFile(t, root, path)), fence, &block); err != nil {
					t.Errorf("%s lost its native %s block: %v", path, fence, err)
				}
			}
			verify := readProjectFile(t, root, skills+"/mulix-verify/SKILL.md")
			if !strings.Contains(verify, "mulix verify") || strings.Contains(verify, "mulix state set verify_result") {
				t.Error("verification must use the native runner rather than hand-written success")
			}
		})
	}
}

func TestUpdateRetiresProjectSpecificAcceptanceAssets(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostClaude, HostDSH}}); err != nil {
		t.Fatal(err)
	}
	const pure = ".claude/skills/mulix-verify/scripts/check-acceptance.py"
	const local = ".dsh/skills/mulix-verify/scripts/check-acceptance.py"
	for _, rel := range []string{pure, local} {
		writeProjectFile(t, root, rel, "old vendor checker\n")
		writeProjectFile(t, root, ".mulix/.installed/"+rel, "old vendor checker\n")
		recordVendorForTest(t, root, rel, "old vendor checker\n")
	}
	writeProjectFile(t, root, local, "project-owned rules\n")
	if _, err := Update(UpdateOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(pure))); !os.IsNotExist(err) {
		t.Errorf("unmodified retired vendor file must be removed: %v", err)
	}
	if got := readProjectFile(t, root, local); got != "project-owned rules\n" {
		t.Errorf("customized retired file changed: %q", got)
	}
}
