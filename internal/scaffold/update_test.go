package scaffold

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/assets"
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
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, ".mulix/templates/new.md", "v2\n"); err != nil {
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
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, ".mulix/templates/new.md", "v2\n"); err != nil {
		t.Fatalf("updateFile: %v", err)
	}
	res = UpdateResult{}
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, ".mulix/templates/new.md", "v2\n"); err != nil {
		t.Fatalf("updateFile: %v", err)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("expected skip when already current, got %+v", res)
	}
}

func TestUpdateFile_UpdatesUnmodifiedFile(t *testing.T) {
	root := t.TempDir()
	var res UpdateResult
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "v1\n"); err != nil {
		t.Fatal(err)
	}
	res = UpdateResult{}
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "v2\n"); err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 {
		t.Fatalf("expected updated report, got %+v", res)
	}
	if got := readProjectFile(t, root, unitRel); got != "v2\n" {
		t.Fatalf("file = %q, want v2", got)
	}
	if got := readProjectFile(t, root, ".mulix/.installed/"+unitRel); got != "v2\n" {
		t.Fatalf("baseline = %q, want v2", got)
	}
}

func TestUpdateFile_MergesLocallyModifiedFile(t *testing.T) {
	root := t.TempDir()
	var res UpdateResult
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "a\nb\nc\n"); err != nil {
		t.Fatal(err)
	}
	// user edits the tail; incoming changes the head — disjoint
	writeProjectFile(t, root, unitRel, "a\nb\nlocal edit\n")
	res = UpdateResult{}
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "mulix edit\nb\nc\n"); err != nil {
		t.Fatal(err)
	}
	if len(res.Merged) != 1 {
		t.Fatalf("expected merged report, got %+v", res)
	}
	got := readProjectFile(t, root, unitRel)
	want := "mulix edit\nb\nlocal edit\n"
	if got != want {
		t.Fatalf("merged = %q, want %q", got, want)
	}
}

func TestUpdateFile_ConflictingChangePreservesLocalAndBaseline(t *testing.T) {
	root := t.TempDir()
	var res UpdateResult
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "a\nb\nc\n"); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, root, unitRel, "a\nlocal\nc\n")
	res = UpdateResult{}
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "a\nincoming\nc\n"); err != nil {
		t.Fatal(err)
	}
	if len(res.Conflicted) != 1 {
		t.Fatalf("expected conflicted report, got %+v", res)
	}
	if got := readProjectFile(t, root, unitRel); got != "a\nlocal\nc\n" {
		t.Fatalf("conflict changed local: %q", got)
	}
	if got := readProjectFile(t, root, ".mulix/.installed/"+unitRel); got != "a\nb\nc\n" {
		t.Fatalf("conflict changed baseline: %q", got)
	}
}

func TestUpdateFile_SkipsWithoutBaseline(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, unitRel, "user edited, no baseline\n")
	var res UpdateResult
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "v2\n"); err != nil {
		t.Fatal(err)
	}
	if len(res.Blocked) != 1 || !res.NeedsAttention() {
		t.Fatalf("expected blocked without baseline, got %+v", res)
	}
	if got := readProjectFile(t, root, unitRel); got != "user edited, no baseline\n" {
		t.Fatalf("file must be untouched, got %q", got)
	}
}

func TestUpdateFile_ForceOverwritesAndRebaselines(t *testing.T) {
	root := t.TempDir()
	var res UpdateResult
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "v1\n"); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, root, unitRel, "user edit\n")
	res = UpdateResult{}
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root, Force: true}, unitRel, "v2\n"); err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) != 1 {
		t.Fatalf("expected updated under force, got %+v", res)
	}
	if got := readProjectFile(t, root, unitRel); got != "v2\n" {
		t.Fatalf("file = %q, want v2", got)
	}
	if got := readProjectFile(t, root, ".mulix/.installed/"+unitRel); got != "v2\n" {
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
		".claude/skills/brainstorming/builder-check-prompt.md",
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
		recordVendorForTest(t, root, rel, "old\n")
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

// An upstream superpowers release deletes a file the project installed.
// The baseline still records it, so update cleans it up with no list to
// maintain — and leaves the rest of the skill directory alone.
func TestUpdate_RemovesFileUpstreamNoLongerShips(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	staleRel := ".claude/skills/writing-plans/plan-document-reviewer-prompt.md"
	writeProjectFile(t, root, staleRel, "old prompt\n")
	writeProjectFile(t, root, ".mulix/.installed/"+staleRel, "old prompt\n")
	recordVendorForTest(t, root, staleRel, "old prompt\n")

	res, err := Update(UpdateOptions{Root: root})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(staleRel))); !os.IsNotExist(err) {
		t.Fatalf("expected the upstream-deleted file to be removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", "writing-plans", "SKILL.md")); err != nil {
		t.Fatalf("expected the skill's shipped files to survive: %v", err)
	}
	for _, rel := range res.Removed {
		if rel == staleRel {
			return
		}
	}
	t.Fatalf("expected %s in Removed, got %+v", staleRel, res.Removed)
}

// The baseline records everything init wrote, including what it seeds
// once and then leaves to the project. The obsolete sweep must stay
// inside the trees mulix owns, so those files survive.
func TestUpdate_KeepsSeededOnceFilesOutOfTheObsoleteSweep(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	res, err := Update(UpdateOptions{Root: root})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	for _, rel := range []string{".mulix/memory/constitution.md", ".mulix/.runtime/.gitignore"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s to survive update: %v", rel, err)
		}
		for _, removed := range res.Removed {
			if removed == rel {
				t.Fatalf("update removed a seeded-once file: %s", rel)
			}
		}
	}
}

func TestUpdate_DerivesDSHHostFromBaselines(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Simulate an install by an older mulix: the bundled skill minus its
	// last line, with the baseline recording exactly that. Untouched on
	// disk, so update replaces it in place.
	bundledData, err := fs.ReadFile(assets.Skills, "skills/mulix-build/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	bundled := string(bundledData)
	lines := strings.Split(strings.TrimSuffix(bundled, "\n"), "\n")
	oldVersion := strings.Join(lines[:len(lines)-1], "\n") + "\n"
	staleRel := ".dsh/skills/mulix-build/SKILL.md"
	writeProjectFile(t, root, staleRel, oldVersion)
	writeProjectFile(t, root, ".mulix/.installed/"+staleRel, oldVersion)
	recordVendorForTest(t, root, staleRel, oldVersion)

	// And a deleted skill file: update rewrites it from the bundle.
	missingRel := ".dsh/skills/mulix-specify/SKILL.md"
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(missingRel))); err != nil {
		t.Fatal(err)
	}

	res, err := Update(UpdateOptions{Root: root})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := readProjectFile(t, root, staleRel); got != bundled {
		t.Error("expected the stale dsh skill to be refreshed to the bundled version")
	}
	bundledSpecify, err := fs.ReadFile(assets.Skills, "skills/mulix-specify/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if got := readProjectFile(t, root, missingRel); got != string(bundledSpecify) {
		t.Error("expected the deleted dsh skill to be rewritten from the bundle")
	}
	reportedUpdated := false
	reportedWritten := false
	for _, u := range res.Updated {
		if strings.HasPrefix(u, ".dsh/skills/") {
			reportedUpdated = true
		}
	}
	for _, w := range res.Written {
		if strings.HasPrefix(w, ".dsh/skills/") {
			reportedWritten = true
		}
	}
	if !reportedUpdated || !reportedWritten {
		t.Fatalf("expected dsh files in the updated and written reports, updated=%v written=%v", res.Updated, res.Written)
	}

	// The AGENTS.md section is refreshed as part of the same update. Right
	// after init it is already current, so it reports as skipped rather
	// than updated — it must be reported in one of the two.
	reportedAgents := false
	for _, u := range res.Updated {
		if strings.HasPrefix(u, "AGENTS.md") {
			reportedAgents = true
		}
	}
	for _, s := range res.Skipped {
		if strings.HasPrefix(s, "AGENTS.md") {
			reportedAgents = true
		}
	}
	if !reportedAgents {
		t.Fatalf("expected the AGENTS.md section to be reported, updated=%v skipped=%v", res.Updated, res.Skipped)
	}
}

func TestUpdate_ThreeRunsKeepLocalAndPureBaseline(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	rel := ".mulix/templates/spec-template.md"
	vendor := readProjectFile(t, root, rel)
	local := vendor + "\nLOCAL RULE\n"
	writeProjectFile(t, root, rel, local)
	for i := 0; i < 3; i++ {
		if _, err := Update(UpdateOptions{Root: root}); err != nil {
			t.Fatal(err)
		}
		if got := readProjectFile(t, root, rel); got != local {
			t.Fatalf("run %d lost local", i+1)
		}
		if got := readProjectFile(t, root, ".mulix/.installed/"+rel); got != vendor {
			t.Fatalf("run %d polluted baseline", i+1)
		}
	}
}

func TestUpdate_DSHOnlyNeverCreatesClaude(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(UpdateOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
		t.Fatalf("unexpected Claude tree: %v", err)
	}
}

func TestUpdate_ClaudeOnlyProjectNeverGetsDSH(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	res, err := Update(UpdateOptions{Root: root})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	for _, rel := range []string{".dsh/skills/mulix-using/SKILL.md", "AGENTS.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("expected update to not create %s in a claude-only project", rel)
		}
	}
	for _, w := range res.Written {
		if strings.HasPrefix(w, ".dsh/") || strings.HasPrefix(w, "AGENTS.md") {
			t.Errorf("unexpected dsh write in a claude-only project: %s", w)
		}
	}
}

func TestUpdate_RefreshesStaleAgentsSection(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// The user (or an older mulix) mangled the section body.
	agents := readProjectFile(t, root, "AGENTS.md")
	stale := strings.Replace(agents, "`mulix state show`", "`mulix state show --stale`", 1)
	if stale == agents {
		t.Fatal("test setup failed to mangle the section")
	}
	writeProjectFile(t, root, "AGENTS.md", stale)

	res, err := Update(UpdateOptions{Root: root})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := readProjectFile(t, root, "AGENTS.md"); got != agents {
		t.Fatalf("expected the section to be restored, got:\n%s", got)
	}
	found := false
	for _, u := range res.Updated {
		if strings.HasPrefix(u, "AGENTS.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the AGENTS.md section in the updated report, got %+v", res.Updated)
	}
}
