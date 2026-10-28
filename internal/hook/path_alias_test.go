package hook

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

func TestDecideContext_WindowsTrailingAliasesNeverReachControlMetadata(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows filename aliases")
	}
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "existing"}[existing], func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, ".mulix"), 0700); err != nil {
				t.Fatal(err)
			}
			if existing {
				if err := os.MkdirAll(filepath.Join(root, ".mulix", "sessions"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, ".mulix", "active"), []byte("X\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			s := flow.New("X", "")
			s.Phase = flow.PhaseBuild
			r := sessionctx.Resolved{Root: root, Mode: sessionctx.Session, Change: "X", Phase: s.Phase, State: &s}
			for _, absolute := range []bool{false, true} {
				for _, path := range []string{".mulix/sessions./other.json", ".mulix/sessions /other.json", ".mulix/active.", ".mulix/active ", ".mulix/context.json.", ".mulix/.locks./context.lock"} {
					target := filepath.FromSlash(path)
					if absolute {
						target = filepath.Join(root, target)
					}
					if d := DecideContext(root, r, Request{ToolName: "Write", ToolInput: ToolInput{FilePath: target}}); d.Allow {
						t.Fatalf("metadata alias allowed: %q", target)
					}
				}
			}
		})
	}
}

func TestDecideContext_RelativeAliasHasSamePolicyAsAbsolute(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".mulix"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mulix", "active"), []byte("X\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, ".mulix"), filepath.Join(root, "metadata-alias")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	s := flow.New("X", "")
	s.Phase = flow.PhaseBuild
	r := sessionctx.Resolved{Root: root, Mode: sessionctx.Session, Change: "X", Phase: s.Phase, State: &s}
	for _, path := range []string{filepath.Join(root, "metadata-alias", "active"), "metadata-alias/active"} {
		if d := DecideContext(root, r, Request{ToolName: "Edit", ToolInput: ToolInput{FilePath: path}}); d.Allow {
			t.Fatalf("relative/absolute alias allowed: %s", path)
		}
	}
}
