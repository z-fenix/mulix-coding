package cliutil

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/state"
	"github.com/z-fenix/mulix-coding/internal/tracker"
	"gopkg.in/yaml.v3"
)

func TestNewPinsArtifactLayoutAndTracker(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	create := func(title string) string {
		t.Helper()
		if out, err := runPresetCLI(t, "new", title); err != nil {
			t.Fatalf("new %s: %v %s", title, err, out)
		}
		id, err := state.Active(root)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	load := func(id string) map[string]any {
		t.Helper()
		b, err := os.ReadFile(state.PathFor(root, id))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := yaml.Unmarshal(b, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	old := create("legacy")
	oldBefore, err := os.ReadFile(state.PathFor(root, old))
	if err != nil {
		t.Fatal(err)
	}
	a := tracker.Target{Kind: "github", Host: "github.example", Project: "team/first"}
	b := tracker.Target{Kind: "gitlab", Host: "gitlab.example", Project: "team/second", ProjectID: "42"}
	if err := tracker.SaveConfig(root, a); err != nil {
		t.Fatal(err)
	}
	x := create("configured x")
	xBefore, err := os.ReadFile(state.PathFor(root, x))
	if err != nil {
		t.Fatal(err)
	}
	if err := tracker.SaveConfig(root, b); err != nil {
		t.Fatal(err)
	}
	y := create("configured y")
	for _, tc := range []struct{ id, kind, host, project string }{{x, "github", "github.example", "team/first"}, {y, "gitlab", "gitlab.example", "team/second"}} {
		m := load(tc.id)
		if m["artifact_layout"] != "runtime-issues-v1" {
			t.Errorf("%s layout = %v, want runtime-issues-v1", tc.id, m["artifact_layout"])
			continue
		}
		base := ".mulix/.runtime/" + tc.id + "/"
		for k, want := range map[string]string{"spec_path": base + "spec.md", "tasks_dir": base + "issues", "report_path": base + "report.md"} {
			if m[k] != want {
				t.Errorf("%s %s=%v want %s", tc.id, k, m[k], want)
			}
		}
		binding, ok := m["tracker"].(map[string]any)
		if !ok {
			t.Errorf("%s missing tracker", tc.id)
			continue
		}
		target, ok := binding["target"].(map[string]any)
		if !ok || target["kind"] != tc.kind || target["host"] != tc.host || target["project"] != tc.project {
			t.Errorf("%s target=%v", tc.id, binding["target"])
		}
		if tc.kind == "gitlab" && target["project_id"] != "42" {
			t.Errorf("%s project_id=%v", tc.id, target["project_id"])
		}
		if key, ok := binding["remote_key"].(string); !ok || key == "" {
			t.Errorf("%s lacks remote key", tc.id)
		}
		for _, p := range []string{base + "spec.md", base + "report.md"} {
			if _, err := os.Lstat(filepath.Join(root, p)); !os.IsNotExist(err) {
				t.Errorf("artifact prematurely created %s: %v", p, err)
			}
		}
		if info, err := os.Stat(filepath.Join(root, base, "issues")); err != nil || !info.IsDir() {
			t.Errorf("issues directory missing: %v", err)
		}
		for _, p := range []string{"docs/specs/" + tc.id, "docs/changes/" + tc.id} {
			if _, err := os.Lstat(filepath.Join(root, p)); !os.IsNotExist(err) {
				t.Errorf("unexpected old directory %s: %v", p, err)
			}
		}
	}
	if after, _ := os.ReadFile(state.PathFor(root, old)); string(after) != string(oldBefore) {
		t.Error("old change was rewritten")
	}
	if after, _ := os.ReadFile(state.PathFor(root, x)); string(after) != string(xBefore) {
		t.Error("reconfiguration rewrote x")
	}
	if out, err := runPresetCLI(t, "state", "select", x); err != nil {
		t.Fatalf("select x: %s %v", out, err)
	}
	for _, field := range []string{"artifact_layout", "tracker", "tasks_dir", "spec_path", "tasks_path", "report_path"} {
		if out, err := runPresetCLI(t, "state", "set", field, "elsewhere"); err == nil {
			t.Errorf("set %s unexpectedly succeeded: %s", field, out)
		}
	}
	out, err := runPresetCLI(t, "state", "show")
	if err != nil || !strings.Contains(out, "runtime-issues-v1") || !strings.Contains(out, "tasks_dir:") {
		t.Errorf("state show missing layout/tasks dir: %s %v", out, err)
	}
}

func TestNewRuntimeCollisionAndPlanPath(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	if err := tracker.SaveConfig(root, tracker.Target{Kind: "markdown"}); err != nil {
		t.Fatal(err)
	}
	candidate := time.Now().Format("2006-01-02") + "-collision"
	if err := os.MkdirAll(filepath.Join(root, "docs", "specs", candidate), 0755); err != nil {
		t.Fatal(err)
	}
	if out, err := runPresetCLI(t, "new", "collision"); err != nil {
		t.Fatalf("new: %s %v", out, err)
	}
	id, err := state.Active(root)
	if err != nil {
		t.Fatal(err)
	}
	if id != candidate+"-2" {
		t.Fatalf("collision allocated %s", id)
	}
	s, err := state.Load(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.PlanPath(); got != ".mulix/.runtime/"+id+"/issues" {
		t.Errorf("plan path %s", got)
	}
	legacy := flow.New("legacy", "")
	legacy.TasksPath = "docs/changes/legacy/tasks.md"
	if got := legacy.PlanPath(); got != "docs/changes/legacy/tasks.md" {
		t.Errorf("legacy plan path %s", got)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "changes", id)); !os.IsNotExist(err) {
		t.Errorf("unexpected legacy change dir: %v", err)
	}
}

func TestNewRejectsMalformedTrackerBeforeAllocation(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	if err := os.WriteFile(filepath.Join(root, ".mulix", "tracker.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, "new", "invalid config"); err == nil {
		t.Fatal("malformed tracker accepted")
	}
	if _, err := os.Stat(filepath.Join(root, ".mulix", ".runtime")); !os.IsNotExist(err) {
		t.Fatalf("allocated runtime with bad config: %v", err)
	}
}

func TestPendingProjectIdentityPinsOnceUnderChangeLock(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	if err := tracker.SaveConfig(root, tracker.Target{Kind: "gitlab", Host: "gitlab.example", Project: "team/pending"}); err != nil {
		t.Fatal(err)
	}
	if out, err := runPresetCLI(t, "new", "pending identity"); err != nil {
		t.Fatalf("new: %s %v", out, err)
	}
	id, _ := state.Active(root)
	updateID := func(value string) error {
		_, err := state.Update(context.Background(), root, id, func(s *flow.State) error { s.Tracker.Target.ProjectID = value; return nil }, nil)
		return err
	}
	for _, invalid := range []string{"0", "00", "01", "not-a-provider-id"} {
		if err := updateID(invalid); err == nil {
			t.Errorf("invalid provider ID %q accepted", invalid)
		}
	}
	pending, err := state.Load(root, id)
	if err != nil || pending.Tracker.Target.ProjectID != "" {
		t.Errorf("invalid ID changed pending state: %+v %v", pending.Tracker, err)
	}
	for _, mutate := range []func(*flow.State){
		func(s *flow.State) { s.Tracker.Target.Host = "other.example" },
		func(s *flow.State) { s.Tracker.RemoteKey = strings.Repeat("a", 32) },
	} {
		if _, err := state.Update(context.Background(), root, id, func(s *flow.State) error { mutate(s); return nil }, nil); err == nil {
			t.Error("accepted change to selector or binding key")
		}
	}
	if err := updateID("456"); err != nil {
		t.Fatalf("pin verified project ID: %v", err)
	}
	for _, invalid := range []string{"789", ""} {
		if err := updateID(invalid); err == nil {
			t.Errorf("accepted ID replacement %q", invalid)
		}
	}
	got, err := state.Load(root, id)
	if err != nil || got.Tracker.Target.ProjectID != "456" {
		t.Fatalf("pinned ID changed: %+v %v", got.Tracker, err)
	}
}

func TestNewPinnedBindingCannotBeReplacedBySave(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	if err := tracker.SaveConfig(root, tracker.Target{Kind: "github", Host: "github.example", Project: "team/first"}); err != nil {
		t.Fatal(err)
	}
	if out, err := runPresetCLI(t, "new", "immutable binding"); err != nil {
		t.Fatalf("new: %s %v", out, err)
	}
	id, _ := state.Active(root)
	original, err := state.Load(root, id)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(state.PathFor(root, id))
	changed := original
	binding := *original.Tracker
	binding.Target.Project = "team/second"
	changed.Tracker = &binding
	if err := state.Save(root, changed); err == nil {
		t.Error("saved a different pinned tracker")
	}
	changed = original
	changed.ArtifactLayout, changed.TasksDir, changed.Tracker = "", "", nil
	if err := state.Save(root, changed); err == nil {
		t.Error("saved a downgrade to legacy")
	}
	if after, _ := os.ReadFile(state.PathFor(root, id)); string(before) != string(after) {
		t.Error("rejected save changed state")
	}
}

func TestNewRejectsInvalidPinnedState(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	if err := tracker.SaveConfig(root, tracker.Target{Kind: "markdown"}); err != nil {
		t.Fatal(err)
	}
	if out, err := runPresetCLI(t, "new", "invalid pin"); err != nil {
		t.Fatalf("new: %s %v", out, err)
	}
	id, _ := state.Active(root)
	p := state.PathFor(root, id)
	original, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		field string
		value any
	}{
		{"artifact_layout", "unknown-layout"}, {"spec_path", "docs/specs/wrong/spec.md"}, {"tasks_dir", ".mulix/.runtime/other/issues"}, {"tasks_path", "docs/changes/other/tasks.md"}, {"report_path", "elsewhere.md"}, {"tracker", nil},
	} {
		t.Run(tc.field, func(t *testing.T) {
			var m map[string]any
			if err := yaml.Unmarshal(original, &m); err != nil {
				t.Fatal(err)
			}
			m[tc.field] = tc.value
			data, err := yaml.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, data, 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := state.Load(root, id); err == nil {
				t.Error("invalid pinned state loaded")
			}
		})
	}
	if err := os.WriteFile(p, original, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestNew_DateChangeActiveStateAndDirectoriesAgree(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	out, err := runPresetCLI(t, "new", "Add Login Flow")
	if err != nil {
		t.Fatalf("new: %v %s", err, out)
	}
	change, err := state.Active(root)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-add-login-flow$`).MatchString(change) {
		t.Fatalf("not dated: %q", change)
	}
	s, err := state.Load(root, change)
	if err != nil || s.Change != change || s.Phase != flow.PhaseSpecify {
		t.Fatalf("state mismatch: %+v %v", s, err)
	}
	if s.SpecPath != "" || s.TasksPath != "" || s.ReportPath != "" {
		t.Fatal("new unexpectedly pre-populated artifact paths")
	}
	for field, path := range map[string]string{"spec_path": "docs/specs/" + change + "/spec.md", "tasks_path": "docs/changes/" + change + "/tasks.md", "report_path": "docs/changes/" + change + "/report.md"} {
		if out, err := runPresetCLI(t, "state", "set", field, path); err != nil {
			t.Fatalf("set %s: %v %s", field, err, out)
		}
	}
	s, err = state.Load(root, change)
	if err != nil || s.SpecPath != "docs/specs/"+change+"/spec.md" || s.TasksPath != "docs/changes/"+change+"/tasks.md" || s.ReportPath != "docs/changes/"+change+"/report.md" {
		t.Fatalf("artifact names changed: %+v %v", s, err)
	}
	for _, dir := range []string{"docs/specs/" + change, "docs/changes/" + change, ".mulix/.runtime/" + change} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil || !info.IsDir() {
			t.Fatalf("missing %s: %v", dir, err)
		}
	}
	out, err = runPresetCLI(t, "new", "Add Login Flow")
	if err != nil {
		t.Fatalf("second new: %v %s", err, out)
	}
	second, err := state.Active(root)
	if err != nil || second != change+"-2" {
		t.Fatalf("second id=%q: %v", second, err)
	}
}

func TestNew_LegacyNumberedChangeCanStillSelectAndTransition(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	old := flow.New("001-legacy", "")
	old.SpecPath = "docs/specs/001-legacy/spec.md"
	if err := state.Save(root, old); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, "new", "New Work"); err != nil {
		t.Fatal(err)
	}
	out, err := runPresetCLI(t, "state", "select", old.Change)
	if err != nil {
		t.Fatalf("select legacy: %v %s", err, out)
	}
	active, err := state.Active(root)
	if err != nil || active != old.Change {
		t.Fatalf("active=%q %v", active, err)
	}
	full := filepath.Join(root, filepath.FromSlash(old.SpecPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("# Legacy specification\nFR-001: Preserve legacy selection.\n```mulix-acceptance\ncriteria:\n  - id: AC-001\n    requirement: FR-001\n    expected: The legacy change can transition to clarify.\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = runPresetCLI(t, "state", "transition", "spec-complete")
	if err != nil {
		t.Fatalf("legacy transition: %v %s", err, out)
	}
	loaded, err := state.Load(root, old.Change)
	if err != nil || loaded.Phase != flow.PhaseClarify {
		t.Fatalf("legacy phase: %+v %v", loaded, err)
	}
	data, err := os.ReadFile(state.PathFor(root, old.Change))
	if err != nil || !strings.Contains(string(data), "mulix.state.v2") {
		t.Fatalf("schema changed: %v", err)
	}
}
