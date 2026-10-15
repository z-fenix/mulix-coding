package scaffold

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func assertPartialOutcome(t *testing.T, result UpdateResult, rel, completed, failed string) {
	t.Helper()
	// Decode the public result contract so the pre-fix test can demonstrate a
	// missing outcome instead of failing to compile before Partial is added.
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var output struct{ Partial []string }
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	for _, item := range output.Partial {
		if strings.Contains(item, rel) && strings.Contains(item, completed) && strings.Contains(item, "failed: "+failed) {
			return
		}
	}
	t.Fatalf("missing partial outcome for %s (%s; failed: %s): %+v", rel, completed, failed, result)
}

func TestUpdateReview_WindowsPathIdentityProtectsShippedAndOverrides(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows path identity")
	}
	t.Run("shipped baseline recased", func(t *testing.T) {
		root := t.TempDir()
		if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}}); err != nil {
			t.Fatal(err)
		}
		rel := entryRel(HostDSH, entrySkillName)
		base := baselinePath(root, rel)
		if err := os.Rename(base, base+".case-temp"); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(base+".case-temp", filepath.Join(filepath.Dir(base), "skill.md")); err != nil {
			t.Fatal(err)
		}
		result, err := Update(UpdateOptions{Root: root, Force: true})
		if err != nil || result.NeedsAttention() {
			t.Fatalf("update: %v %+v", err, result)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("still-shipped entry deleted: %v", err)
		}
		if len(result.Removed) != 0 {
			t.Fatalf("still-shipped entry treated as obsolete: %v", result.Removed)
		}
	})
	t.Run("override directory recased", func(t *testing.T) {
		root := t.TempDir()
		if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}}); err != nil {
			t.Fatal(err)
		}
		rel := ".mulix/templates/overrides/keep.md"
		alias := ".mulix/templates/OVERRIDES/keep.md"
		writeProjectFile(t, root, rel, "USER OVERRIDE\n")
		writeProjectFile(t, root, ".mulix/.installed/"+alias, "OLD OVERRIDE\n")
		if _, err := Update(UpdateOptions{Root: root, Force: true}); err != nil {
			t.Fatal(err)
		}
		if got := readProjectFile(t, root, rel); got != "USER OVERRIDE\n" {
			t.Fatal("excluded override changed")
		}
		if validManagedRel(alias) {
			t.Fatal("case alias admitted into managed ownership")
		}
	})
}

func TestUpdateReview_WindowsMetadataPathIdentity(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("native Windows path identity")
	}
	upper := ".mulix/templates/UNIT.md"
	t.Run("duplicate vendor identity rejected", func(t *testing.T) {
		st := newInstallState()
		st.Files[unitRel] = vendorRecord{SHA256: contentDigest([]byte("B"))}
		st.Files[upper] = vendorRecord{SHA256: contentDigest([]byte("B"))}
		if err := st.validate(); err == nil {
			t.Fatal("two spellings of one vendor file accepted")
		}
	})
	t.Run("excluded override record rejected", func(t *testing.T) {
		st := newInstallState()
		st.Files[".mulix/templates/OVERRIDES/keep.md"] = vendorRecord{SHA256: contentDigest([]byte("B"))}
		if err := st.validate(); err == nil {
			t.Fatal("override alias registered as managed")
		}
	})
	t.Run("vendor lookup retains customization", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, unitRel, "a\nb\nc\nlocal\n")
		writeProjectFile(t, root, ".mulix/.installed/"+unitRel, "a\nb\nc\nd\n")
		st := newInstallState()
		st.Files[upper] = vendorRecord{SHA256: contentDigest([]byte("a\nb\nc\nd\n"))}
		if err := saveInstallState(root, st); err != nil {
			t.Fatal(err)
		}
		run := migrationRun(t, root, false)
		if err := run.updateFile(managedFile{Rel: unitRel, Data: []byte("A\nb\nc\nd\n")}); err != nil {
			t.Fatal(err)
		}
		if run.result.NeedsAttention() {
			t.Fatalf("recased vendor record lost trust: %+v", run.result)
		}
		if got := readProjectFile(t, root, unitRel); got != "A\nb\nc\nlocal\n" {
			t.Fatal("local customization changed")
		}
	})
	t.Run("pending lookup and duplicate accept", func(t *testing.T) {
		root, run, f := conflictFixture(t)
		p := run.state.Pending[unitRel]
		delete(run.state.Pending, unitRel)
		run.state.Pending[upper] = p
		if err := saveInstallState(root, run.state); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(unitRel))); err != nil {
			t.Fatal(err)
		}
		retry := migrationRun(t, root, false)
		if err := retry.updateFile(f); err != nil {
			t.Fatal(err)
		}
		if !retry.result.NeedsAttention() {
			t.Fatal("recased pending bypassed missing local file")
		}
		if _, exists, err := readInstalledFile(root, unitRel); err != nil || exists {
			t.Fatalf("pending target restored: %v", err)
		}
		if _, err := newUpdateRun(UpdateOptions{Root: root, Accept: []string{unitRel}}); err != nil {
			t.Fatalf("equivalent accept path lost pending: %v", err)
		}
		if _, err := newUpdateRun(UpdateOptions{Root: root, Accept: []string{unitRel, upper}}); err == nil {
			t.Fatal("duplicate accept identity allowed")
		}
	})
}

func TestUpdateReview_OrdinaryCanonicalPendingPrecedesAliasMigration(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}}); err != nil {
		t.Fatal(err)
	}
	target := entryRel(HostDSH, entrySkillName)
	vendor := readProjectFile(t, root, target)
	local := strings.Replace(vendor, "# Using mulix", "# Local entry", 1)
	base := strings.Replace(vendor, "# Using mulix", "# Old vendor entry", 1)
	writeProjectFile(t, root, target, local)
	writeProjectFile(t, root, ".mulix/.installed/"+target, base)
	run := migrationRun(t, root, false)
	run.state.Files[target] = vendorRecord{SHA256: contentDigest([]byte(base))}
	if err := saveInstallState(root, run.state); err != nil {
		t.Fatal(err)
	}
	if err := run.updateFile(managedFile{Rel: target, Data: []byte(vendor)}); err != nil {
		t.Fatal(err)
	}
	pending := run.state.Pending[target]
	if pending.Stage != "conflict" || len(pending.Sources) != 0 {
		t.Fatal("fixture has no ordinary entry conflict")
	}
	candidate := readProjectFile(t, root, pending.Candidate)
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(target))); err != nil {
		t.Fatal(err)
	}
	source := entryRel(HostDSH, "using-mulix")
	old := string(frozenEntry(t, HostDSH, "using-mulix"))
	writeProjectFile(t, root, source, old)
	for i := 0; i < 2; i++ {
		result, err := Update(UpdateOptions{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		if !result.NeedsAttention() || len(result.Migrated) != 0 {
			t.Fatalf("ordinary pending bypassed by alias: %+v", result)
		}
		st, err := loadInstallState(root)
		if err != nil || st.Pending[target].Candidate != pending.Candidate {
			t.Fatalf("pending cleared: %v", err)
		}
		if _, exists, err := readInstalledFile(root, target); err != nil || exists {
			t.Fatalf("pending target installed: %v", err)
		}
		if got := readProjectFile(t, root, source); got != old {
			t.Fatal("alias cleaned without acceptance")
		}
		if got := readProjectFile(t, root, pending.Candidate); got != candidate {
			t.Fatal("candidate changed")
		}
	}
}

func TestUpdateReview_ObsoletePendingForceRecoversAfterSaveFailure(t *testing.T) {
	root, run, _ := conflictFixture(t)
	run.state.Hosts = []Host{HostDSH}
	if err := saveInstallState(root, run.state); err != nil {
		t.Fatal(err)
	}
	candidate := run.state.Pending[unitRel].Candidate
	original := readProjectFile(t, root, candidate)
	run.opts.Force = true
	failure := errors.New("metadata save interrupted")
	run.saveState = func(*installState) error { return failure }
	if err := run.removeObsolete(unitRel); !errors.Is(err, failure) {
		t.Fatalf("error: %v", err)
	}
	for _, rel := range []string{unitRel, ".mulix/.installed/" + unitRel} {
		if _, exists, err := readInstalledFile(root, rel); err != nil || exists {
			t.Fatalf("fixture not removed: %s %v", rel, err)
		}
	}
	result, err := Update(UpdateOptions{Root: root})
	if err != nil || !result.NeedsAttention() {
		t.Fatalf("normal retry concealed pending: %v %+v", err, result)
	}
	result, err = Update(UpdateOptions{Root: root, Force: true})
	if err != nil || result.NeedsAttention() {
		t.Fatalf("force retry cannot recover: %v %+v", err, result)
	}
	st, err := loadInstallState(root)
	if err != nil || len(st.Pending) != 0 {
		t.Fatalf("obsolete pending stranded: %v %+v", err, st)
	}
	if got := readProjectFile(t, root, candidate); got != original {
		t.Fatal("candidate discarded during recovery")
	}
	if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}}); err != nil {
		t.Fatalf("init still blocked after recovery: %v", err)
	}
}

func TestUpdateReview_SameFilePartialWriteOutcomes(t *testing.T) {
	for _, stage := range []string{"vendor baseline", "vendor record"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			writeProjectFile(t, root, unitRel, "OLD LOCAL\n")
			writeProjectFile(t, root, ".mulix/.installed/"+unitRel, "OLD VENDOR\n")
			run := migrationRun(t, root, true)
			failure := errors.New("interrupted " + stage)
			if stage == "vendor record" {
				run.saveState = func(*installState) error { return failure }
			} else {
				realWrite := run.write
				run.write = func(full string, data []byte, mode fs.FileMode) error {
					if full == baselinePath(root, unitRel) {
						return failure
					}
					return realWrite(full, data, mode)
				}
			}
			if err := run.updateFile(managedFile{Rel: unitRel, Data: []byte("NEW VENDOR\n")}); !errors.Is(err, failure) {
				t.Fatalf("error: %v", err)
			}
			if got := readProjectFile(t, root, unitRel); got != "NEW VENDOR\n" {
				t.Fatal("local write did not complete")
			}
			assertPartialOutcome(t, run.result, unitRel, "local written", stage)
			if len(run.result.Updated)+len(run.result.Written)+len(run.result.Merged) != 0 {
				t.Fatal("incomplete per-file update claimed successful")
			}
		})
	}
}

func TestUpdateReview_SameFilePartialRemovalOutcomes(t *testing.T) {
	for _, stage := range []string{"obsolete baseline", "obsolete metadata"} {
		t.Run(stage, func(t *testing.T) {
			root, run, _ := conflictFixture(t)
			run.opts.Force = true
			failure := errors.New("interrupted " + stage)
			if stage == "obsolete metadata" {
				run.saveState = func(*installState) error { return failure }
			} else {
				realRemove := run.remove
				run.remove = func(full string) error {
					if full == baselinePath(root, unitRel) {
						return failure
					}
					return realRemove(full)
				}
			}
			if err := run.removeObsolete(unitRel); !errors.Is(err, failure) {
				t.Fatalf("error: %v", err)
			}
			if _, exists, err := readInstalledFile(root, unitRel); err != nil || exists {
				t.Fatalf("local deletion not complete: %v", err)
			}
			assertPartialOutcome(t, run.result, unitRel, "local removed", stage)
			if len(run.result.Removed) != 0 {
				t.Fatal("incomplete removal claimed successful")
			}
		})
	}
}

func TestUpdateReview_SameFilePartialMigrationAndAcceptance(t *testing.T) {
	t.Run("migration baseline", func(t *testing.T) {
		root := t.TempDir()
		source := entryRel(HostClaude, "using-mulix")
		writeProjectFile(t, root, source, string(frozenEntry(t, HostClaude, "using-mulix")))
		incoming := incomingEntry(t, HostClaude)
		run := migrationRun(t, root, false)
		failure := errors.New("baseline interrupted")
		realWrite := run.write
		run.write = func(full string, data []byte, mode fs.FileMode) error {
			if full == baselinePath(root, incoming.Rel) {
				return failure
			}
			return realWrite(full, data, mode)
		}
		if _, _, err := run.migrateEntry(HostClaude, incoming); !errors.Is(err, failure) {
			t.Fatalf("error: %v", err)
		}
		assertPartialOutcome(t, run.result, incoming.Rel, "local written", "migration baseline")
		if len(run.result.Migrated) != 0 {
			t.Fatal("incomplete migration claimed successful")
		}
	})
	t.Run("source cleanup", func(t *testing.T) {
		root := t.TempDir()
		for _, alias := range entryAliases {
			writeProjectFile(t, root, entryRel(HostClaude, alias), string(frozenEntry(t, HostClaude, alias)))
		}
		incoming := incomingEntry(t, HostClaude)
		run := migrationRun(t, root, false)
		failure := errors.New("source cleanup interrupted")
		realRemove := run.remove
		run.remove = func(full string) error {
			if full == filepath.Join(root, filepath.FromSlash(entryRel(HostClaude, entryAliases[1]))) {
				return failure
			}
			return realRemove(full)
		}
		if _, _, err := run.migrateEntry(HostClaude, incoming); !errors.Is(err, failure) {
			t.Fatalf("error: %v", err)
		}
		assertPartialOutcome(t, run.result, incoming.Rel, "source removed: "+entryRel(HostClaude, entryAliases[0]), "source cleanup")
	})
	t.Run("acceptance metadata", func(t *testing.T) {
		root, run, incoming := conflictFixture(t)
		writeProjectFile(t, root, unitRel, "resolved custom\n")
		failure := errors.New("acceptance save interrupted")
		run.saveState = func(*installState) error { return failure }
		if err := run.acceptPending(unitRel, incoming); !errors.Is(err, failure) {
			t.Fatalf("error: %v", err)
		}
		assertPartialOutcome(t, run.result, unitRel, "vendor baseline written", "acceptance metadata")
		if len(run.result.Accepted) != 0 {
			t.Fatal("incomplete acceptance claimed successful")
		}
	})
}

func TestUpdateReview_AuxiliaryIgnoreCRLFRulesKeepOriginalBytes(t *testing.T) {
	const rules = "# User rules\r\n*.private\r\n*\r\n!.gitignore\r\n"
	t.Run("backup", func(t *testing.T) {
		root := t.TempDir()
		writeProjectFile(t, root, unitRel, "OLD LOCAL\n")
		writeProjectFile(t, root, ".mulix/.backups/.gitignore", rules)
		run := migrationRun(t, root, true)
		if err := run.updateFile(managedFile{Rel: unitRel, Data: []byte("NEW VENDOR\n")}); err != nil {
			t.Fatalf("CRLF backup rules rejected: %v", err)
		}
		if got := readProjectFile(t, root, ".mulix/.backups/.gitignore"); got != rules {
			t.Fatal("backup rules rewritten")
		}
		if len(run.result.Backups) != 1 {
			t.Fatal("no protective backup")
		}
	})
	t.Run("conflict", func(t *testing.T) {
		root, run, f := seedConflict(t)
		writeProjectFile(t, root, ".mulix/.update-conflicts/.gitignore", rules)
		if err := run.updateFile(f); err != nil {
			t.Fatalf("CRLF conflict rules rejected: %v", err)
		}
		if got := readProjectFile(t, root, ".mulix/.update-conflicts/.gitignore"); got != rules {
			t.Fatal("conflict rules rewritten")
		}
		if len(run.result.Conflicted) != 1 {
			t.Fatal("conflict not recorded")
		}
	})
}
