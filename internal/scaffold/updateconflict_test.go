package scaffold

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

const conflictBase = "a\nb\nc\n"
const conflictLocal = "a\nlocal\nc\n"
const conflictIncoming = "a\nincoming\nc\n"

func seedConflict(t *testing.T) (string, *updateRun, managedFile) {
	t.Helper()
	root := t.TempDir()
	run, err := newUpdateRun(UpdateOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := run.updateFile(managedFile{Rel: unitRel, Data: []byte(conflictBase)}); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, root, unitRel, conflictLocal)
	run.result = UpdateResult{}
	return root, run, managedFile{Rel: unitRel, Data: []byte(conflictIncoming)}
}

func conflictFixture(t *testing.T) (string, *updateRun, managedFile) {
	t.Helper()
	root, run, f := seedConflict(t)
	if err := run.updateFile(f); err != nil {
		t.Fatal(err)
	}
	return root, run, f
}

func TestUpdateConflict_PreservesPairAndCandidateOnRetry(t *testing.T) {
	root, run, f := conflictFixture(t)
	pending, ok := run.state.Pending[unitRel]
	if !ok {
		t.Fatal("conflict did not persist pending")
	}
	candidate := readProjectFile(t, root, pending.Candidate)
	if !strings.Contains(candidate, "<<<<<<< incoming (mulix)\n") || !strings.Contains(candidate, ">>>>>>> local\n") {
		t.Fatalf("missing tool markers: %q", candidate)
	}
	if got := readProjectFile(t, root, path.Join(path.Dir(pending.Candidate), "incoming.md")); got != conflictIncoming {
		t.Fatal("candidate vendor is not pure N")
	}
	for i := 0; i < 2; i++ {
		retry, err := newUpdateRun(UpdateOptions{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		if err := retry.updateFile(f); err != nil {
			t.Fatal(err)
		}
		if len(retry.result.Conflicted) != 1 {
			t.Fatalf("retry not conflicted: %+v", retry.result)
		}
		if got := readProjectFile(t, root, unitRel); got != conflictLocal {
			t.Fatal("retry changed L")
		}
		if got := readProjectFile(t, root, ".mulix/.installed/"+unitRel); got != conflictBase {
			t.Fatal("retry changed B")
		}
		if got := readProjectFile(t, root, pending.Candidate); got != candidate {
			t.Fatal("retry changed candidate")
		}
	}
}

func TestUpdateConflict_MissingLocalDoesNotInstallOverPending(t *testing.T) {
	root, run, f := conflictFixture(t)
	p := run.state.Pending[unitRel]
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(unitRel))); err != nil {
		t.Fatal(err)
	}
	retry, err := newUpdateRun(UpdateOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := retry.updateFile(f); err != nil {
		t.Fatal(err)
	}
	if len(retry.result.Conflicted) != 1 {
		t.Fatalf("missing L erased conflict: %+v", retry.result)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(unitRel))); !os.IsNotExist(err) {
		t.Fatal("missing pending L was overwritten")
	}
	if retry.state.Pending[unitRel].Candidate != p.Candidate {
		t.Fatal("pending changed")
	}
}

func TestUpdateConflict_ChangedVendorBlocksWithoutReplacingCandidate(t *testing.T) {
	root, run, _ := conflictFixture(t)
	p := run.state.Pending[unitRel]
	candidate := readProjectFile(t, root, p.Candidate)
	if err := run.updateFile(managedFile{Rel: unitRel, Data: []byte("newer vendor\n")}); err != nil {
		t.Fatal(err)
	}
	if len(run.result.Blocked) != 1 {
		t.Fatal("changed incoming erased pending")
	}
	if got := readProjectFile(t, root, p.Candidate); got != candidate {
		t.Fatal("candidate replaced")
	}
	if got := readProjectFile(t, root, unitRel); got != conflictLocal {
		t.Fatal("L changed")
	}
}

func TestUpdateConflict_CandidateRetryDoesNotOverwriteHumanEdits(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(map[bool]string{false: "reuse unchanged", true: "preserve edited"}[edit], func(t *testing.T) {
			root, run, f := seedConflict(t)
			failure := errors.New("metadata save failed")
			run.saveState = func(*installState) error { return failure }
			if err := run.updateFile(f); !errors.Is(err, failure) {
				t.Fatalf("error=%v", err)
			}
			entries, err := os.ReadDir(filepath.Join(root, ".mulix", ".update-conflicts"))
			if err != nil {
				t.Fatal(err)
			}
			original := ""
			for _, e := range entries {
				if e.IsDir() {
					if original != "" {
						t.Fatal("multiple initial candidates")
					}
					original = path.Join(".mulix/.update-conflicts", e.Name(), "merged.md")
				}
			}
			if original == "" {
				t.Fatal("failed save did not leave candidate")
			}
			if edit {
				writeProjectFile(t, root, original, "HUMAN CANDIDATE\n")
			}
			retry, err := newUpdateRun(UpdateOptions{Root: root})
			if err != nil {
				t.Fatal(err)
			}
			if err := retry.updateFile(f); err != nil {
				t.Fatal(err)
			}
			next := retry.state.Pending[unitRel].Candidate
			if edit {
				if next == original {
					t.Fatal("reused edited candidate")
				}
				if got := readProjectFile(t, root, original); got != "HUMAN CANDIDATE\n" {
					t.Fatal("human candidate overwritten")
				}
			} else if next != original {
				t.Fatal("identical candidate not reused")
			}
		})
	}
}

func TestAcceptPending_PreservesResolutionAndPureVendorAcrossNextVersion(t *testing.T) {
	root, _, f := conflictFixture(t)
	resolution := "a\nresolved custom\nc\n"
	writeProjectFile(t, root, unitRel, resolution)
	run, err := newUpdateRun(UpdateOptions{Root: root, Accept: []string{unitRel}})
	if err != nil {
		t.Fatal(err)
	}
	realWrite := run.write
	run.write = func(full string, data []byte, mode fs.FileMode) error {
		if full == filepath.Join(root, filepath.FromSlash(unitRel)) {
			t.Fatal("accept rewrote L")
		}
		return realWrite(full, data, mode)
	}
	if err := run.acceptPending(unitRel, f); err != nil {
		t.Fatal(err)
	}
	if got := readProjectFile(t, root, unitRel); got != resolution {
		t.Fatal("resolution overwritten")
	}
	if got := readProjectFile(t, root, ".mulix/.installed/"+unitRel); got != conflictIncoming {
		t.Fatal("B is not pure N")
	}
	if _, ok := run.state.Pending[unitRel]; ok {
		t.Fatal("pending survived acceptance")
	}
	if len(run.result.Accepted) != 1 {
		t.Fatalf("accepted=%v", run.result.Accepted)
	}
	run.write = realWrite
	if err := run.updateFile(f); err != nil {
		t.Fatal(err)
	}
	if run.result.NeedsAttention() {
		t.Fatal("same vendor conflicted again")
	}
	next := "a\nincoming\nc\n\nEXTRA\n"
	if err := run.updateFile(managedFile{Rel: unitRel, Data: []byte(next)}); err != nil {
		t.Fatal(err)
	}
	if got := readProjectFile(t, root, unitRel); got != "a\nresolved custom\nc\n\nEXTRA\n" {
		t.Fatalf("next merge=%q", got)
	}
	if got := readProjectFile(t, root, ".mulix/.installed/"+unitRel); got != next {
		t.Fatal("next B polluted")
	}
}

func TestAcceptPending_RejectsMarkersButAllowsMarkdownSeparator(t *testing.T) {
	for _, body := range []string{"<<<<<<< incoming (mulix)\ntext\n", ">>>>>>> local\r\n", "Heading\n=======\n"} {
		t.Run(body, func(t *testing.T) {
			root, _, f := conflictFixture(t)
			writeProjectFile(t, root, unitRel, body)
			run, err := newUpdateRun(UpdateOptions{Root: root, Accept: []string{unitRel}})
			if err != nil {
				t.Fatal(err)
			}
			err = run.acceptPending(unitRel, f)
			allowed := body == "Heading\n=======\n"
			if allowed && err != nil {
				t.Fatal(err)
			}
			if !allowed && err == nil {
				t.Fatal("unresolved marker accepted")
			}
			if got := readProjectFile(t, root, unitRel); got != body {
				t.Fatal("accept changed L")
			}
			if !allowed {
				if got := readProjectFile(t, root, ".mulix/.installed/"+unitRel); got != conflictBase {
					t.Fatal("rejected acceptance changed B")
				}
			}
		})
	}
}

func TestAcceptPending_MissingLocalOrChangedIncomingIsRejected(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed N", true: "missing L"}[missing], func(t *testing.T) {
			root, _, f := conflictFixture(t)
			if missing {
				if err := os.Remove(filepath.Join(root, filepath.FromSlash(unitRel))); err != nil {
					t.Fatal(err)
				}
			} else {
				f.Data = []byte("different N\n")
			}
			run, err := newUpdateRun(UpdateOptions{Root: root, Accept: []string{unitRel}})
			if err != nil {
				t.Fatal(err)
			}
			if err := run.acceptPending(unitRel, f); err == nil {
				t.Fatal("invalid acceptance succeeded")
			}
			if _, ok := run.state.Pending[unitRel]; !ok {
				t.Fatal("pending lost")
			}
		})
	}
}

func TestAcceptPending_MetadataFailureRetainsPendingForRetry(t *testing.T) {
	root, _, f := conflictFixture(t)
	writeProjectFile(t, root, unitRel, "RESOLVED\n")
	run, err := newUpdateRun(UpdateOptions{Root: root, Accept: []string{unitRel}})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("metadata commit failed")
	run.saveState = func(*installState) error { return failure }
	if err := run.acceptPending(unitRel, f); !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
	st, err := loadInstallState(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.Pending[unitRel]; !ok {
		t.Fatal("failed save cleared durable pending")
	}
	if _, ok := run.state.Pending[unitRel]; !ok {
		t.Fatal("failed save cleared in-memory pending")
	}
	retry, err := newUpdateRun(UpdateOptions{Root: root, Accept: []string{unitRel}})
	if err != nil {
		t.Fatal(err)
	}
	if err := retry.acceptPending(unitRel, f); err != nil {
		t.Fatal(err)
	}
	if got := readProjectFile(t, root, unitRel); got != "RESOLVED\n" {
		t.Fatal("retry replaced resolution")
	}
}

func TestAcceptPending_InvalidOptionsFailBeforeWrites(t *testing.T) {
	root, _, _ := conflictFixture(t)
	original := readProjectFile(t, root, installStateRel)
	for _, opts := range []UpdateOptions{
		{Root: root, Force: true, Accept: []string{unitRel}},
		{Root: root, Accept: []string{unitRel, unitRel}},
		{Root: root, Accept: []string{"../outside"}},
		{Root: root, Accept: []string{".mulix/templates/overrides/user.md"}},
		{Root: root, Accept: []string{".mulix/templates/no-pending.md"}},
	} {
		if _, err := newUpdateRun(opts); err == nil {
			t.Fatalf("invalid options accepted: %+v", opts)
		}
		if got := readProjectFile(t, root, installStateRel); got != original {
			t.Fatal("option error changed metadata")
		}
	}
}

func TestUpdateConflict_UnshippedMissingBaseIsStillReported(t *testing.T) {
	root, run, _ := conflictFixture(t)
	run.state.Hosts = []Host{HostClaude}
	if err := saveInstallState(root, run.state); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(baselinePath(root, unitRel)); err != nil {
		t.Fatal(err)
	}
	res, err := Update(UpdateOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if !res.NeedsAttention() {
		t.Fatal("pending without an obsolete baseline disappeared from output")
	}
	if got := readProjectFile(t, root, unitRel); got != conflictLocal {
		t.Fatal("pending L changed")
	}
}

func TestUpdateConflict_UnshippedPendingIsNotForgotten(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "retain pending", true: "force obsolete cleanup"}[force], func(t *testing.T) {
			root, run, _ := conflictFixture(t)
			run.state.Hosts = []Host{HostClaude}
			if err := saveInstallState(root, run.state); err != nil {
				t.Fatal(err)
			}
			writeProjectFile(t, root, unitRel, conflictBase)
			res, err := Update(UpdateOptions{Root: root, Force: force})
			if err != nil {
				t.Fatal(err)
			}
			st, err := loadInstallState(root)
			if err != nil {
				t.Fatal(err)
			}
			_, pending := st.Pending[unitRel]
			if force {
				if pending || len(res.Removed) != 1 {
					t.Fatalf("force left stale pending: %+v", res)
				}
			} else {
				if !pending || !res.NeedsAttention() {
					t.Fatal("unshipped pending was forgotten")
				}
				if got := readProjectFile(t, root, unitRel); got != conflictBase {
					t.Fatal("pending target was deleted")
				}
			}
		})
	}
}

func TestUpdateConflict_ForceBacksUpBeforeClearingPending(t *testing.T) {
	root, _, f := conflictFixture(t)
	run, err := newUpdateRun(UpdateOptions{Root: root, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := run.updateFile(f); err != nil {
		t.Fatal(err)
	}
	if _, ok := run.state.Pending[unitRel]; ok {
		t.Fatal("force did not clear pending")
	}
	if got := readProjectFile(t, root, run.backups.dir+"/local/"+unitRel); got != conflictLocal {
		t.Fatal("force lost old L")
	}
	if got := readProjectFile(t, root, run.backups.dir+"/baseline/"+unitRel); got != conflictBase {
		t.Fatal("force lost old B")
	}
	if got := readProjectFile(t, root, unitRel); got != conflictIncoming {
		t.Fatal("force did not install N")
	}
}

func TestUpdateConflict_InitRefusesPending(t *testing.T) {
	root, _, _ := conflictFixture(t)
	for _, force := range []bool{false, true} {
		if _, err := Init(InitOptions{Root: root, Force: force}); err == nil {
			t.Fatal("init claimed an unresolved update")
		}
	}
	if got := readProjectFile(t, root, unitRel); got != conflictLocal {
		t.Fatal("init changed pending L")
	}
	if _, err := os.Stat(root + "/.claude"); !os.IsNotExist(err) {
		t.Fatal("init wrote before checking pending")
	}
}
