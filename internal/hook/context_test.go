package hook

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

func TestDecideContext_WindowsAliasesAndCaseUseSamePolicy(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path identity")
	}
	raw := t.TempDir()
	if err := os.MkdirAll(filepath.Join(raw, ".mulix", "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(raw)
	if err != nil {
		t.Fatal(err)
	}
	s := flow.New("X", "")
	s.Phase = flow.PhaseBuild
	r := sessionctx.Resolved{Root: root, Mode: sessionctx.Session, Change: "X", Phase: s.Phase, State: &s}
	for _, p := range []string{".MULIX/SESSIONS/a.json", ".mulix/active", ".mulix/.runtime/X/STATE.YAML"} {
		d := DecideContext(root, r, Request{ToolName: "Write", ToolInput: ToolInput{FilePath: filepath.Join(raw, filepath.FromSlash(p))}})
		if d.Allow {
			t.Fatalf("control alias allowed: %s (raw=%s root=%s)", p, raw, root)
		}
	}
	s.Phase = flow.PhaseSpecify
	r.Phase = s.Phase
	d := DecideContext(root, r, Request{ToolName: "Write", ToolInput: ToolInput{FilePath: filepath.Join(raw, "docs", "specs", "X", "spec.md")}})
	if !d.Allow {
		t.Fatalf("own spec through root alias denied: %+v", d)
	}
}
