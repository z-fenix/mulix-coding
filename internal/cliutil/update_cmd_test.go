package cliutil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/assets"
	"github.com/z-fenix/mulix-coding/internal/scaffold"
)

func registerCLIVendorFixture(t *testing.T, rel, vendor string) {
	t.Helper()
	full := filepath.Join(".mulix", "install-state.json")
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Schema  string                       `json:"schema"`
		Hosts   []string                     `json:"hosts"`
		Files   map[string]map[string]string `json:"files"`
		Pending map[string]json.RawMessage   `json:"pending,omitempty"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(vendor))
	record.Files[rel] = map[string]string{"vendor_sha256": hex.EncodeToString(hash[:])}
	data, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUpdate_NoOpSucceeds(t *testing.T) {
	dir := chdirTemp(t)
	if _, err := scaffold.Init(scaffold.InitOptions{Root: dir}); err != nil {
		t.Fatal(err)
	}
	out, err := runPresetCLI(t, "update")
	if err != nil || !strings.Contains(out, "0 conflicted") || !strings.Contains(out, "0 blocked") {
		t.Fatalf("no-op: %v %s", err, out)
	}
}

func cliConflictFixture(t *testing.T) (string, string, string) {
	t.Helper()
	dir := chdirTemp(t)
	if _, err := scaffold.Init(scaffold.InitOptions{Root: dir}); err != nil {
		t.Fatal(err)
	}
	rel := ".mulix/templates/spec-template.md"
	n, err := os.ReadFile(filepath.FromSlash(rel))
	if err != nil {
		t.Fatal(err)
	}
	end := strings.IndexByte(string(n), '\n')
	if end < 0 {
		t.Fatal("template has no newline")
	}
	base := "OLD TOP\n" + string(n[end+1:])
	local := "LOCAL TOP\n" + string(n[end+1:])
	if err := os.WriteFile(filepath.FromSlash(rel), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".mulix", ".installed", filepath.FromSlash(rel)), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	registerCLIVendorFixture(t, rel, base)
	out, err := runPresetCLI(t, "update")
	if err == nil || !strings.Contains(out, "1 conflicted") || !strings.Contains(out, ".update-conflicts/") {
		t.Fatalf("conflict not reported: %v %s", err, out)
	}
	got, readErr := os.ReadFile(filepath.FromSlash(rel))
	if readErr != nil || string(got) != local {
		t.Fatal("CLI conflict changed L")
	}
	return dir, rel, string(n)
}

func TestUpdate_AcceptResolvesPendingAndPrintsBackup(t *testing.T) {
	_, rel, vendor := cliConflictFixture(t)
	resolution := vendor + "\nCLI RESOLUTION\n"
	if err := os.WriteFile(filepath.FromSlash(rel), []byte(resolution), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runPresetCLI(t, "update", "--accept", rel)
	if err != nil || !strings.Contains(out, "1 accepted") || !strings.Contains(out, "backup:") {
		t.Fatalf("accept: %v %s", err, out)
	}
	got, err := os.ReadFile(filepath.FromSlash(rel))
	if err != nil || string(got) != resolution {
		t.Fatal("CLI accept replaced resolution")
	}
	got, err = os.ReadFile(filepath.Join(".mulix", ".installed", filepath.FromSlash(rel)))
	if err != nil || string(got) != vendor {
		t.Fatal("CLI accept did not store pure N")
	}
	if out, err := runPresetCLI(t, "update"); err != nil {
		t.Fatalf("retry: %v %s", err, out)
	}
}

func TestUpdate_InvalidAcceptFlagsPreserveMetadata(t *testing.T) {
	dir := chdirTemp(t)
	if _, err := scaffold.Init(scaffold.InitOptions{Root: dir}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(".mulix", "install-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	rel := ".mulix/templates/spec-template.md"
	for _, args := range [][]string{
		{"update", "--force", "--accept", rel},
		{"update", "--accept", rel, "--accept", rel},
		{"update", "--accept", "../outside"},
		{"update", "--accept", ".mulix/templates/overrides/user.md"},
		{"update", "--accept", rel},
	} {
		out, err := runPresetCLI(t, args...)
		if err == nil {
			t.Fatalf("invalid flags succeeded: %v %s", args, out)
		}
		if strings.Contains(err.Error(), "unknown flag") {
			t.Fatalf("accept flag missing: %v", err)
		}
		after, err := os.ReadFile(filepath.Join(".mulix", "install-state.json"))
		if err != nil || string(after) != string(before) {
			t.Fatal("invalid flags changed metadata")
		}
	}
}

func TestUpdate_PartialIOFailurePrintsCompletedPaths(t *testing.T) {
	dir := chdirTemp(t)
	if _, err := scaffold.Init(scaffold.InitOptions{Root: dir}); err != nil {
		t.Fatal(err)
	}
	first := ".claude/skills/brainstorming/SKILL.md"
	broken := ".claude/skills/brainstorming/builder-check-prompt.md"
	if err := os.Remove(filepath.FromSlash(first)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.FromSlash(broken)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.FromSlash(broken), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := runPresetCLI(t, "update")
	if err == nil || !strings.Contains(out, "1 written") || !strings.Contains(out, first) {
		t.Fatalf("partial output lost: %v %s", err, out)
	}
	if _, err := os.Stat(filepath.FromSlash(first)); err != nil {
		t.Fatal("earlier restoration did not complete")
	}
}

func TestUpdate_AllBlockedPathsArePrinted(t *testing.T) {
	dir := chdirTemp(t)
	if _, err := scaffold.Init(scaffold.InitOptions{Root: dir}); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for i := 1; i <= 7; i++ {
		rel := fmt.Sprintf(".mulix/templates/obsolete-%d.md", i)
		if err := os.WriteFile(filepath.FromSlash(rel), []byte("UNKNOWN\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(".mulix", ".installed", filepath.FromSlash(rel)), []byte("UNKNOWN\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, rel)
	}
	out, err := runPresetCLI(t, "update")
	if err == nil || !strings.Contains(out, "7 blocked") {
		t.Fatalf("blocked summary: %v %s", err, out)
	}
	for _, rel := range paths {
		if !strings.Contains(out, rel) {
			t.Fatalf("critical path omitted: %s", rel)
		}
	}
}

func TestInitCmd_ForcePrintsBackupDirectory(t *testing.T) {
	dir := chdirTemp(t)
	if _, err := scaffold.Init(scaffold.InitOptions{Root: dir}); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join(".mulix", "templates", "spec-template.md")
	if err := os.WriteFile(rel, []byte("LOCAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runPresetCLI(t, "init", "--force")
	if err != nil || !strings.Contains(out, "backup:") || !strings.Contains(out, ".backups") {
		t.Fatalf("init backup not visible: %v %s", err, out)
	}
}

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
	if _, err := scaffold.Init(scaffold.InitOptions{Root: dir}); err != nil {
		t.Fatal(err)
	}

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
	registerCLIVendorFixture(t, filepath.ToSlash(rel), oldVersion)
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
	if _, err := scaffold.Init(scaffold.InitOptions{Root: dir}); err != nil {
		t.Fatal(err)
	}

	rel := filepath.Join(".claude", "skills", "mulix-specify", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rel, []byte("user content, installed before baselines existed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(".mulix", ".installed", rel)); err != nil {
		t.Fatal(err)
	}
	out, err := runPresetCLI(t, "update")
	if err == nil {
		t.Fatalf("blocked update unexpectedly succeeded (output: %s)", out)
	}

	got, err := os.ReadFile(rel)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "user content, installed before baselines existed\n" {
		t.Fatalf("file without baseline must be untouched, got:\n%s", got)
	}
	if !strings.Contains(out, "no trusted baseline") || !strings.Contains(out, "blocked") {
		t.Fatalf("expected the summary to report the skipped file, got: %s", out)
	}
}
