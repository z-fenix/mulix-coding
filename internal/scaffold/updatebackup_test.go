package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateBackup_PreservesFirstSnapshotAndMetadata(t *testing.T) {
	root := t.TempDir()
	rel := ".mulix/templates/unit.md"
	writeProjectFile(t, root, rel, "local\n")
	writeProjectFile(t, root, ".mulix/.installed/"+rel, "vendor\n")
	writeProjectFile(t, root, installStateRel, "original metadata\n")
	b := &updateBackups{root: root}
	if err := b.preserve(rel); err != nil {
		t.Fatal(err)
	}
	for suffix, want := range map[string]string{"local/" + rel: "local\n", "baseline/" + rel: "vendor\n", "install-state.json": "original metadata\n"} {
		if got := readProjectFile(t, root, b.dir+"/"+suffix); got != want {
			t.Fatalf("backup %s = %q", suffix, got)
		}
	}
	writeProjectFile(t, root, rel, "later local\n")
	writeProjectFile(t, root, ".mulix/.installed/"+rel, "later baseline\n")
	if err := b.preserve(rel); err != nil {
		t.Fatal(err)
	}
	if got := readProjectFile(t, root, b.dir+"/local/"+rel); got != "local\n" {
		t.Fatal("first snapshot overwritten")
	}
	if got := readProjectFile(t, root, b.dir+"/baseline/"+rel); got != "vendor\n" {
		t.Fatal("first baseline overwritten")
	}
	if got := readProjectFile(t, root, ".mulix/.backups/.gitignore"); got != "*\n!.gitignore\n" {
		t.Fatalf("ignore = %q", got)
	}
	another := &updateBackups{root: root}
	if err := another.preserve(rel); err != nil {
		t.Fatal(err)
	}
	if another.dir == b.dir {
		t.Fatal("backup runs share directory")
	}
}

func TestUpdateBackup_ReadFailureIsNotMissing(t *testing.T) {
	root := t.TempDir()
	rel := ".mulix/templates/unit.md"
	writeProjectFile(t, root, rel, "keep\n")
	if err := os.MkdirAll(baselinePath(root, rel), 0o755); err != nil {
		t.Fatal(err)
	}
	b := &updateBackups{root: root}
	if err := b.preserve(rel); err == nil {
		t.Fatal("directory baseline ignored")
	}
	if b.backed[rel] {
		t.Fatal("failed backup marked complete")
	}
	if got := readProjectFile(t, root, rel); got != "keep\n" {
		t.Fatal("local changed")
	}
}

func TestUpdateBackup_RejectsPathsOutsideManagedAndTwoSeeds(t *testing.T) {
	b := &updateBackups{root: t.TempDir()}
	for _, rel := range []string{"../outside", "AGENTS.md", ".mulix/templates/overrides/custom.md", ".mulix/.runtime/change/state.yaml"} {
		if err := b.preserve(rel); err == nil {
			t.Fatalf("accepted %q", rel)
		}
	}
	if b.dir != "" {
		t.Fatal("invalid paths created a backup")
	}
	for _, rel := range []string{".mulix/memory/constitution.md", ".mulix/.runtime/.gitignore"} {
		writeProjectFile(t, b.root, rel, "seed\n")
		if err := b.preserve(rel); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpdateBackup_PreservesCustomIgnore(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, ".mulix/.backups/.gitignore", "USER RULE\n")
	b := &updateBackups{root: root}
	if err := b.preserve(".mulix/templates/unit.md"); err == nil {
		t.Fatal("custom ignore was overwritten")
	}
	if got := readProjectFile(t, root, ".mulix/.backups/.gitignore"); got != "USER RULE\n" {
		t.Fatal("custom ignore changed")
	}
}

func TestUpdateBackup_RejectsIgnoreSymlink(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeProjectFile(t, outside, "ignore", auxiliaryGitignore)
	if err := os.MkdirAll(filepath.Join(root, ".mulix", ".backups"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "ignore"), filepath.Join(root, ".mulix", ".backups", ".gitignore")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeProjectFile(t, root, ".mulix/templates/unit.md", "local\n")
	b := &updateBackups{root: root}
	if err := b.preserve(".mulix/templates/unit.md"); err == nil {
		t.Fatal("backup ignore symlink accepted")
	}
}

func TestUpdateBackup_RejectsSymlinkParents(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	writeProjectFile(t, outside, "unit.md", "outside\n")
	if err := os.MkdirAll(filepath.Join(root, ".mulix"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".mulix", "templates")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	b := &updateBackups{root: root}
	if err := b.preserve(".mulix/templates/unit.md"); err == nil {
		t.Fatal("followed symlink parent")
	}
	if got := readProjectFile(t, outside, "unit.md"); got != "outside\n" {
		t.Fatal("outside changed")
	}
}

func TestInit_CurrentVendorTrustCalibrationBacksUpOldBaseline(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	rel := ".mulix/templates/spec-template.md"
	vendor := readProjectFile(t, root, rel)
	writeProjectFile(t, root, ".mulix/.installed/"+rel, "CUSTOM OLD BASE\n")
	originalMeta := readProjectFile(t, root, installStateRel)
	res, err := Init(InitOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Backups) != 1 {
		t.Fatalf("backups=%v", res.Backups)
	}
	backup := res.Backups[0]
	for suffix, want := range map[string]string{"local/" + rel: vendor, "baseline/" + rel: "CUSTOM OLD BASE\n", "install-state.json": originalMeta} {
		data, err := os.ReadFile(filepath.Join(backup, filepath.FromSlash(suffix)))
		if err != nil || string(data) != want {
			t.Fatalf("backup %s = %q, %v", suffix, data, err)
		}
	}
	if got := readProjectFile(t, root, ".mulix/.installed/"+rel); got != vendor {
		t.Fatal("baseline not calibrated")
	}
	st, err := loadInstallState(root)
	if err != nil || st.Files[rel].SHA256 != contentDigest([]byte(vendor)) {
		t.Fatalf("trust not registered: %v", err)
	}
	again, err := Init(InitOptions{Root: root})
	if err != nil || len(again.Backups) != 0 {
		t.Fatalf("no-op backup: %+v, %v", again, err)
	}
}

func TestInit_CorruptMetadataStopsBeforeForce(t *testing.T) {
	root := t.TempDir()
	rel := ".mulix/templates/spec-template.md"
	writeProjectFile(t, root, rel, "KEEP\n")
	writeProjectFile(t, root, installStateRel, "{broken")
	if _, err := Init(InitOptions{Root: root, Force: true}); err == nil {
		t.Fatal("corrupt metadata ignored")
	}
	if got := readProjectFile(t, root, rel); got != "KEEP\n" {
		t.Fatal("force changed local before metadata validation")
	}
}
