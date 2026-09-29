package scaffold

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// writeProjectFile writes relPath (slash-separated) under root.
func writeProjectFile(t *testing.T, root, relPath, content string) string {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", relPath, err)
	}
	return full
}

func readProjectFile(t *testing.T, root, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("ReadFile %s: %v", relPath, err)
	}
	return string(data)
}

func TestUpdateFile_WritesMissingFileAndBaseline(t *testing.T) {
	root := t.TempDir()
	var res UpdateResult
	if err := updateFile(&res, UpdateOptions{Root: root}, ".mulix/templates/new.md", "v2\n"); err != nil {
		t.Fatalf("updateFile: %v", err)
	}
	if len(res.Written) != 1 || res.Written[0] != ".mulix/templates/new.md" {
		t.Fatalf("expected written report, got %+v", res)
	}
	if got := readProjectFile(t, root, ".mulix/templates/new.md"); got != "v2\n" {
		t.Fatalf("file = %q, want v2", got)
	}
	if got := readProjectFile(t, root, ".mulix/.installed/.mulix/templates/new.md"); got != "v2\n" {
		t.Fatalf("baseline = %q, want v2", got)
	}
}

func TestUpdateFile_SkipsWhenAlreadyCurrent(t *testing.T) {
	root := t.TempDir()
	var res UpdateResult
	if err := updateFile(&res, UpdateOptions{Root: root}, ".mulix/templates/new.md", "v2\n"); err != nil {
		t.Fatalf("updateFile: %v", err)
	}
	res = UpdateResult{}
	if err := updateFile(&res, UpdateOptions{Root: root}, ".mulix/templates/new.md", "v2\n"); err != nil {
		t.Fatalf("updateFile: %v", err)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("expected skip when already current, got %+v", res)
	}
}

func TestUpdateFile_UpdatesUnmodifiedFile(t *testing.T) {
	root := t.TempDir()
	var res UpdateResult
	if err := updateFile(&res, UpdateOptions{Root: root}, "x.md", "v1\n"); err != nil {
		t.Fatal(err)
	}
	res = UpdateResult{}
	if err := updateFile(&res, UpdateOptions{Root: root}, "x.md", "v2\n"); err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 {
		t.Fatalf("expected updated report, got %+v", res)
	}
	if got := readProjectFile(t, root, "x.md"); got != "v2\n" {
		t.Fatalf("file = %q, want v2", got)
	}
	if got := readProjectFile(t, root, ".mulix/.installed/x.md"); got != "v2\n" {
		t.Fatalf("baseline = %q, want v2", got)
	}
}

func TestUpdateFile_MergesLocallyModifiedFile(t *testing.T) {
	root := t.TempDir()
	var res UpdateResult
	if err := updateFile(&res, UpdateOptions{Root: root}, "x.md", "a\nb\nc\n"); err != nil {
		t.Fatal(err)
	}
	// user edits the tail; incoming changes the head — disjoint
	writeProjectFile(t, root, "x.md", "a\nb\nlocal edit\n")
	res = UpdateResult{}
	if err := updateFile(&res, UpdateOptions{Root: root}, "x.md", "mulix edit\nb\nc\n"); err != nil {
		t.Fatal(err)
	}
	if len(res.Merged) != 1 {
		t.Fatalf("expected merged report, got %+v", res)
	}
	got := readProjectFile(t, root, "x.md")
	want := "mulix edit\nb\nlocal edit\n"
	if got != want {
		t.Fatalf("merged = %q, want %q", got, want)
	}
}

func TestUpdateFile_ConflictingChangeWritesMarkers(t *testing.T) {
	root := t.TempDir()
	var res UpdateResult
	if err := updateFile(&res, UpdateOptions{Root: root}, "x.md", "a\nb\nc\n"); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, root, "x.md", "a\nlocal\nc\n")
	res = UpdateResult{}
	if err := updateFile(&res, UpdateOptions{Root: root}, "x.md", "a\nincoming\nc\n"); err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicted) != 1 {
		t.Fatalf("expected conflicted report, got %+v", res)
	}
	got := readProjectFile(t, root, "x.md")
	for _, want := range []string{"<<<<<<<", "incoming", "=======", "local", ">>>>>>>"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected conflict markers containing %q, got:\n%s", want, got)
		}
	}
}

func TestUpdateFile_SkipsWithoutBaseline(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "x.md", "user edited, no baseline\n")
	var res UpdateResult
	if err := updateFile(&res, UpdateOptions{Root: root}, "x.md", "v2\n"); err != nil {
		t.Fatal(err)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("expected skip without baseline, got %+v", res)
	}
	if got := readProjectFile(t, root, "x.md"); got != "user edited, no baseline\n" {
		t.Fatalf("file must be untouched, got %q", got)
	}
}

func TestUpdateFile_ForceOverwritesAndRebaselines(t *testing.T) {
	root := t.TempDir()
	var res UpdateResult
	if err := updateFile(&res, UpdateOptions{Root: root}, "x.md", "v1\n"); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, root, "x.md", "user edit\n")
	res = UpdateResult{}
	if err := updateFile(&res, UpdateOptions{Root: root, Force: true}, "x.md", "v2\n"); err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 {
		t.Fatalf("expected updated under force, got %+v", res)
	}
	if got := readProjectFile(t, root, "x.md"); got != "v2\n" {
		t.Fatalf("file = %q, want v2", got)
	}
	if got := readProjectFile(t, root, ".mulix/.installed/x.md"); got != "v2\n" {
		t.Fatalf("baseline = %q, want v2", got)
	}
}

func TestInit_RecordsBaselineForWrittenFiles(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	installed := filepath.Join(root, ".mulix", ".installed")
	var baselines int
	if err := filepath.Walk(installed, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			baselines++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if baselines == 0 {
		t.Fatal("expected init to record baseline copies under .mulix/.installed")
	}
	// Spot-check one: the installed template and its baseline match.
	src := readProjectFile(t, root, ".mulix/templates/tasks-template.md")
	base := readProjectFile(t, root, ".mulix/.installed/.mulix/templates/tasks-template.md")
	if !reflect.DeepEqual(src, base) {
		t.Fatal("baseline should mirror the installed template")
	}
}

func TestInit_InstallsEverySuperpowersSkillDirectory(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, rel := range []string{
		".claude/skills/writing-plans/plan-document-reviewer-prompt.md",
		".claude/skills/subagent-driven-development/implementer-prompt.md",
		".claude/skills/executing-plans/scripts/task-done",
		".claude/skills/using-superpowers/references/claude-code-tools.md",
		".mulix/superpowers/VERSION",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to be installed: %v", rel, err)
		}
	}
	// Installed skills must not point back at the upstream plugin.
	body := readProjectFile(t, root, ".claude/skills/subagent-driven-development/SKILL.md")
	if strings.Contains(body, "superpowers:") || strings.Contains(body, ".superpowers/") {
		t.Fatal("expected the installed skill to be rewritten for mulix")
	}
}

func TestInit_ScriptsAreExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no executable permission bit")
	}
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, ".claude", "skills", "subagent-driven-development", "scripts", "task-brief"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("expected task-brief to be executable, mode %v", info.Mode())
	}
}

func TestUpdate_RemovesUntouchedObsoleteFileKeepsModifiedOne(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Simulate an older install: both obsolete files with baselines, one
	// edited locally since.
	for _, rel := range []string{".claude/skills/mulix-plan/SKILL.md", ".mulix/templates/analyze-template.md"} {
		writeProjectFile(t, root, rel, "old\n")
		writeProjectFile(t, root, ".mulix/.installed/"+rel, "old\n")
	}
	writeProjectFile(t, root, ".mulix/templates/analyze-template.md", "user edit\n")

	res, err := Update(UpdateOptions{Root: root})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "mulix-plan")); !os.IsNotExist(err) {
		t.Fatalf("expected the untouched obsolete skill (and its dir) to be removed, stat err = %v", err)
	}
	if got := readProjectFile(t, root, ".mulix/templates/analyze-template.md"); got != "user edit\n" {
		t.Fatalf("expected the modified obsolete file to be kept, got %q", got)
	}
	if len(res.Removed) != 1 {
		t.Fatalf("expected exactly one removal, got %+v", res.Removed)
	}
}
