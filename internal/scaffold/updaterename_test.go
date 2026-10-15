package scaffold

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func frozenEntry(t *testing.T, host Host, alias string) []byte {
	t.Helper()
	file := "using-mulix.md"
	if alias == "mulix-using-mulix" {
		file = "mulix-using-mulix.md"
	}
	data, err := os.ReadFile(filepath.Join("testdata", "legacy", file))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.ReplaceAll(data, []byte(SkillsDirPlaceholder), []byte(host.skillsDir()))
}

func incomingEntry(t *testing.T, host Host) managedFile {
	t.Helper()
	// Independent mechanical rename fixture; frozen old evidence stays untouched.
	data := frozenEntry(t, host, "using-mulix")
	data = bytes.Replace(data, []byte("name: using-mulix"), []byte("name: mulix-using"), 1)
	return managedFile{Rel: entryRel(host, entrySkillName), Data: data}
}

func migrationRun(t *testing.T, root string, force bool) *updateRun {
	t.Helper()
	run, err := newUpdateRun(UpdateOptions{Root: root, Force: force})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestEntryMigration_KnownAliasesIncludingBaselineMissing(t *testing.T) {
	for _, c := range []struct {
		host     Host
		alias    string
		baseline bool
	}{{HostClaude, "using-mulix", true}, {HostDSH, "using-mulix", false}, {HostClaude, "mulix-using-mulix", false}} {
		t.Run(string(c.host)+"/"+c.alias, func(t *testing.T) {
			root := t.TempDir()
			rel := entryRel(c.host, c.alias)
			old := frozenEntry(t, c.host, c.alias)
			writeProjectFile(t, root, rel, string(old))
			if c.baseline {
				writeProjectFile(t, root, ".mulix/.installed/"+rel, string(old))
			}
			writeProjectFile(t, root, filepath.ToSlash(filepath.Join(filepath.Dir(rel), "USER.md")), "KEEP AUX\n")
			run := migrationRun(t, root, false)
			incoming := incomingEntry(t, c.host)
			handled, ready, err := run.migrateEntry(c.host, incoming)
			if err != nil || !handled || !ready {
				t.Fatalf("handled=%v ready=%v err=%v result=%+v", handled, ready, err, run.result)
			}
			if got := readProjectFile(t, root, incoming.Rel); got != string(incoming.Data) {
				t.Fatal("canonical bytes wrong")
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
				t.Fatal("old entry survived")
			}
			if got := readProjectFile(t, root, filepath.ToSlash(filepath.Join(filepath.Dir(rel), "USER.md"))); got != "KEEP AUX\n" {
				t.Fatal("auxiliary file removed")
			}
			if got := readProjectFile(t, root, run.backups.dir+"/local/"+rel); got != string(old) {
				t.Fatal("old entry not backed up")
			}
			if len(run.state.Pending) != 0 || len(run.result.Migrated) != 1 {
				t.Fatalf("migration not completed: %+v", run.result)
			}
		})
	}
}

func TestEntryMigration_CustomSourceAndPureSourcePreserveUniqueCustomization(t *testing.T) {
	root := t.TempDir()
	host := HostClaude
	rel := entryRel(host, "using-mulix")
	base := frozenEntry(t, host, "using-mulix")
	writeProjectFile(t, root, rel, string(base)+"\nLOCAL ENTRY RULE\n")
	writeProjectFile(t, root, ".mulix/.installed/"+rel, string(base))
	other := entryRel(host, "mulix-using-mulix")
	writeProjectFile(t, root, other, string(frozenEntry(t, host, "mulix-using-mulix")))
	run := migrationRun(t, root, false)
	incoming := incomingEntry(t, host)
	handled, ready, err := run.migrateEntry(host, incoming)
	if err != nil || !handled || !ready {
		t.Fatalf("migration: %v %v %v %+v", handled, ready, err, run.result)
	}
	want := string(incoming.Data) + "\nLOCAL ENTRY RULE\n"
	if got := readProjectFile(t, root, incoming.Rel); got != want {
		t.Fatal("unique custom source was not preserved")
	}
	if got := readProjectFile(t, root, ".mulix/.installed/"+incoming.Rel); got != string(incoming.Data) {
		t.Fatal("B contains merged data")
	}
	retry := migrationRun(t, root, false)
	if err := retry.updateFile(incoming); err != nil {
		t.Fatal(err)
	}
	if got := readProjectFile(t, root, incoming.Rel); got != want {
		t.Fatal("next update lost customization")
	}
}

func TestEntryMigration_BlocksUnknownMultipleCustomAndTargetCollision(t *testing.T) {
	for _, kind := range []string{"unknown without base", "polluted equal base", "multiple custom", "target collision"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			host := HostClaude
			oldRel := entryRel(host, "using-mulix")
			base := frozenEntry(t, host, "using-mulix")
			local := string(base)
			switch kind {
			case "unknown without base":
				local = "UNKNOWN SOURCE\n"
			case "polluted equal base":
				local = "CUSTOM BASE\n"
				writeProjectFile(t, root, ".mulix/.installed/"+oldRel, local)
			case "multiple custom":
				local += "\nCUSTOM ONE\n"
				writeProjectFile(t, root, ".mulix/.installed/"+oldRel, string(base))
				rel2 := entryRel(host, "mulix-using-mulix")
				base2 := frozenEntry(t, host, "mulix-using-mulix")
				writeProjectFile(t, root, rel2, string(base2)+"\nCUSTOM TWO\n")
				writeProjectFile(t, root, ".mulix/.installed/"+rel2, string(base2))
			}
			writeProjectFile(t, root, oldRel, local)
			incoming := incomingEntry(t, host)
			if kind == "target collision" {
				writeProjectFile(t, root, incoming.Rel, "TARGET CUSTOM\n")
			}
			run := migrationRun(t, root, false)
			handled, ready, err := run.migrateEntry(host, incoming)
			if err != nil || !handled || ready || len(run.result.Blocked) != 1 {
				t.Fatalf("not blocked: %v %v %v %+v", handled, ready, err, run.result)
			}
			if got := readProjectFile(t, root, oldRel); got != local {
				t.Fatal("source changed")
			}
			if kind == "target collision" {
				if got := readProjectFile(t, root, incoming.Rel); got != "TARGET CUSTOM\n" {
					t.Fatal("target changed")
				}
			} else if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(incoming.Rel))); !os.IsNotExist(err) {
				t.Fatal("blocked family got a default canonical entry")
			}
		})
	}
}

func TestEntryMigration_TwoPristineAndExistingOfficialTarget(t *testing.T) {
	root := t.TempDir()
	host := HostClaude
	for _, alias := range entryAliases {
		writeProjectFile(t, root, entryRel(host, alias), string(frozenEntry(t, host, alias)))
	}
	incoming := incomingEntry(t, host)
	writeProjectFile(t, root, incoming.Rel, string(incoming.Data))
	run := migrationRun(t, root, false)
	handled, ready, err := run.migrateEntry(host, incoming)
	if err != nil || !handled || !ready {
		t.Fatalf("recovery failed: %v %v %v", handled, ready, err)
	}
	for _, alias := range entryAliases {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(entryRel(host, alias)))); !os.IsNotExist(err) {
			t.Fatal("pristine source survived")
		}
	}
}

func interruptedMigration(t *testing.T) (string, managedFile, string) {
	t.Helper()
	root := t.TempDir()
	host := HostClaude
	for _, alias := range entryAliases {
		writeProjectFile(t, root, entryRel(host, alias), string(frozenEntry(t, host, alias)))
	}
	incoming := incomingEntry(t, host)
	run := migrationRun(t, root, false)
	stopped := entryRel(host, entryAliases[1])
	realRemove := run.remove
	failure := errors.New("source cleanup interrupted")
	run.remove = func(full string) error {
		if full == filepath.Join(root, filepath.FromSlash(stopped)) {
			return failure
		}
		return realRemove(full)
	}
	if _, _, err := run.migrateEntry(host, incoming); !errors.Is(err, failure) {
		t.Fatalf("failure not surfaced: %v", err)
	}
	st, err := loadInstallState(root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Pending[incoming.Rel].Stage != "accepted-cleanup" {
		t.Fatal("cleanup checkpoint was lost")
	}
	return root, incoming, stopped
}

func TestEntryMigration_CheckpointResumesAndThenUpdatesNewVendor(t *testing.T) {
	root, incoming, _ := interruptedMigration(t)
	incoming.Data = append(append([]byte{}, incoming.Data...), []byte("\nNEXT VENDOR\n")...)
	run := migrationRun(t, root, false)
	handled, ready, err := run.migrateEntry(HostClaude, incoming)
	if err != nil || !handled || !ready {
		t.Fatalf("resume failed: %v %v %v", handled, ready, err)
	}
	if got := readProjectFile(t, root, incoming.Rel); got != string(incoming.Data) {
		t.Fatal("resume skipped newer N")
	}
	if len(run.state.Pending) != 0 {
		t.Fatal("checkpoint survived completed cleanup")
	}
}

func TestEntryMigration_ChangedTargetSourceOrBackupBlocksCheckpoint(t *testing.T) {
	for _, which := range []string{"target", "source", "backup", "baseline"} {
		t.Run(which, func(t *testing.T) {
			root, incoming, stopped := interruptedMigration(t)
			st, err := loadInstallState(root)
			if err != nil {
				t.Fatal(err)
			}
			switch which {
			case "target":
				writeProjectFile(t, root, incoming.Rel, "EDITED TARGET\n")
			case "source":
				writeProjectFile(t, root, stopped, "EDITED SOURCE\n")
			case "backup":
				writeProjectFile(t, root, st.Pending[incoming.Rel].BackupDir+"/local/"+stopped, "EDITED BACKUP\n")
			case "baseline":
				writeProjectFile(t, root, ".mulix/.installed/"+incoming.Rel, "EDITED BASELINE\n")
			}
			run := migrationRun(t, root, false)
			handled, ready, err := run.migrateEntry(HostClaude, incoming)
			if err != nil || !handled || ready || len(run.result.Blocked) != 1 {
				t.Fatalf("unsafe resume: %v %v %v %+v", handled, ready, err, run.result)
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(stopped))); err != nil {
				t.Fatal("remaining source was deleted")
			}
			if run.state.Pending[incoming.Rel].Stage != "accepted-cleanup" {
				t.Fatal("blocked checkpoint cleared")
			}
		})
	}
}

func TestEntryMigration_ForceRefreshesChangedSourceSnapshot(t *testing.T) {
	root, incoming, stopped := interruptedMigration(t)
	writeProjectFile(t, root, stopped, "LATER SOURCE EDIT\n")
	run := migrationRun(t, root, true)
	_, ready, err := run.migrateEntry(HostClaude, incoming)
	if err != nil || !ready {
		t.Fatalf("force recovery: %v %+v", err, run.result)
	}
	if got := readProjectFile(t, root, run.backups.dir+"/local/"+stopped); got != "LATER SOURCE EDIT\n" {
		t.Fatal("force used stale backup")
	}
}

func renameConflictFixture(t *testing.T) (string, *updateRun, managedFile, string) {
	t.Helper()
	root := t.TempDir()
	host := HostClaude
	source := entryRel(host, "using-mulix")
	base := frozenEntry(t, host, "using-mulix")
	needle := "mulix enforces a seven-phase workflow with two layers of enforcement, not"
	local := bytes.Replace(base, []byte(needle), []byte("LOCAL SAME LINE"), 1)
	if bytes.Equal(local, base) {
		t.Fatal("fixture did not edit a line")
	}
	writeProjectFile(t, root, source, string(local))
	writeProjectFile(t, root, ".mulix/.installed/"+source, string(base))
	incoming := incomingEntry(t, host)
	incoming.Data = bytes.Replace(incoming.Data, []byte(needle), []byte("INCOMING SAME LINE"), 1)
	run := migrationRun(t, root, false)
	handled, ready, err := run.migrateEntry(host, incoming)
	if err != nil || !handled || ready || len(run.result.Conflicted) != 1 {
		t.Fatalf("rename did not conflict: %v %v %v %+v", handled, ready, err, run.result)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(incoming.Rel))); !os.IsNotExist(err) {
		t.Fatal("rename wrote a conflicted active target")
	}
	return root, run, incoming, source
}

func TestEntryMigration_ConflictAcceptRequiresCanonicalNameAndKeepsResolution(t *testing.T) {
	root, run, incoming, source := renameConflictFixture(t)
	if run.state.Pending[incoming.Rel].Stage != "conflict" || len(run.state.Pending[incoming.Rel].Sources) != 1 {
		t.Fatal("rename conflict lost sources")
	}
	wrong := "---\nname: using-mulix\ndescription: fixture\n---\nRESOLUTION\n"
	writeProjectFile(t, root, incoming.Rel, wrong)
	accept := migrationRun(t, root, false)
	if err := accept.acceptPending(incoming.Rel, incoming); err == nil {
		t.Fatal("old frontmatter accepted under new path")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(source))); err != nil {
		t.Fatal("invalid acceptance removed source")
	}
	resolution := strings.Replace(wrong, "name: using-mulix", "name: mulix-using", 1)
	writeProjectFile(t, root, incoming.Rel, resolution)
	if err := accept.acceptPending(incoming.Rel, incoming); err != nil {
		t.Fatal(err)
	}
	if got := readProjectFile(t, root, incoming.Rel); got != resolution {
		t.Fatal("accept replaced manual resolution")
	}
	if got := readProjectFile(t, root, ".mulix/.installed/"+incoming.Rel); got != string(incoming.Data) {
		t.Fatal("rename accept polluted B")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(source))); !os.IsNotExist(err) {
		t.Fatal("accepted source survived")
	}
	if len(accept.state.Pending) != 0 {
		t.Fatal("accepted cleanup not completed")
	}
}

func TestEntryMigration_ConflictChangedSourcePreventsAcceptance(t *testing.T) {
	root, _, incoming, source := renameConflictFixture(t)
	writeProjectFile(t, root, incoming.Rel, "---\nname: mulix-using\ndescription: fixture\n---\nRESOLUTION\n")
	writeProjectFile(t, root, source, "LATER SOURCE EDIT\n")
	accept := migrationRun(t, root, false)
	if err := accept.acceptPending(incoming.Rel, incoming); err == nil {
		t.Fatal("accept removed a changed source")
	}
	if got := readProjectFile(t, root, source); got != "LATER SOURCE EDIT\n" {
		t.Fatal("changed source was deleted")
	}
	if accept.state.Pending[incoming.Rel].Stage != "conflict" {
		t.Fatal("invalid acceptance advanced checkpoint")
	}
}

func TestEntryMigration_CheckpointSaveFailureDoesNotCleanSources(t *testing.T) {
	root := t.TempDir()
	source := entryRel(HostClaude, "using-mulix")
	old := frozenEntry(t, HostClaude, "using-mulix")
	writeProjectFile(t, root, source, string(old))
	run := migrationRun(t, root, false)
	failure := errors.New("checkpoint save failed")
	run.saveState = func(*installState) error { return failure }
	incoming := incomingEntry(t, HostClaude)
	if _, _, err := run.migrateEntry(HostClaude, incoming); !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
	if got := readProjectFile(t, root, source); got != string(old) {
		t.Fatal("source cleaned before checkpoint saved")
	}
	if len(run.result.Migrated) != 0 {
		t.Fatal("failed migration reported completed")
	}
}

func TestEntryMigration_DSHBlockedBootstrapDoesNotPointAtMissingEntry(t *testing.T) {
	root := t.TempDir()
	source := entryRel(HostDSH, "using-mulix")
	writeProjectFile(t, root, source, "UNKNOWN LOCAL SOURCE\n")
	original := agentsBootstrapBegin + "\nOLD BOOTSTRAP\n" + agentsBootstrapEnd + "\n"
	writeProjectFile(t, root, "AGENTS.md", original)
	res, err := Update(UpdateOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if !res.NeedsAttention() {
		t.Fatal("unknown entry not blocked")
	}
	if got := readProjectFile(t, root, "AGENTS.md"); got != original {
		t.Fatal("bootstrap advanced despite missing canonical entry")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(entryRel(HostDSH, entrySkillName)))); !os.IsNotExist(err) {
		t.Fatal("default entry bypassed blocked family")
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
		t.Fatal("DSH migration installed Claude")
	}
}

func TestEntryMigration_CanonicalDSHFootprintWithoutMetadata(t *testing.T) {
	root := t.TempDir()
	files, err := managedFiles([]Host{HostDSH})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, incoming := range files {
		if incoming.Rel == entryRel(HostDSH, entrySkillName) {
			writeProjectFile(t, root, incoming.Rel, string(incoming.Data))
			found = true
			break
		}
	}
	if !found {
		t.Fatal("current bundle has no canonical entry")
	}
	res, err := Update(UpdateOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
		t.Fatal("canonical DSH footprint installed Claude")
	}
	if len(res.Blocked) > 0 {
		t.Fatalf("exact current entry blocked: %v", res.Blocked)
	}
}

func TestInit_MulixUsingCanonicalEntryOnly(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(entryRel(HostDSH, entrySkillName))))
	if err != nil || !strings.Contains(strings.ReplaceAll(string(data), "\r\n", "\n"), "name: mulix-using\n") {
		t.Fatalf("canonical missing: %v", err)
	}
	for _, alias := range entryAliases {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(entryRel(HostDSH, alias)))); !os.IsNotExist(err) {
			t.Fatal("fresh init installed an old alias")
		}
	}
}
