package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
		".claude/skills/mulix-using-mulix/SKILL.md",
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
