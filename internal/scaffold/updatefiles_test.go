package scaffold

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

const unitRel = ".mulix/templates/unit.md"

func recordVendorForTest(t *testing.T, root, rel, vendor string) {
	t.Helper()
	st, err := loadInstallState(root)
	if err != nil {
		t.Fatal(err)
	}
	st.Files[rel] = vendorRecord{SHA256: contentDigest([]byte(vendor))}
	if err := saveInstallState(root, st); err != nil {
		t.Fatal(err)
	}
}

func updateManagedFileForTest(t *testing.T, res *UpdateResult, opts UpdateOptions, rel, incoming string) error {
	t.Helper()
	run, err := newUpdateRun(opts)
	if err != nil {
		return err
	}
	err = run.updateFile(managedFile{Rel: rel, Data: []byte(incoming)})
	*res = run.result
	return err
}

func TestChooseFileAction(t *testing.T) {
	cases := []struct {
		name                   string
		exists, trusted, force bool
		local, base, incoming  string
		want                   fileAction
	}{
		{"current even with unknown base", true, false, false, "N", "?", "N", actionCurrent},
		{"restore missing even when N equals B", false, true, false, "", "N", "N", actionInstall},
		{"force unknown", true, false, true, "L", "?", "N", actionForce},
		{"equal unknown L B is not trust", true, false, false, "CUSTOM", "CUSTOM", "N", actionBlocked},
		{"same N preserves L", true, true, false, "L", "N", "N", actionUnchanged},
		{"untouched vendor", true, true, false, "B", "B", "N", actionReplace},
		{"both changed", true, true, false, "L", "B", "N", actionMerge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chooseFileAction(c.exists, c.trusted, c.force, []byte(c.local), []byte(c.base), []byte(c.incoming)); got != c.want {
				t.Fatalf("action=%s want %s", got, c.want)
			}
		})
	}
}

func TestUpdateFile_TwoIncomingVersionsKeepPureBaselines(t *testing.T) {
	root := t.TempDir()
	var res UpdateResult
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "a\nb\nc\nd\n"); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, root, unitRel, "a\nb\nc\nLOCAL\n")
	for _, pair := range [][2]string{{"A\nb\nc\nd\n", "A\nb\nc\nLOCAL\n"}, {"A\nB\nc\nd\n", "A\nB\nc\nLOCAL\n"}} {
		n, want := pair[0], pair[1]
		if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, n); err != nil {
			t.Fatal(err)
		}
		if len(res.Merged) != 1 {
			t.Fatalf("not merged: %+v", res)
		}
		if got := readProjectFile(t, root, unitRel); got != want {
			t.Fatalf("local=%q want=%q", got, want)
		}
		if got := readProjectFile(t, root, ".mulix/.installed/"+unitRel); got != n {
			t.Fatalf("baseline=%q want N=%q", got, n)
		}
		st, err := loadInstallState(root)
		if err != nil || st.Files[unitRel].SHA256 != contentDigest([]byte(n)) {
			t.Fatalf("vendor digest not advanced: %v", err)
		}
	}
}

func TestUpdateFile_UnknownOrChangedBaselineIsBlocked(t *testing.T) {
	for _, recorded := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy merged baseline", true: "digest mismatch"}[recorded], func(t *testing.T) {
			root := t.TempDir()
			writeProjectFile(t, root, unitRel, "CUSTOM\n")
			writeProjectFile(t, root, ".mulix/.installed/"+unitRel, "CUSTOM\n")
			if recorded {
				recordVendorForTest(t, root, unitRel, "real vendor\n")
			}
			var res UpdateResult
			if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "new vendor\n"); err != nil {
				t.Fatal(err)
			}
			if !res.NeedsAttention() || len(res.Blocked) != 1 {
				t.Fatalf("not blocked: %+v", res)
			}
			if got := readProjectFile(t, root, unitRel); got != "CUSTOM\n" {
				t.Fatal("local overwritten")
			}
			if got := readProjectFile(t, root, ".mulix/.installed/"+unitRel); got != "CUSTOM\n" {
				t.Fatal("untrusted baseline overwritten")
			}
		})
	}
}

func TestUpdateFile_ExactIncomingProvesLegacyBaseWithoutAbsorbingLocal(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, unitRel, "N\nLOCAL\n")
	writeProjectFile(t, root, ".mulix/.installed/"+unitRel, "N\n")
	var res UpdateResult
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "N\n"); err != nil {
		t.Fatal(err)
	}
	if res.NeedsAttention() {
		t.Fatalf("exact base not recognized: %+v", res)
	}
	if got := readProjectFile(t, root, unitRel); got != "N\nLOCAL\n" {
		t.Fatal("local lost")
	}
	st, err := loadInstallState(root)
	if err != nil || st.Files[unitRel].SHA256 != contentDigest([]byte("N\n")) {
		t.Fatalf("vendor proof not registered: %v", err)
	}
	if len(res.Backups) != 0 {
		t.Fatal("no-op created backup")
	}
}

func TestUpdateFile_CurrentLocalCalibrationBacksUpUnknownBase(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, unitRel, "N\n")
	writeProjectFile(t, root, ".mulix/.installed/"+unitRel, "UNKNOWN BASE\n")
	var res UpdateResult
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root}, unitRel, "N\n"); err != nil {
		t.Fatal(err)
	}
	if len(res.Backups) != 1 {
		t.Fatalf("backups=%v", res.Backups)
	}
	data, err := os.ReadFile(filepath.Join(res.Backups[0], "baseline", filepath.FromSlash(unitRel)))
	if err != nil || string(data) != "UNKNOWN BASE\n" {
		t.Fatalf("old B lost: %q %v", data, err)
	}
}

func TestUpdateFile_BackupFailurePreventsForce(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, unitRel, "KEEP\n")
	writeProjectFile(t, root, ".mulix/.backups", "not a directory")
	var res UpdateResult
	if err := updateManagedFileForTest(t, &res, UpdateOptions{Root: root, Force: true}, unitRel, "N\n"); err == nil {
		t.Fatal("force ignored backup failure")
	}
	if got := readProjectFile(t, root, unitRel); got != "KEEP\n" {
		t.Fatal("local lost")
	}
}

func TestUpdateFile_BaselineWriteFailureKeepsBackupAndPreviousResults(t *testing.T) {
	root := t.TempDir()
	run, err := newUpdateRun(UpdateOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := run.updateFile(managedFile{Rel: unitRel, Data: []byte("B\n")}); err != nil {
		t.Fatal(err)
	}
	next := ".mulix/templates/next.md"
	writeProjectFile(t, root, next, "old\n")
	writeProjectFile(t, root, ".mulix/.installed/"+next, "old\n")
	recordVendorForTest(t, root, next, "old\n")
	run.state, err = loadInstallState(root)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("baseline replacement failed")
	realWrite := run.write
	run.write = func(full string, data []byte, mode fs.FileMode) error {
		if full == baselinePath(root, next) {
			return failure
		}
		return realWrite(full, data, mode)
	}
	if err := run.updateFile(managedFile{Rel: next, Data: []byte("new\n")}); !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
	if len(run.result.Written) != 1 || run.result.Written[0] != unitRel {
		t.Fatal("previous completion lost")
	}
	if got := readProjectFile(t, root, ".mulix/.installed/"+next); got != "old\n" {
		t.Fatal("failed B replacement changed B")
	}
	if got := readProjectFile(t, root, run.backups.dir+"/local/"+next); got != "old\n" {
		t.Fatal("old local not backed up")
	}
}

func TestUpdateObsolete_RequiresTrustedBaselineAndPreservesOverrides(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{unitRel, ".mulix/templates/overrides/custom.md"} {
		writeProjectFile(t, root, rel, "CUSTOM\n")
		writeProjectFile(t, root, ".mulix/.installed/"+rel, "CUSTOM\n")
	}
	run, err := newUpdateRun(UpdateOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := run.removeObsolete(unitRel); err != nil {
		t.Fatal(err)
	}
	if len(run.result.Blocked) != 1 {
		t.Fatal("unknown equal obsolete bytes deleted")
	}
	paths, err := unshippedBaselineFiles(root, []Host{HostClaude}, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != unitRel {
		t.Fatalf("sweep paths=%v", paths)
	}
	if err := run.removeObsolete(".mulix/templates/overrides/custom.md"); err == nil {
		t.Fatal("override path accepted")
	}
}

func TestUpdateObsolete_ForceBacksUpUnknownContent(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, unitRel, "CUSTOM\n")
	writeProjectFile(t, root, ".mulix/.installed/"+unitRel, "UNKNOWN BASE\n")
	run, err := newUpdateRun(UpdateOptions{Root: root, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := run.removeObsolete(unitRel); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(unitRel))); !os.IsNotExist(err) {
		t.Fatalf("obsolete remained: %v", err)
	}
	if got := readProjectFile(t, root, run.backups.dir+"/local/"+unitRel); got != "CUSTOM\n" {
		t.Fatal("force lost old L")
	}
	if got := readProjectFile(t, root, run.backups.dir+"/baseline/"+unitRel); got != "UNKNOWN BASE\n" {
		t.Fatal("force lost old B")
	}
}

func TestUpdateHosts_NoEvidenceIsAnError(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, ".dsh/skills/unrelated/SKILL.md", "user skill\n")
	if _, err := Update(UpdateOptions{Root: root}); err == nil {
		t.Fatal("uninstalled project got a default host")
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
		t.Fatal("missing evidence created Claude")
	}
}
