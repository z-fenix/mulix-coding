package cliutil

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/state"
)

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
	if err := os.WriteFile(full, []byte("# Legacy specification\n"), 0o644); err != nil {
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
