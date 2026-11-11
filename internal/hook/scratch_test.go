package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

func TestDecideContext_ScratchAndNoChangePolicy(t *testing.T) {
	root := t.TempDir()
	scratch := filepath.Join(os.TempDir(), "mulix-scratch-"+strings.Repeat("a", 64))
	foreign := filepath.Join(os.TempDir(), "mulix-scratch-"+strings.Repeat("b", 64))
	for _, phase := range flow.Phases {
		s := flow.New("X", "")
		s.Phase = phase
		r := sessionctx.Resolved{Root: root, Mode: sessionctx.Session, Change: "X", Phase: phase, State: &s}
		data, err := json.Marshal(map[string]string{"scratch_dir": scratch})
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &r); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			path string
			want bool
		}{
			{filepath.Join(scratch, "brainstorm", "content", "screen.html"), phase == flow.PhaseDesign},
			{filepath.Join(scratch, "verification", "tests.log"), phase == flow.PhaseBuild || phase == flow.PhaseVerify},
			{filepath.Join(scratch, "migrations", "manifest.json"), phase == flow.PhaseBuild || phase == flow.PhaseVerify},
			{filepath.Join(scratch, "diagnosing-superpowers", "session", "report.md"), true},
			{filepath.Join(scratch, "verification-other", "tests.log"), false},
			{filepath.Join(scratch, "tests.log"), false},
			{filepath.Join(scratch+"-other", "verification", "tests.log"), false},
			{filepath.Join(foreign, "verification", "tests.log"), false},
			{filepath.Join(root, ".mulix", ".runtime", "X", "state.yaml"), false},
		} {
			for _, tool := range []string{"Write", "Edit"} {
				t.Run(string(phase)+"/"+tool+"/"+tc.path, func(t *testing.T) {
					if d := DecideContext(root, r, req(tool, tc.path)); d.Allow != tc.want {
						t.Fatalf("want allow=%t, got %+v", tc.want, d)
					}
				})
			}
		}
	}
	for _, mode := range []sessionctx.Mode{sessionctx.Legacy, sessionctx.Session} {
		r := sessionctx.Resolved{Root: root, Mode: mode}
		for _, p := range []string{".mulix/.runtime/_shared/report.md", ".mulix/.runtime/X/sdd/tasks/progress.md", ".mulix/.runtime/X/state.yaml"} {
			if d := DecideContext(root, r, req("Write", p)); d.Allow {
				t.Errorf("no-change %s allowed runtime write %s", mode, p)
			}
		}
		if d := DecideContext(root, r, req("Write", "src/main.go")); !d.Allow {
			t.Errorf("no-change ordinary write changed: %+v", d)
		}
	}
}

// Symlink resolution must not change which context owns a reserved scratch root.
func TestDecideContext_ScratchSymlinkOwnership(t *testing.T) {
	for _, scenario := range []string{"own-root-alias", "foreign-root-escape", "outside-alias", "child-escape"} {
		t.Run(scenario, func(t *testing.T) {
			root, temp, outside := t.TempDir(), t.TempDir(), t.TempDir()
			for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
				t.Setenv(key, temp)
			}
			own := filepath.Join(temp, "mulix-scratch-"+strings.Repeat("a", 64))
			foreign := filepath.Join(temp, "mulix-scratch-"+strings.Repeat("b", 64))
			link := func(target, name string) {
				t.Helper()
				if err := os.Symlink(target, name); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			mkdir := func(p string) {
				t.Helper()
				if err := os.MkdirAll(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			var paths []string
			s := flow.New("X", "")
			s.Phase = flow.PhaseBuild
			switch scenario {
			case "own-root-alias":
				mkdir(filepath.Join(foreign, "brainstorm"))
				link(foreign, own)
				s.Phase = flow.PhaseDesign
				paths = []string{filepath.Join(own, "brainstorm", "screen.html"), filepath.Join(foreign, "brainstorm", "screen.html")}
			case "foreign-root-escape":
				link(outside, foreign)
				paths = []string{filepath.Join(foreign, "verification", "tests.log")}
			case "outside-alias":
				mkdir(foreign)
				alias := filepath.Join(outside, "alias")
				link(foreign, alias)
				paths = []string{filepath.Join(alias, "verification", "tests.log")}
			case "child-escape":
				mkdir(filepath.Join(own, "verification"))
				mkdir(foreign)
				link(foreign, filepath.Join(own, "verification", "alias"))
				paths = []string{filepath.Join(own, "verification", "alias", "tests.log")}
			}
			r := sessionctx.Resolved{Root: root, ScratchDir: own, Mode: sessionctx.Session, Change: "X", Phase: s.Phase, State: &s}
			for _, tool := range []string{"Write", "Edit"} {
				for _, p := range paths {
					if d := DecideContext(root, r, req(tool, p)); d.Allow {
						t.Errorf("%s accepted scratch ownership alias %s: %+v", tool, p, d)
					}
				}
			}
			if scenario != "own-root-alias" {
				if d := DecideContext(root, r, req("Write", filepath.Join(own, "verification", "local.log"))); !d.Allow {
					t.Errorf("ordinary owned scratch write changed: %+v", d)
				}
			}
		})
	}
}
