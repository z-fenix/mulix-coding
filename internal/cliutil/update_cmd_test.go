package cliutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mulix-dev/mulix-coding/assets"
)

func bundledSkill(t *testing.T, name string) string {
	t.Helper()
	data, err := fs.ReadFile(assets.Skills, "skills/"+name+"/SKILL.md")
	if err != nil {
		t.Fatalf("reading bundled skill %s: %v", name, err)
	}
	return string(data)
}

func TestUpdate_MergesLocallyModifiedSkill(t *testing.T) {
	dir := chdirTemp(t)
	initMulixRoot(t, dir)

	// Simulate an install by an older mulix: the bundled skill minus its
	// last line, with a baseline copy recording exactly that.
	bundled := bundledSkill(t, "mulix-build")
	lines := strings.Split(strings.TrimSuffix(bundled, "\n"), "\n")
	oldVersion := strings.Join(lines[:len(lines)-1], "\n") + "\n"

	rel := filepath.Join(".claude", "skills", "mulix-build", "SKILL.md")
	baseRel := filepath.Join(".mulix", ".installed", ".claude", "skills", "mulix-build", "SKILL.md")
	for _, p := range []string{rel, baseRel} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(oldVersion), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The user rewrote the skill's first line; mulix's new version only
	// appends — disjoint edits, so a clean merge is expected.
	local := append([]string{"USER EDIT"}, lines[1:]...)
	if err := os.WriteFile(rel, []byte(strings.Join(local, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runPresetCLI(t, "update")
	if err != nil {
		t.Fatalf("update: %v (output: %s)", err, out)
	}

	got, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.HasPrefix(text, "USER EDIT\n") {
		t.Fatal("expected the local edit to survive the update")
	}
	if !strings.HasSuffix(text, bundled[len(bundled)-40:]) {
		t.Fatalf("expected the new bundled tail to be merged in, got tail:\n%s", text[len(text)-80:])
	}
	if strings.Contains(text, "<<<<<<<") {
		t.Fatal("expected a clean merge, found conflict markers")
	}
	if !strings.Contains(out, "merged") {
		t.Fatalf("expected the summary to report the merge, got: %s", out)
	}
}

func TestUpdate_SkipsFileWithoutBaseline(t *testing.T) {
	dir := chdirTemp(t)
	initMulixRoot(t, dir)

	rel := filepath.Join(".claude", "skills", "mulix-specify", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rel, []byte("user content, installed before baselines existed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runPresetCLI(t, "update")
	if err != nil {
		t.Fatalf("update: %v (output: %s)", err, out)
	}

	got, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "user content, installed before baselines existed\n" {
		t.Fatalf("file without baseline must be untouched, got:\n%s", got)
	}
	if !strings.Contains(out, "no baseline") {
		t.Fatalf("expected the summary to report the skipped file, got: %s", out)
	}
}
