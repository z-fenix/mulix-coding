package hook

import (
	"testing"

	"github.com/mulix-dev/mulix-coding/internal/flow"
)

func req(tool, path string) Request {
	return Request{HookEventName: "PreToolUse", ToolName: tool, ToolInput: ToolInput{FilePath: path}}
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
		".mulix/.runtime/add-login/brainstorm/123-456/content/layout.html",
	} {
		if d := Decide("/repo", s, req("Write", p)); !d.Allow {
			t.Fatalf("expected %s to be writable in design, got: %+v", p, d)
		}
	}
	for _, p := range []string{
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

func TestDecide_BuildPhaseAllowsSddWorkspaceOnly(t *testing.T) {
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

func TestDecide_SharedRuntimeDirAlwaysWritable(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseArchive

	if d := Decide("/repo", s, req("Write", ".mulix/.runtime/_shared/diagnosing-superpowers/abc/report.md")); !d.Allow {
		t.Fatalf("expected .mulix/.runtime/_shared/ to be writable in any phase, got: %+v", d)
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

	d := Decide(`D:\repo`, s, req("Write", `D:\repo\docs\specs\add-login\notes.md`))
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
