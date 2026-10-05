package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentsBootstrapSectionContainsSkillsDir(t *testing.T) {
	section := agentsBootstrapSection(".dsh/skills")
	if !strings.Contains(section, "`.dsh/skills/`") {
		t.Fatalf("expected the section to name the skills dir, got:\n%s", section)
	}
	if !strings.HasPrefix(section, agentsBootstrapBegin) || !strings.HasSuffix(section, agentsBootstrapEnd) {
		t.Fatal("expected the section to be wrapped in the sentinel markers")
	}
}

func TestMergeAgentsBootstrap_CreatesFileWhenMissing(t *testing.T) {
	root := t.TempDir()
	changed, err := mergeAgentsBootstrap(root, ".dsh/skills")
	if err != nil {
		t.Fatalf("mergeAgentsBootstrap: %v", err)
	}
	if !changed {
		t.Fatal("expected the file to be written")
	}
	data, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("reading AGENTS.md: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, agentsBootstrapBegin) || !strings.Contains(text, agentsBootstrapEnd) {
		t.Fatalf("expected the sentinel markers, got:\n%s", text)
	}
	if !strings.HasSuffix(text, "\n") {
		t.Fatal("expected the file to end with a newline")
	}
}

func TestMergeAgentsBootstrap_AppendsAfterUserContent(t *testing.T) {
	root := t.TempDir()
	user := "# My project\n\nCustom notes that must survive.\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(user), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := mergeAgentsBootstrap(root, ".dsh/skills"); err != nil {
		t.Fatalf("mergeAgentsBootstrap: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, user) {
		t.Fatalf("expected the user content to be preserved verbatim at the top, got:\n%s", text)
	}
	if !strings.Contains(text, agentsBootstrapBegin) {
		t.Fatal("expected the mulix section to be appended")
	}
}

func TestMergeAgentsBootstrap_IsIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := mergeAgentsBootstrap(root, ".dsh/skills"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := mergeAgentsBootstrap(root, ".dsh/skills")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected the second merge to be a no-op")
	}
	second, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("expected the file to be byte-identical after the second merge")
	}
	if got := strings.Count(string(second), agentsBootstrapBegin); got != 1 {
		t.Fatalf("expected exactly one begin marker, got %d", got)
	}
}

func TestMergeAgentsBootstrap_ReplacesStaleSection(t *testing.T) {
	root := t.TempDir()
	user := "# My project\n"
	stale := user + "\n" + agentsBootstrapBegin + "\n\n# mulix — stale body\n\nOUTDATED: skills in `.somewhere/old/`\n" + agentsBootstrapEnd + "\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := mergeAgentsBootstrap(root, ".dsh/skills"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.HasPrefix(text, user) {
		t.Fatal("expected the user content to survive the replacement")
	}
	if strings.Contains(text, "OUTDATED") || strings.Contains(text, ".somewhere/old/") {
		t.Fatalf("expected the stale section body to be replaced, got:\n%s", text)
	}
	if !strings.Contains(text, "`.dsh/skills/`") {
		t.Fatalf("expected the fresh section body, got:\n%s", text)
	}
	if got := strings.Count(text, agentsBootstrapBegin); got != 1 {
		t.Fatalf("expected exactly one begin marker, got %d", got)
	}
}

func TestMergeAgentsBootstrap_RejectsUnmatchedBeginMarker(t *testing.T) {
	root := t.TempDir()
	broken := "# My project\n\n" + agentsBootstrapBegin + "\nno end marker anywhere\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := mergeAgentsBootstrap(root, ".dsh/skills"); err == nil {
		t.Fatal("expected an error when the begin marker has no end marker")
	}
}
