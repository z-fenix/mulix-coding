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

func TestDecide_PlanPhaseAllowsChangeDirWrite(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhasePlan
	s.SpecPath = "docs/specs/add-login/spec.md"

	d := Decide("/repo", s, req("Write", "docs/changes/add-login/plan.md"))
	if !d.Allow {
		t.Fatalf("expected write under the change dir to be allowed, got: %+v", d)
	}
}

func TestDecide_PlanPhaseBlocksSpecDirWrite(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhasePlan
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
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseBuild // even the unrestricted phase

	d := Decide("/repo", s, req("Edit", "docs/changes/add-login/.runtime/state.yaml"))
	if d.Allow {
		t.Fatal("expected state file writes to always be blocked")
	}
}

func TestDecide_BuildPhaseAllowsSddSubdirWrite(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseBuild

	d := Decide("/repo", s, req("Write", "docs/changes/add-login/.runtime/sdd/plan.md"))
	if !d.Allow {
		t.Fatalf("expected .runtime/sdd/ to be writable during build (subagent-dispatch artifacts), got: %+v", d)
	}
}

func TestDecide_BuildPhaseStillBlocksStateFileInsideRuntimeSdd(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhaseBuild

	d := Decide("/repo", s, req("Edit", "docs/changes/add-login/.runtime/state.yaml"))
	if d.Allow {
		t.Fatal("expected state.yaml to stay blocked even though .runtime/sdd/ is now writable")
	}
}

func TestDecide_NonBuildPhaseStillBlocksRuntimeSdd(t *testing.T) {
	s := flow.New("add-login", "")
	s.Phase = flow.PhasePlan
	s.SpecPath = "docs/specs/add-login/spec.md"

	d := Decide("/repo", s, req("Write", "docs/changes/add-login/.runtime/sdd/plan.md"))
	if d.Allow {
		t.Fatal("expected .runtime/sdd/ to stay blocked outside the build phase")
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
