package cliutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/guard"
	"github.com/z-fenix/mulix-coding/internal/quality"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func (f trackerFixture) load() flow.State {
	f.t.Helper()
	s, err := state.Load(f.root, f.s.Change)
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f trackerFixture) stateBytes() string {
	f.t.Helper()
	return trackerRead(f.t, state.PathFor(f.root, f.s.Change))
}

func (f trackerFixture) calls(name string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.fake, name))
	if err != nil && !os.IsNotExist(err) {
		f.t.Fatal(err)
	}
	return string(b)
}

func (f trackerFixture) assertSyncBlocked(event string) {
	f.t.Helper()
	before, writes := f.stateBytes(), f.calls("posts")+f.calls("closes")
	if out, err := runPresetCLI(f.t, "guard", event); err == nil || !strings.Contains(err.Error(), "tracker sync") {
		f.t.Errorf("guard did not block %s on remote sync: %v\n%s", event, err, out)
	}
	if out, err := runPresetCLI(f.t, "state", "transition", event, "--force", "--reason", "recover fixture"); err == nil || !strings.Contains(err.Error(), "tracker sync") || strings.Contains(out, "readiness overridden") {
		f.t.Errorf("forced %s bypassed remote sync or ran readiness first: %v\n%s", event, err, out)
	}
	if f.stateBytes() != before || f.calls("posts")+f.calls("closes") != writes {
		f.t.Errorf("failed %s changed phase, readiness/evidence, or provider state", event)
	}
}

func (f trackerFixture) publishTasks() {
	f.t.Helper()
	f.success("publish", "tasks", "--expect", f.token("publish", "tasks", "--dry-run"))
}

func (f trackerFixture) writeTwoTasks() {
	f.t.Helper()
	qualityCLIFile(f.t, f.root, f.s.TasksDir+"/01-value.md", issueTaskText(1, "Implement Value", "code", "TestValue"))
	qualityCLIFile(f.t, f.root, f.s.TasksDir+"/02-existing.md", issueTaskText(2, "Check Existing", "verification", "TestExisting"))
	qualityCLIFile(f.t, f.root, quality.PolicyPath, issueTaskPolicy)
}

func TestRemoteSyncBlocksForcedAdvance(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		phase      flow.Phase
		event      flow.Event
		setup      func(trackerFixture)
	}{
		{"unverified target", "github", flow.PhaseSpecify, flow.EventSpecComplete, func(f trackerFixture) {}},
		{"verified but unpublished", "github", flow.PhaseSpecify, flow.EventSpecComplete, func(f trackerFixture) {
			s := f.load()
			s.Tracker.Target.ProjectID = "42"
			saveStateForTest(f.t, f.root, s)
		}},
		{"create is pending", "github", flow.PhaseSpecify, flow.EventSpecComplete, func(f trackerFixture) {
			token := f.token("publish", "spec", "--dry-run")
			f.t.Setenv("MULIX_TRACKER_WORKFLOW_LOST_NUMBER", "10")
			if _, err := f.run("publish", "spec", "--expect", token); err == nil || f.posts() != 1 {
				f.t.Fatal("fixture did not establish a pending create")
			}
		}},
		{"unreachable project", "github", flow.PhaseSpecify, flow.EventSpecComplete, func(f trackerFixture) {
			f.publishSpec()
			f.t.Setenv("MULIX_TRACKER_WORKFLOW_OFFLINE", "1")
		}},
		{"wrong GitLab pin despite numeric Issue still resolvable", "gitlab", flow.PhaseSpecify, flow.EventSpecComplete, func(f trackerFixture) {
			f.publishSpec()
			f.t.Setenv("MULIX_TRACKER_WORKFLOW_PROJECT", "77")
		}},
		{"missing Issue", "github", flow.PhaseSpecify, flow.EventSpecComplete, func(f trackerFixture) {
			f.publishSpec()
			f.setRemote(nil)
		}},
		{"Issue number reused", "github", flow.PhaseDesign, flow.EventDesignApproved, func(f trackerFixture) {
			f.publishSpec()
			issues := f.remote()
			issues[0].ID++
			f.setRemote(issues)
		}},
		{"remote drift during clarification", "github", flow.PhaseClarify, flow.EventClarifySkipped, func(f trackerFixture) {
			f.publishSpec()
			issues := f.remote()
			issues[0].Body += "\nNew requirement.\n"
			f.setRemote(issues)
		}},
		{"local drift during clarification", "github", flow.PhaseClarify, flow.EventClarifyComplete, func(f trackerFixture) {
			f.publishSpec()
			qualityCLIFile(f.t, f.root, f.s.SpecPath, fakeTrackerSpec+"\n## Clarifications\nUnpublished answers.\n")
		}},
		{"task drafts have no bindings", "github", flow.PhaseTasks, flow.EventTasksComplete, func(f trackerFixture) {
			f.publishSpec()
			f.writeTwoTasks()
		}},
		{"task binding is pending", "github", flow.PhaseTasks, flow.EventTasksComplete, func(f trackerFixture) {
			f.publishSpec()
			f.writeTwoTasks()
			s := f.load()
			s.Phase = flow.PhaseTasks
			saveStateForTest(f.t, f.root, s)
			token := f.token("publish", "tasks", "--dry-run")
			f.t.Setenv("MULIX_TRACKER_WORKFLOW_LOST_NUMBER", "12")
			if _, err := f.run("publish", "tasks", "--expect", token); err == nil || f.posts() != 3 || f.load().Tracker.Issues[2].Published() {
				f.t.Fatal("fixture did not leave the last slot pending")
			}
		}},
		{"extra published task whose file was removed", "github", flow.PhaseTasks, flow.EventTasksComplete, func(f trackerFixture) {
			f.publishSpec()
			f.writeTwoTasks()
			s := f.load()
			s.Phase = flow.PhaseTasks
			saveStateForTest(f.t, f.root, s)
			f.publishTasks()
			if err := os.Remove(filepath.Join(f.root, s.TasksDir, "02-existing.md")); err != nil {
				f.t.Fatal(err)
			}
		}},
		{"bound task moved with identical content", "gitlab", flow.PhaseTasks, flow.EventTasksComplete, func(f trackerFixture) {
			f.publishSpec()
			f.writeTwoTasks()
			s := f.load()
			s.Phase = flow.PhaseTasks
			saveStateForTest(f.t, f.root, s)
			f.publishTasks()
			if err := os.Rename(filepath.Join(f.root, s.TasksDir, "02-existing.md"), filepath.Join(f.root, s.TasksDir, "02-renamed.md")); err != nil {
				f.t.Fatal(err)
			}
		}},
		{"Build cannot audit an override past task drift", "github", flow.PhaseBuild, flow.EventBuildComplete, func(f trackerFixture) {
			f.publishSpec()
			f.writeTwoTasks()
			s := f.load()
			s.Phase = flow.PhaseTasks
			saveStateForTest(f.t, f.root, s)
			f.publishTasks()
			issues := f.remote()
			issues[2].Body += "\nUnreviewed edit.\n"
			f.setRemote(issues)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTrackerFixtureFor(t, tc.kind)
			tc.setup(f)
			s := f.load()
			if tc.phase != s.Phase || tc.phase == flow.PhaseClarify {
				s.Phase, s.DesignTrack, s.DesignApproved, s.ClarifySkipped = tc.phase, flow.TrackBounded, true, true
				saveStateForTest(t, f.root, s)
			}
			if tc.phase != flow.PhaseBuild && !guard.Run(f.root, s, tc.event).Passed() {
				t.Fatalf("fixture did not satisfy ordinary readiness: %+v", guard.Run(f.root, s, tc.event).Failures())
			}
			f.assertSyncBlocked(string(tc.event))
		})
	}
}

func TestRemoteSyncRejectsEditorSaveDuringRead(t *testing.T) {
	for _, args := range [][]string{{"guard", "clarify-complete"}, {"state", "transition", "clarify-complete"}, {"state", "transition", "clarify-complete", "--force", "--reason", "recovery"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			f := newTrackerFixture(t)
			f.publishSpec()
			s := f.load()
			s.Phase = flow.PhaseClarify
			saveStateForTest(t, f.root, s)
			before, writes := f.stateBytes(), f.calls("posts")+f.calls("closes")
			t.Setenv("MULIX_TRACKER_WORKFLOW_EDIT_ON_GET", filepath.Join(f.root, s.SpecPath))
			out, err := runPresetCLI(t, args...)
			if err == nil || !strings.Contains(err.Error(), "tracker sync") || strings.Contains(out, "readiness overridden") {
				t.Fatalf("editor save passed verified snapshot check: %v\n%s", err, out)
			}
			if f.stateBytes() != before || f.calls("posts")+f.calls("closes") != writes {
				t.Fatal("editor save changed state or wrote remotely")
			}
		})
	}
}

func TestRecheckTrackerInputsDetectsTaskSetChanges(t *testing.T) {
	for _, mutation := range []string{"edit", "rename", "add", "remove"} {
		t.Run(mutation, func(t *testing.T) {
			f := newTrackerFixture(t)
			f.writeTwoTasks()
			s := f.load()
			spec, err := quality.ReadProjectFile(f.root, s.SpecPath)
			if err != nil {
				t.Fatal(err)
			}
			tasks, err := quality.ReadTaskSnapshotInputs(s.QualityProject(f.root))
			if err != nil {
				t.Fatal(err)
			}
			verified := []verifiedTrackerSnapshot{{doc: quality.TaskDocument{Path: s.SpecPath, Markdown: spec}}}
			for _, doc := range tasks {
				verified = append(verified, verifiedTrackerSnapshot{doc: doc})
			}
			if err := recheckTrackerInputs(f.root, s, verified); err != nil {
				t.Fatal(err)
			}
			first := filepath.Join(f.root, filepath.FromSlash(tasks[0].Path))
			switch mutation {
			case "edit":
				err = os.WriteFile(first, append(tasks[0].Markdown, '\n'), 0644)
			case "rename":
				err = os.Rename(first, filepath.Join(filepath.Dir(first), "01-renamed.md"))
			case "add":
				err = os.WriteFile(filepath.Join(filepath.Dir(first), "03-new.md"), []byte("draft"), 0644)
			case "remove":
				err = os.Remove(first)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := recheckTrackerInputs(f.root, s, verified); err == nil {
				t.Fatalf("%s accepted", mutation)
			}
		})
	}
}

func TestRemoteSyncLeavesLocalReadinessOverridable(t *testing.T) {
	f := newTrackerFixture(t)
	f.publishSpec()
	f.writeTwoTasks()
	s := f.load()
	s.Phase = flow.PhaseTasks
	saveStateForTest(t, f.root, s)
	f.publishTasks()
	if err := os.Remove(filepath.Join(f.root, quality.PolicyPath)); err != nil {
		t.Fatal(err)
	}
	before := f.stateBytes()
	if out, err := runPresetCLI(t, "guard", "tasks-complete"); err == nil || strings.Contains(err.Error(), "tracker sync") || !strings.Contains(out, "quality policy") || f.stateBytes() != before {
		t.Errorf("missing policy should be reported as ordinary read-only readiness: %v\n%s", err, out)
	}
	issues := f.remote()
	drifted := append([]fakeHostedIssue(nil), issues...)
	drifted[2].Body += "Unapplied remote edit.\n"
	f.setRemote(drifted)
	f.assertSyncBlocked("tasks-complete")
	f.setRemote(issues)
	const reason = "recover the local fixture with missing policy"
	if out, err := runPresetCLI(t, "state", "transition", "tasks-complete", "--force", "--reason", reason); err != nil {
		t.Fatalf("synced snapshots should allow ordinary readiness recovery: %v\n%s", err, out)
	}
	s = f.load()
	if s.Phase != flow.PhaseBuild || len(s.GuardOverrides) != 1 || s.GuardOverrides[0].Reason != reason || len(s.QualityRuns) != 0 || f.posts() != 3 || f.calls("closes") != "" {
		t.Fatalf("ordinary override changed evidence or wrote to the tracker: phase=%s overrides=%v receipts=%d", s.Phase, s.GuardOverrides, len(s.QualityRuns))
	}
}

func TestWorkflowProtectsOnlyBoundSnapshots(t *testing.T) {
	f := newTrackerFixture(t)
	decide := func(tool, rel string, want bool) {
		t.Helper()
		payload := fmt.Appendf(nil, `{"tool_name":%q,"tool_input":{"file_path":%q}}`, tool, rel)
		d := evaluatePreToolUse(t.Context(), f.root, payload).Decision
		if d.Allow != want || !want && !strings.Contains(d.Next, "tracker apply") {
			t.Errorf("%s %s: got %+v, want allow=%v (or explicit sync guidance)", tool, rel, d, want)
		}
	}
	decide("Write", f.s.SpecPath, true)
	token := f.token("publish", "spec", "--dry-run")
	t.Setenv("MULIX_TRACKER_WORKFLOW_LOST_NUMBER", "10")
	if _, err := f.run("publish", "spec", "--expect", token); err == nil {
		t.Fatal("fixture did not leave draft pending")
	}
	decide("Edit", f.s.SpecPath, true)
	f.publishSpec() // reconcile without replacing the draft or another POST
	decide("Write", f.s.SpecPath, false)
	qualityCommand(t, "state", "transition", "spec-complete")
	decide("Edit", f.s.SpecPath, false)
	decide("Read", f.s.SpecPath, true)
	qualityCommand(t, "state", "set", "clarify_skipped", "true")
	qualityCommand(t, "state", "transition", "clarify-skipped")
	qualityCommand(t, "state", "set", "design_track", "bounded")
	qualityCommand(t, "state", "set", "design_approved", "true")
	qualityCommand(t, "state", "transition", "design-approved")
	f.writeTwoTasks()
	first, second := f.s.TasksDir+"/01-value.md", f.s.TasksDir+"/02-existing.md"
	decide("Edit", first, true)
	token = f.token("publish", "tasks", "--dry-run")
	t.Setenv("MULIX_TRACKER_WORKFLOW_LOST_NUMBER", "12")
	if _, err := f.run("publish", "tasks", "--expect", token); err == nil {
		t.Fatal("fixture did not leave second task pending")
	}
	decide("Edit", first, false)
	alias := f.s.TasksDir + "/01-VALUE.md"
	original := filepath.Join(f.root, filepath.FromSlash(first))
	aliased := filepath.Join(f.root, filepath.FromSlash(alias))
	_, statErr := os.Stat(aliased)
	madeLink := os.IsNotExist(statErr)
	if madeLink {
		// Model an alias that reaches the same file even on a case-sensitive FS.
		if err := os.Link(original, aliased); err != nil {
			t.Fatal(err)
		}
	} else if statErr != nil {
		t.Fatal(statErr)
	}
	decide("Edit", alias, false)
	if madeLink {
		if err := os.Remove(aliased); err != nil {
			t.Fatal(err)
		}
		qualityCLIFile(t, f.root, alias, "This file is a distinct draft.")
		decide("Write", alias, true)
	}
	decide("Write", second, true)
	decide("Write", f.s.TasksDir+"/03-another-draft.md", true)
	decide("Read", first, true)
}

func TestIssueCloseUsesTaskReceiptAndArchive(t *testing.T) {
	for _, kind := range []string{"github", "gitlab"} {
		t.Run(kind, func(t *testing.T) {
			host := "claude"
			if kind == "gitlab" {
				host = "dsh"
			}
			f := fakeTrackerProject(t)
			qualityCommand(t, "_setup", "--host", host, "--tracker", kind, "--hostname", "tracker.example.test", "--project", "team/project")
			if _, err := os.Lstat(filepath.Join(f.root, "."+host, "skills", "mulix-taskstoissues", "SKILL.md")); !os.IsNotExist(err) {
				t.Fatalf("new installation shipped the obsolete exporter: %v", err)
			}
			qualityCommand(t, "new", "Deliver value", "--global")
			id, err := state.Active(f.root)
			if err != nil || id == "" {
				t.Fatalf("new change not selected: %q %v", id, err)
			}
			f.s.Change = id
			f.s = f.load()
			t.Setenv("MULIX_TRACKER_WORKFLOW_CHANGE", id)
			qualityCLIFile(t, f.root, f.s.SpecPath, fakeTrackerSpec)
			// Test both a retained session binding and global fallback after archive.
			qualityCommand(t, "context", "mode", "session")
			if kind == "github" {
				qualityCommand(t, "state", "select", f.s.Change)
			}
			deny := func(args ...string) {
				t.Helper()
				before, writes := f.stateBytes(), f.calls("closes")
				if out, err := f.run(append([]string{"close"}, args...)...); err == nil || trackerTestToken.MatchString(out) {
					t.Fatalf("ineligible/unauthorized close %v: %v\n%s", args, err, out)
				}
				if f.stateBytes() != before || f.calls("closes") != writes {
					t.Fatalf("rejected close %v changed archive/evidence or attempted a write", args)
				}
			}
			f.publishSpec()
			deny("spec", "--change", f.s.Change, "--dry-run")
			deny("spec", "--change", f.s.Change, "--expect", strings.Repeat("a", 64))
			qualityCommand(t, "state", "transition", "spec-complete")
			issues := f.remote()
			issues[0].Body += "\n## Clarifications\n\n- Exactly two, for all callers.\n"
			f.setRemote(issues)
			token := f.token("preview", "spec")
			f.success("apply", "spec", "--expect", token)
			qualityCommand(t, "guard", "clarify-complete")
			qualityCommand(t, "state", "transition", "clarify-complete")
			qualityCommand(t, "state", "set", "design_track", "bounded")
			qualityCommand(t, "state", "set", "design_approved", "true")
			qualityCommand(t, "state", "transition", "design-approved")
			f.writeTwoTasks()
			f.publishTasks()
			deny("task-1", "--dry-run")
			qualityCommand(t, "state", "transition", "tasks-complete")
			qualityCommand(t, "state", "set", "execution_method", "inline")
			qualityCLIFile(t, f.root, "go.mod", "module fixture\n\ngo 1.27.0\n")
			qualityCLIFile(t, f.root, "value.go", "package fixture\nfunc Value() int {return 0}\n")
			qualityCLIFile(t, f.root, "value_test.go", `package fixture
import "testing"
func TestValue(t *testing.T) { if Value()!=2 { t.Fatal("Value must equal two") } }
func TestExisting(t *testing.T) { if Value()!=2 { t.Fatal("Existing caller requires two") } }
func TestFinal(t *testing.T) { if Value()+1!=3 { t.Fatal("Composed result must be three") } }
`)
			deny("task-1", "--dry-run")
			deny("task-1", "--expect", strings.Repeat("b", 64))
			realTaskEvidence(t, f.root)
			workspace := ".mulix/.runtime/" + f.s.Change + "/sdd/issues/"
			ledger := "# SDD progress — plan: " + f.s.TasksDir + "\n"
			receipt := func(n string) string {
				return strings.TrimSpace(qualityCommand(t, "test", "receipt", "--task", n, "--id-only"))
			}
			line := func(n int, id string) string { return fmt.Sprintf("- Task %d: complete (receipt %s)\n", n, id) }
			one := receipt("1")
			qualityCLIFile(t, f.root, workspace+"progress.md", ledger+"- Task 1: complete\n"+line(2, one))
			deny("task-1", "--dry-run") // legacy completion or wrong task number is not enough
			ledger += line(1, one)
			qualityCLIFile(t, f.root, workspace+"progress.md", ledger)
			qualityCLIFile(t, f.root, workspace+"plan-path", "somewhere-else/issues\n")
			deny("task-1", "--dry-run")
			qualityCLIFile(t, f.root, workspace+"plan-path", f.s.TasksDir+"\n")
			before := f.stateBytes()
			view := f.success("close", "task-1", "--dry-run")
			m := trackerTestToken.FindStringSubmatch(view)
			if m == nil || !strings.Contains(view, one) || !strings.Contains(view, "#11") || !strings.Contains(view, "project ID 42") || f.calls("closes") != "" || f.stateBytes() != before {
				t.Fatalf("close preview did not identify the exact authorized operation without writing:\n%s", view)
			}
			deny("task-1")
			deny("task-1", "--dry-run", "--expect", m[1])
			currentSource := trackerRead(t, filepath.Join(f.root, "value.go"))
			qualityCLIFile(t, f.root, "value.go", currentSource+"\n// different source after approval\n")
			deny("task-1", "--expect", m[1])
			qualityCLIFile(t, f.root, "value.go", currentSource)
			issues = f.remote()
			var changed []fakeHostedIssue
			for i, slot := range []string{"spec", "task-1", "task-2"} {
				changed = append([]fakeHostedIssue(nil), issues...)
				changed[i].Body += "\nEdit after approval.\n"
				f.setRemote(changed)
				writes := f.calls("closes")
				if out, err := f.run("close", "task-1", "--expect", m[1]); err == nil || f.calls("closes") != writes {
					t.Errorf("drifted %s allowed a task close against the old receipt: %v %s", slot, err, out)
				}
				f.setRemote(issues)
			}
			if t.Failed() {
				return
			}
			// A subsequent passing attempt still requires its own marker and approval.
			qualityCommand(t, "test", "run", "--task", "1", "--stage", "green")
			deny("task-1", "--expect", m[1])
			ledger = strings.Replace(ledger, line(1, one), line(1, receipt("1")), 1)
			qualityCLIFile(t, f.root, workspace+"progress.md", ledger)
			deny("task-1", "--expect", m[1])
			// Neither task 2's receipt nor closure is required to close task 1.
			f.success("close", "task-1", "--expect", f.token("close", "task-1", "--dry-run"))
			if issues := f.remote(); issues[0].Status != "open" || issues[1].Status != "closed" || issues[2].Status != "open" {
				t.Fatalf("task close affected the wrong Issue: %+v", issues)
			}
			qualityCommand(t, "test", "run", "--task", "2", "--stage", "check")
			qualityCLIFile(t, f.root, workspace+"progress.md", ledger+line(2, receipt("2")))
			deny("task-2", "--expect", m[1])
			closeAttempts := 1
			if kind == "github" {
				f.success("close", "task-2", "--expect", f.token("close", "task-2", "--dry-run"))
				closeAttempts++
			} // Leave the other provider's qualified task open; closure is optional.
			qualityCommand(t, "guard", "build-complete")
			qualityCommand(t, "state", "transition", "build-complete")
			qualityCLIFile(t, f.root, f.s.ReportPath, "# Fixture review\n```mulix-review\nreviewer: test fixture reviewer\nresult: approved\nblocking: []\n```\n")
			qualityCommand(t, "verify")
			issues = f.remote()
			changed = append([]fakeHostedIssue(nil), issues...)
			changed[2].Body += "\nOut-of-band change.\n"
			f.setRemote(changed)
			f.assertSyncBlocked("verify-pass")
			f.setRemote(issues)
			qualityCommand(t, "state", "transition", "verify-pass")
			qualityCommand(t, "state", "set", "archive_confirmation", "confirmed")
			deny("spec", "--change", f.s.Change, "--dry-run") // phase Archive is not yet archived
			f.setRemote(changed)
			f.assertSyncBlocked("archived")
			f.setRemote(issues)
			qualityCommand(t, "state", "transition", "archived")
			if active, _ := state.Active(f.root); active != "" || !f.load().Archived || f.remote()[0].Status != "open" {
				t.Fatal("archive did not finish locally or closed the Issue implicitly")
			}
			deny("task-1", "--change", f.s.Change, "--dry-run")
			before = f.stateBytes()
			token = f.token("close", "spec", "--change", f.s.Change, "--dry-run")
			if kind == "github" {
				// A token for an archived record is not a route around an active
				// binding to another change.
				qualityCommand(t, "new", "Other work", "--session-id", "different-session")
				deny("spec", "--change", f.s.Change, "--session-id", "different-session", "--expect", token)
			}
			changed = append([]fakeHostedIssue(nil), issues...)
			changed[0].ID++ // never close a replacement at the same number
			f.setRemote(changed)
			deny("spec", "--change", f.s.Change, "--expect", token)
			f.setRemote(issues)
			t.Setenv("MULIX_TRACKER_WORKFLOW_CLOSE_FAIL", "yes")
			if out, err := f.run("close", "spec", "--change", f.s.Change, "--expect", token); err == nil || strings.Contains(fmt.Sprint(err)+out, "fake-sensitive-token") || strings.Count(f.calls("closes"), "\n") != closeAttempts+1 {
				t.Fatalf("expected sanitized provider failure after explicit spec authorization: %v\n%s", err, out)
			}
			if f.stateBytes() != before || f.remote()[0].Status != "open" {
				t.Fatal("provider failure modified the archived state or Issue")
			}
			t.Setenv("MULIX_TRACKER_WORKFLOW_CLOSE_FAIL", "")
			f.success("close", "spec", "--change", f.s.Change, "--expect", f.token("close", "spec", "--change", f.s.Change, "--dry-run"))
			if f.stateBytes() != before || f.remote()[0].Status != "closed" || f.posts() != 3 {
				t.Fatal("post-archive closure changed archived state or performed something other than closure")
			}
			if kind == "gitlab" && f.remote()[2].Status != "open" {
				t.Fatal("another task was closed without authorization")
			}
		})
	}
}

func TestTrackerVerifyFailRemainsLocalRecovery(t *testing.T) {
	for _, phase := range []flow.Phase{flow.PhaseVerify, flow.PhaseArchive} {
		t.Run(string(phase), func(t *testing.T) {
			f := newTrackerFixture(t)
			s := f.load()
			s.Phase, s.VerifyResult = phase, flow.VerifyFail
			saveStateForTest(t, f.root, s)
			t.Setenv("MULIX_TRACKER_WORKFLOW_OFFLINE", "yes")
			qualityCommand(t, "guard", "verify-fail")
			qualityCommand(t, "state", "transition", "verify-fail")
			if got := f.load(); got.Phase != flow.PhaseBuild || got.VerifyFailures != 1 || f.calls("calls") != "" {
				t.Fatalf("local recovery failed or called remote: %+v, %s", got, f.calls("calls"))
			}
		})
	}
}

func TestTrackerLocalLayoutsContinueWithoutProvider(t *testing.T) {
	f := newTrackerFixture(t)
	t.Setenv("MULIX_TRACKER_WORKFLOW_OFFLINE", "yes")
	legacy := setupQualityCLI(t, f.root, "legacy")
	legacy.Phase = flow.PhaseSpecify
	saveStateForTest(t, f.root, legacy)
	markdown := setupIssueQualityCLI(t, f.root, "local", flow.PhaseSpecify)
	for _, item := range []struct{ change, kind string }{{legacy.Change, "legacy"}, {markdown.Change, "markdown"}, {f.s.Change, "github"}} {
		change := item.change
		qualityCommand(t, "state", "select", change)
		if out := qualityCommand(t, "state", "show"); !strings.Contains(out, "tracker:       "+item.kind+"\n") {
			t.Errorf("change %s has no tracker mode for the phase skill: %s", change, out)
		}
		if change == f.s.Change {
			f.assertSyncBlocked("spec-complete")
			continue
		}
		qualityCommand(t, "guard", "spec-complete")
		qualityCommand(t, "state", "transition", "spec-complete")
		if _, err := f.run("close", "spec", "--change", change, "--dry-run"); err == nil {
			t.Fatal("local layouts must not offer Issue closure")
		}
	}
	if f.calls("calls") != "" {
		t.Fatal("local layouts or an unverified target invoked a remote")
	}
}
