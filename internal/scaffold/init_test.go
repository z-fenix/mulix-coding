package scaffold

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/assets"
)

func TestInit_WritesExpectedFiles(t *testing.T) {
	root := t.TempDir()

	res, err := Init(InitOptions{Root: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(res.Skipped) != 0 {
		t.Fatalf("expected nothing skipped on a fresh init, got %v", res.Skipped)
	}

	mustExist := []string{
		".mulix/memory/constitution.md",
		".mulix/templates/spec-template.md",
		".claude/skills/mulix-build/SKILL.md",
		".claude/skills/using-mulix/SKILL.md",
		".claude/skills/brainstorming/SKILL.md",
		".claude/skills/brainstorming/scripts/start-server.sh",
		".claude/skills/test-driven-development/SKILL.md",
		".claude/skills/subagent-driven-development/scripts/sdd-workspace",
		".mulix/superpowers/LICENSE",
		".mulix/.runtime/.gitignore",
		".claude/settings.json",
	}
	for _, rel := range mustExist {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
}

func TestInit_HookIsRegisteredInSettingsJSON(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	var hooks map[string][]hookMatcherGroup
	if err := json.Unmarshal(parsed["hooks"], &hooks); err != nil {
		t.Fatalf("parsing hooks: %v", err)
	}
	preToolUse, ok := hooks["PreToolUse"]
	if !ok || len(preToolUse) == 0 {
		t.Fatalf("expected a PreToolUse hook group, got %+v", hooks)
	}
	found := false
	for _, h := range preToolUse[0].Hooks {
		if h.Command == "mulix hook" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a hook entry with command 'mulix hook', got %+v", preToolUse)
	}
}

func TestInit_PreservesExistingUnrelatedSettings(t *testing.T) {
	root := t.TempDir()
	claudeDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	existing := `{"someOtherSetting": "keep-me", "hooks": {"SessionStart": [{"matcher": "startup", "hooks": [{"type": "command", "command": "echo hi"}]}]}}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}

	var someOther string
	if err := json.Unmarshal(parsed["someOtherSetting"], &someOther); err != nil || someOther != "keep-me" {
		t.Fatalf("expected someOtherSetting to be preserved, got %q (err=%v)", someOther, err)
	}

	var hooks map[string][]hookMatcherGroup
	if err := json.Unmarshal(parsed["hooks"], &hooks); err != nil {
		t.Fatalf("parsing hooks: %v", err)
	}
	if len(hooks["SessionStart"]) == 0 {
		t.Fatal("expected pre-existing SessionStart hook to be preserved")
	}
	if len(hooks["PreToolUse"]) == 0 {
		t.Fatal("expected PreToolUse hook to be added alongside existing hooks")
	}
}

func TestInit_WithoutForceSkipsExistingFiles(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, ".claude", "skills", "mulix-build")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	customContent := []byte("# my custom override\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), customContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	res, err := Init(InitOptions{Root: root})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	foundSkipped := false
	for _, s := range res.Skipped {
		if s == ".claude/skills/mulix-build/SKILL.md" {
			foundSkipped = true
		}
	}
	if !foundSkipped {
		t.Fatalf("expected mulix-build SKILL.md to be reported skipped, got %v", res.Skipped)
	}

	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading SKILL.md: %v", err)
	}
	if string(data) != string(customContent) {
		t.Fatal("expected custom SKILL.md content to be left untouched without --force")
	}
}

func TestInit_ForceOverwritesExistingFiles(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, ".claude", "skills", "mulix-build")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Init(InitOptions{Root: root, Force: true}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading SKILL.md: %v", err)
	}
	if string(data) == "stale" {
		t.Fatal("expected --force to overwrite stale SKILL.md content")
	}
}

func TestInit_IsIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	res, err := Init(InitOptions{Root: root})
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if len(res.Written) != 0 {
		t.Fatalf("expected second init to write nothing new, got written=%v", res.Written)
	}
}

func TestInit_DefaultHostsAreClaudeOnly(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "settings.json")); err != nil {
		t.Errorf("expected the Claude hook settings to exist: %v", err)
	}
	for _, rel := range []string{".dsh/skills/using-mulix/SKILL.md", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("expected %s to not exist on a claude-only init", rel)
		}
	}
}

func TestInit_HostDSHInstallsDSHTreeAndBootstrap(t *testing.T) {
	root := t.TempDir()
	res, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}})
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	for _, rel := range []string{
		".dsh/skills/mulix-build/SKILL.md",
		".dsh/skills/using-mulix/SKILL.md",
		".dsh/skills/brainstorming/SKILL.md",
		".dsh/skills/test-driven-development/SKILL.md",
		".dsh/skills/subagent-driven-development/scripts/sdd-workspace",
		".mulix/superpowers/LICENSE",
		"AGENTS.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Error("expected a dsh-only init to not touch .claude/settings.json")
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills")); !os.IsNotExist(err) {
		t.Error("expected a dsh-only init to not write .claude/skills")
	}

	// mulix's own skills are host-aware: the placeholder must be replaced
	// with the DSH skills directory, and the vendored superpowers skills
	// must be verbatim.
	usingMulix := readProjectFile(t, root, ".dsh/skills/using-mulix/SKILL.md")
	if !strings.Contains(usingMulix, "`.dsh/skills/`") {
		t.Error("expected using-mulix to name the DSH skills directory")
	}
	if strings.Contains(usingMulix, SkillsDirPlaceholder) {
		t.Error("expected no placeholder to survive substitution")
	}
	bundled, err := fs.ReadFile(assets.Superpowers, "superpowers/skills/brainstorming/SKILL.md")
	if err != nil {
		t.Fatalf("reading bundled skill: %v", err)
	}
	if got := readProjectFile(t, root, ".dsh/skills/brainstorming/SKILL.md"); got != string(bundled) {
		t.Error("expected the embedded superpowers skill to be copied verbatim")
	}

	// The bootstrap section names the skills dir and the guard discipline.
	agents := readProjectFile(t, root, "AGENTS.md")
	if !strings.Contains(agents, agentsBootstrapBegin) || !strings.Contains(agents, agentsBootstrapEnd) {
		t.Error("expected AGENTS.md to carry the sentinel markers")
	}
	if !strings.Contains(agents, "`.dsh/skills/`") {
		t.Error("expected AGENTS.md to name the DSH skills directory")
	}

	// Baselines are recorded for the DSH tree so update can manage it.
	if _, err := os.Stat(filepath.Join(root, ".mulix", ".installed", ".dsh", "skills", "using-mulix", "SKILL.md")); err != nil {
		t.Errorf("expected a baseline for the DSH tree: %v", err)
	}

	wroteAgents := false
	for _, w := range res.Written {
		if w == "AGENTS.md (mulix section)" {
			wroteAgents = true
		}
	}
	if !wroteAgents {
		t.Errorf("expected the AGENTS.md section to be reported written, got %v", res.Written)
	}
}

func TestInit_HostDSHIsIdempotentIncludingBootstrap(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}}); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	res, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}})
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if len(res.Written) != 0 {
		t.Fatalf("expected second init to write nothing new, got written=%v", res.Written)
	}
	agents := readProjectFile(t, root, "AGENTS.md")
	if got := strings.Count(agents, agentsBootstrapBegin); got != 1 {
		t.Fatalf("expected exactly one mulix section, got %d", got)
	}
}

func TestInit_BothHostsInstallBothTrees(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostClaude, HostDSH}}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, rel := range []string{
		".claude/skills/using-mulix/SKILL.md",
		".dsh/skills/using-mulix/SKILL.md",
		".claude/settings.json",
		"AGENTS.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
	claude := readProjectFile(t, root, ".claude/skills/using-mulix/SKILL.md")
	if !strings.Contains(claude, "`.claude/skills/`") {
		t.Error("expected the claude copy to name the claude skills dir")
	}
	dsh := readProjectFile(t, root, ".dsh/skills/using-mulix/SKILL.md")
	if !strings.Contains(dsh, "`.dsh/skills/`") {
		t.Error("expected the dsh copy to name the dsh skills dir")
	}
	if claude == dsh {
		t.Error("expected the two installed copies to differ in their skills dir")
	}
}

func TestInit_UnknownHostRejected(t *testing.T) {
	if _, err := Init(InitOptions{Root: t.TempDir(), Hosts: []Host{"codex"}}); err == nil {
		t.Fatal("expected an error for an unknown host")
	}
}
