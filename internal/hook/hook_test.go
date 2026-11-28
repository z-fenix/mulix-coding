package hook

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/z-fenix/mulix-coding/internal/flow"
)

func req(tool, path string) Request {
	return Request{HookEventName: "PreToolUse", ToolName: tool, ToolInput: ToolInput{FilePath: path}}
}

func TestRuntimeIssueArtifactWritesPerPhase(t *testing.T) {
	root := t.TempDir()
	var s flow.State
	// Decode a future on-disk fixture without depending on new Go symbols.
	if err := yaml.Unmarshal([]byte("change: example\nartifact_layout: runtime-issues-v1\nspec_path: .mulix/.runtime/example/spec.md\ntasks_dir: .mulix/.runtime/example/issues\nreport_path: .mulix/.runtime/example/report.md\n"), &s); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		phase flow.Phase
		path  string
		allow bool
	}{
		{flow.PhaseSpecify, ".mulix/.runtime/example/spec.md", true},
		{flow.PhaseClarify, ".mulix/.runtime/example/spec.md", true},
		{flow.PhaseTasks, ".mulix/.runtime/example/issues/1.md", true},
		{flow.PhaseTasks, ".mulix/.runtime/example/issues/nested/1.md", false},
		{flow.PhaseTasks, ".mulix/.runtime/example/issues/1.txt", false},
		{flow.PhaseVerify, ".mulix/.runtime/example/report.md", true},
		{flow.PhaseVerify, ".mulix/.runtime/example/spec.md", false},
		{flow.PhaseSpecify, ".mulix/.runtime/other/spec.md", false},
		{flow.PhaseSpecify, ".mulix/.runtime/example/state.yaml", false},
		{flow.PhaseTasks, "docs/changes/example/tasks.md", false},
		{flow.PhaseArchive, ".mulix/.runtime/example/report.md", false},
	}
	for _, tool := range []string{"Write", "Edit"} {
		for _, tc := range cases {
			s.Phase = tc.phase
			if d := Decide(root, s, req(tool, tc.path)); d.Allow != tc.allow {
				t.Errorf("%s %s %s: got %+v want allow=%t", tool, tc.phase, tc.path, d, tc.allow)
			}
		}
	}
	legacy := flow.New("legacy", "")
	legacy.Phase = flow.PhaseTasks
	if d := Decide(root, legacy, req("Write", "docs/changes/legacy/tasks.md")); !d.Allow {
		t.Errorf("legacy tasks blocked: %+v", d)
	}
}

func TestRuntimeIssueArtifactsRejectSymlinkAliases(t *testing.T) {
	root := t.TempDir()
	own := filepath.Join(root, ".mulix", ".runtime", "example")
	if err := os.MkdirAll(filepath.Join(own, "issues"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(own, "issues"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(own, "issues", "outside")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), filepath.Join(own, "spec.md")); err != nil {
		t.Fatal(err)
	}
	s := flow.New("example", "")
	s.ArtifactLayout = flow.RuntimeIssuesLayout
	for _, tc := range []struct {
		phase flow.Phase
		p     string
	}{
		{flow.PhaseTasks, "alias/task.md"}, {flow.PhaseTasks, ".mulix/.runtime/example/issues/outside/task.md"},
		{flow.PhaseSpecify, ".mulix/.runtime/example/spec.md"}, {flow.PhaseBuild, ".mulix/.runtime/example/issues/outside/task.md"},
		{flow.PhaseVerify, ".mulix/.runtime/example/issues/../report.md"},
	} {
		s.Phase = tc.phase
		if d := Decide(root, s, req("Write", tc.p)); d.Allow {
			t.Errorf("%s allowed alias %s", tc.phase, tc.p)
		}
	}
}

func TestBuildRejectsDanglingAliasesIntoOtherRuntime(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".mulix", ".runtime", "other"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, target string }{
		{"relative-alias", ".mulix/.runtime/other/missing.md"},
		{"absolute-alias", filepath.Join(root, ".mulix", ".runtime", "other", "not-created.md")},
	} {
		if err := os.Symlink(tc.target, filepath.Join(root, tc.name)); err != nil {
			t.Fatal(err)
		}
	}
	s := flow.New("example", "")
	s.Phase = flow.PhaseBuild
	for _, name := range []string{"relative-alias", "absolute-alias"} {
		if d := Decide(root, s, req("Write", name)); d.Allow {
			t.Errorf("dangling alias %s to another change allowed", name)
		}
	}
	if d := Decide(root, s, req("Write", "src/new.go")); !d.Allow {
		t.Errorf("ordinary build path blocked: %+v", d)
	}
}

func TestDecide_NonGatedToolAlwaysAllowed(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseSpecify

	d := Decide("/repo", s, req("Bash", "src/main.go"))
	if !d.Allow {
		t.Fatalf("expected Bash to be allowed unconditionally, got: %+v", d)
	}
}

func TestDecide_SpecifyPhaseBlocksSourceWrite(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseSpecify

	d := Decide("/repo", s, req("Write", "src/main.go"))
	if d.Allow {
		t.Fatal("expected source write to be blocked during specify phase")
	}
}

func TestDecide_SpecifyPhaseAllowsSpecDirWrite(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseSpecify

	d := Decide("/repo", s, req("Write", "docs/specs/add-login/notes.md"))
	if !d.Allow {
		t.Fatalf("expected write under the spec dir to be allowed, got: %+v", d)
	}
}

func TestDecide_TasksPhaseAllowsChangeDirWrite(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseTasks
	s.SpecPath = "docs/specs/add-login/spec.md"

	d := Decide("/repo", s, req("Write", "docs/changes/add-login/tasks.md"))
	if !d.Allow {
		t.Fatalf("expected write under the change dir to be allowed, got: %+v", d)
	}
}

func TestDecide_TasksPhaseBlocksSpecDirWrite(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseTasks
	s.SpecPath = "docs/specs/add-login/spec.md"

	d := Decide("/repo", s, req("Write", "docs/specs/add-login/spec.md"))
	if d.Allow {
		t.Fatal("expected the spec dir to no longer be writable once past clarify")
	}
}

func TestDecide_ClarifyPhaseAllowsSpecDirWrite(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseClarify
	s.SpecPath = "docs/specs/add-login/spec.md"

	d := Decide("/repo", s, req("Write", "docs/specs/add-login/spec.md"))
	if !d.Allow {
		t.Fatalf("expected clarify to still be able to edit spec.md, got: %+v", d)
	}
}

func TestDecide_BuildPhaseIsUnrestricted(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseBuild

	d := Decide("/repo", s, req("Write", "src/main.go"))
	if !d.Allow {
		t.Fatalf("expected source write to be allowed during build phase, got: %+v", d)
	}
}

func TestDecide_ArchivePhaseBlocksAllWrites(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseArchive

	d := Decide("/repo", s, req("Write", "docs/specs/add-login/spec.md"))
	if d.Allow {
		t.Fatal("expected archive phase to block all writes")
	}
}

func TestDecide_StateFileNeverDirectlyWritable(t *testing.T) {
	for _, phase := range flow.Phases {
		s := flow.New("add-login", "")
		s.Phase = phase
		d := Decide("/repo", s, req("Edit", ".mulix/.runtime/add-login/state.yaml"))
		if d.Allow {
			t.Fatalf("expected state.yaml writes to be blocked in phase %q", phase)
		}
	}
}

func TestDecide_DesignPhaseWritesOnlyRuntimeDesignDirs(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseDesign
	s.SpecPath = "docs/specs/add-login/spec.md"

	for _, p := range []string{
		".mulix/.runtime/add-login/specs/2026-01-01-login-design.md",
	} {
		if d := Decide("/repo", s, req("Write", p)); !d.Allow {
			t.Fatalf("expected %s to be writable in design, got: %+v", p, d)
		}
	}
	for _, p := range []string{
		".mulix/.runtime/add-login/brainstorm/123-456/content/layout.html",
		".mulix/.runtime/add-login/sdd/tasks/progress.md",
		"docs/changes/add-login/tasks.md",
		"docs/specs/add-login/spec.md",
		"src/main.go",
	} {
		if d := Decide("/repo", s, req("Write", p)); d.Allow {
			t.Fatalf("expected %s to be blocked in design", p)
		}
	}
}

func TestDecide_BuildPhaseAllowsSddWorkspaceAndFreezesDesign(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseBuild

	if d := Decide("/repo", s, req("Write", ".mulix/.runtime/add-login/sdd/tasks/task-1-report.md")); !d.Allow {
		t.Fatalf("expected the sdd workspace to be writable during build, got: %+v", d)
	}
	if d := Decide("/repo", s, req("Write", ".mulix/.runtime/add-login/specs/design.md")); d.Allow {
		t.Fatal("expected the approved design doc to be frozen once build starts")
	}
}

func TestDecide_OtherChangesRuntimeDirBlocked(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseBuild

	if d := Decide("/repo", s, req("Write", ".mulix/.runtime/other/sdd/tasks/progress.md")); d.Allow {
		t.Fatal("expected another change's runtime dir to be blocked")
	}
	// segment-aware: add-login-2 is not add-login
	if d := Decide("/repo", s, req("Write", ".mulix/.runtime/add-login-2/sdd/x.md")); d.Allow {
		t.Fatal("expected a prefix-sharing change id not to match")
	}
}

func TestDecide_RuntimeEvidenceDirectories(t *testing.T) {
	for _, phase := range flow.Phases {
		for _, tool := range []string{"Write", "Edit"} {
			for _, dir := range []string{"verification", "migrations"} {
				t.Run(string(phase)+"/"+tool+"/"+dir, func(t *testing.T) {
					s := flow.New("add-login", "")
					s.Phase = phase
					want := false
					p := ".mulix/.runtime/add-login/" + dir + "/run-1/manifest.json"
					if d := Decide("/repo", s, req(tool, p)); d.Allow != want {
						t.Fatalf("%s in %s: want allow=%t, got %+v", p, phase, want, d)
					}
					for _, blocked := range []string{
						".mulix/.runtime/other/" + dir + "/manifest.json",
						".mulix/.runtime/add-login-2/" + dir + "/manifest.json",
						".mulix/.runtime/add-login/" + dir + "-other/manifest.json",
						".mulix/.runtime/add-login/" + dir + "/../state.yaml",
					} {
						if d := Decide("/repo", s, req(tool, blocked)); d.Allow {
							t.Errorf("expected %s to remain blocked in %s", blocked, phase)
						}
					}
				})
			}
		}
	}
}

func TestDecide_SharedRuntimeDirIsNotWritable(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseArchive

	if d := Decide("/repo", s, req("Write", ".mulix/.runtime/_shared/diagnosing-superpowers/abc/report.md")); d.Allow {
		t.Fatalf("legacy shared runtime must not be a new output destination: %+v", d)
	}
}

func TestDecide_SpecifyPhaseBlocksUnrelatedChangeDir(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseSpecify

	d := Decide("/repo", s, req("Write", "docs/specs/other-change/spec.md"))
	if d.Allow {
		t.Fatal("expected write under a different change's spec dir to be blocked")
	}
}

func TestDecide_PrefixMatchIsSegmentAware(t *testing.T) {
	s := flow.New("1", "")
	s.Phase = flow.PhaseSpecify // specDir falls back to docs/specs/1

	// docs/specs/1-other must NOT match prefix docs/specs/1.
	d := Decide("/repo", s, req("Write", "docs/specs/1-other/spec.md"))
	if d.Allow {
		t.Fatal("expected segment-aware prefix match to reject docs/specs/1-other for spec dir docs/specs/1")
	}
}

func TestDecide_AbsolutePathIsMadeRelativeToRoot(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseSpecify

	root := t.TempDir()
	d := Decide(root, s, req("Write", filepath.Join(root, "docs", "specs", "add-login", "notes.md")))
	if !d.Allow {
		t.Fatalf("expected absolute path under spec dir to be allowed, got: %+v", d)
	}
}

func TestRender_AllowHasNoReason(t *testing.T) {
	data, err := Render(Decision{Allow: true})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got := string(data); !contains(got, `"permissionDecision":"allow"`) {
		t.Fatalf("expected allow decision in output, got %s", got)
	}
}

func TestExitCode(t *testing.T) {
	if ExitCode(Decision{Allow: true}) != 0 {
		t.Fatal("expected exit code 0 for allow")
	}
	if ExitCode(Decision{Allow: false}) != 2 {
		t.Fatal("expected exit code 2 for block")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
