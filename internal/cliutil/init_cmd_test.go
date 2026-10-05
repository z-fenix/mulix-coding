package cliutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCmd_HostDSHInstallsDSHTree(t *testing.T) {
	_ = chdirTemp(t)

	out, err := runPresetCLI(t, "init", "--dir", ".", "--host", "dsh")
	if err != nil {
		t.Fatalf("init --host dsh: %v (output: %s)", err, out)
	}

	for _, rel := range []string{
		filepath.Join(".dsh", "skills", "using-mulix", "SKILL.md"),
		filepath.Join(".dsh", "skills", "brainstorming", "SKILL.md"),
		filepath.Join("AGENTS.md"),
	} {
		if _, err := os.Stat(rel); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(".claude", "settings.json")); !os.IsNotExist(err) {
		t.Error("expected --host dsh to not write .claude/settings.json")
	}
	data, err := os.ReadFile(filepath.Join("AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "`.dsh/skills/`") {
		t.Errorf("expected AGENTS.md to name the dsh skills dir, got:\n%s", data)
	}
	if !strings.Contains(out, "DeepSeek Harness") {
		t.Errorf("expected the DSH note in the output, got: %s", out)
	}
}

func TestInitCmd_HostClaudeKeepsClaudeNote(t *testing.T) {
	_ = chdirTemp(t)

	out, err := runPresetCLI(t, "init", "--dir", ".", "--host", "claude")
	if err != nil {
		t.Fatalf("init --host claude: %v (output: %s)", err, out)
	}
	if !strings.Contains(out, "Claude Code") {
		t.Errorf("expected the Claude Code note in the output, got: %s", out)
	}
	if strings.Contains(out, "DeepSeek Harness") {
		t.Errorf("did not expect the DSH note for a claude-only init, got: %s", out)
	}
	if _, err := os.Stat(filepath.Join(".claude", "settings.json")); err != nil {
		t.Errorf("expected the Claude hook settings to exist: %v", err)
	}
}

func TestInitCmd_BothHosts(t *testing.T) {
	_ = chdirTemp(t)

	out, err := runPresetCLI(t, "init", "--dir", ".", "--host", "claude,dsh")
	if err != nil {
		t.Fatalf("init --host claude,dsh: %v (output: %s)", err, out)
	}
	for _, rel := range []string{
		filepath.Join(".claude", "skills", "using-mulix", "SKILL.md"),
		filepath.Join(".dsh", "skills", "using-mulix", "SKILL.md"),
		filepath.Join(".claude", "settings.json"),
		filepath.Join("AGENTS.md"),
	} {
		if _, err := os.Stat(rel); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
	if !strings.Contains(out, "Claude Code") || !strings.Contains(out, "DeepSeek Harness") {
		t.Errorf("expected both host notes in the output, got: %s", out)
	}
}

func TestInitCmd_UnknownHostFails(t *testing.T) {
	_ = chdirTemp(t)

	out, err := runPresetCLI(t, "init", "--dir", ".", "--host", "codex")
	if err == nil {
		t.Fatalf("expected an error for an unknown host, output: %s", out)
	}
	if !strings.Contains(err.Error()+out, "unknown host") {
		t.Errorf("expected the error to name the unknown host, got err=%v out=%s", err, out)
	}
}
