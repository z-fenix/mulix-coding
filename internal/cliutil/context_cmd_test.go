package cliutil

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func sessionProject(t *testing.T) (string, *sessionctx.Store) {
	t.Helper()
	t.Setenv("MULIX_SESSION_ID", "")
	_ = os.Unsetenv("MULIX_SESSION_ID")
	root := chdirTemp(t)
	initMulixRoot(t, root)
	for _, name := range []string{"X", "Y"} {
		saveStateForTest(t, root, flow.New(name, ""))
	}
	s, err := sessionctx.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(context.Background(), sessionctx.Session); err != nil {
		t.Fatal(err)
	}
	if err := state.SetActive(root, "X"); err != nil {
		t.Fatal(err)
	}
	return s.Root(), s
}
func selectForSession(t *testing.T, s *sessionctx.Store, id, change string) {
	t.Helper()
	target, err := s.PrepareSelection(context.Background(), sessionctx.Identity{Value: id, Present: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Select(context.Background(), target, change); err != nil {
		t.Fatal(err)
	}
}

func TestSessionIdentity_ExplicitFlagOverridesEnv(t *testing.T) {
	for _, tc := range []struct {
		flag    string
		present bool
		env     string
		set     bool
		want    string
		fail    bool
	}{
		{"A", true, "B", true, "A", false}, {"", true, "B", true, "", true}, {"", false, "", true, "", true}, {"", false, "", false, "", false}, {"", false, "B", true, "B", false},
	} {
		cmd := NewRootCmd()
		if tc.present {
			if err := cmd.PersistentFlags().Set("session-id", tc.flag); err != nil {
				t.Fatal(err)
			}
		}
		got, err := sessionIdentity(cmd, func(string) (string, bool) { return tc.env, tc.set })
		if (err != nil) != tc.fail || (!tc.fail && (got.Value != tc.want || got.Present != (tc.present || tc.set))) {
			t.Fatalf("%+v: got=%+v err=%v", tc, got, err)
		}
	}
}

func TestContextCLI_ShowFormatsAndEmptyContext(t *testing.T) {
	root, _ := sessionProject(t)
	out, err := runPresetCLI(t, "context", "show", "--json", "--session-id", "B")
	if err != nil {
		t.Fatal(err)
	}
	var r sessionctx.Resolved
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	if r.Source != sessionctx.GlobalFallback || r.Change != "X" || r.Mode != sessionctx.Session {
		t.Fatalf("context: %+v", r)
	}
	out, err = runPresetCLI(t, "context", "show", "--field", "runtime-dir")
	if err != nil || out != filepath.Join(root, ".mulix", ".runtime", "X")+"\n" {
		t.Fatalf("machine output %q %v", out, err)
	}
	if _, err := runPresetCLI(t, "context", "show", "--json", "--field", "runtime-dir"); err == nil {
		t.Fatal("accepted conflicting formats")
	}
	if err := state.ClearActive(root); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, "context", "show", "--json"); err != nil {
		t.Fatal("empty context should be inspectable:", err)
	}
	out, err = runPresetCLI(t, "context", "show", "--field", "runtime-dir")
	if err == nil || strings.TrimSpace(out) != "" {
		t.Fatalf("no change produced a runtime path: %q %v", out, err)
	}
	out, err = runPresetCLI(t, "context", "show", "--field", "scratch-dir")
	if err != nil || !filepath.IsAbs(strings.TrimSpace(out)) {
		t.Fatalf("empty context scratch: %q %v", out, err)
	}
	if _, err := os.Lstat(strings.TrimSpace(out)); !os.IsNotExist(err) {
		t.Fatalf("scratch query wrote files: %v", err)
	}
}

func TestContextCLI_RelativeTempRootProducesNoPath(t *testing.T) {
	_, _ = sessionProject(t)
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, "relative temporary files")
	}
	if filepath.IsAbs(os.TempDir()) {
		t.Skip("host expands relative temporary roots")
	}
	for _, args := range [][]string{{"--field", "scratch-dir"}, {"--json"}} {
		out, err := runPresetCLI(t, append([]string{"context", "show"}, args...)...)
		if err == nil || strings.TrimSpace(out) != "" || !strings.Contains(err.Error(), "absolute") {
			t.Errorf("invalid temp root produced usable CLI output: %q %v", out, err)
		}
	}
}

func TestContextCLI_UnbindReportsRemovalBeforeFallbackError(t *testing.T) {
	root, s := sessionProject(t)
	selectForSession(t, s, "A", "Y")
	p := filepath.Join(root, ".mulix", "sessions", fmt.Sprintf("%x.json", sha256.Sum256([]byte("A"))))
	if err := os.WriteFile(filepath.Join(root, ".mulix", "active"), []byte("bad/active"), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := runPresetCLI(t, "context", "unbind", "--session-id", "A")
	if err == nil || !strings.Contains(out, "removed=true") {
		t.Fatalf("partial result lost: %q %v", out, err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("binding was not removed:", err)
	}
}

func TestNewSelect_SessionLegacyAndGlobalMatrix(t *testing.T) {
	for _, mode := range []sessionctx.Mode{sessionctx.Legacy, sessionctx.Session} {
		for _, id := range []string{"", "A"} {
			for _, global := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/%v", mode, id, global), func(t *testing.T) {
					root, s := sessionProject(t)
					if err := s.SetMode(context.Background(), mode); err != nil {
						t.Fatal(err)
					}
					args := []string{"state", "select", "Y"}
					if id != "" {
						args = append(args, "--session-id", id)
					}
					if global {
						args = append(args, "--global")
					}
					out, err := runPresetCLI(t, args...)
					if err != nil {
						t.Fatalf("%s %v", out, err)
					}
					want := "Y"
					if mode == sessionctx.Session && id != "" && !global {
						want = "X"
					}
					active, err := state.Active(root)
					if err != nil || active != want {
						t.Fatalf("active=%q want=%q %v", active, want, err)
					}
					got, err := s.Resolve(context.Background(), sessionctx.ResolveRequest{Identity: sessionctx.Identity{Value: id, Present: id != ""}})
					if err != nil || got.Change != "Y" {
						t.Fatalf("selection: %+v %v", got, err)
					}
				})
			}
		}
	}
}

func TestNewSelect_NewBindsWithoutChangingGlobal(t *testing.T) {
	root, s := sessionProject(t)
	out, err := runPresetCLI(t, "new", "Add Login", "--session-id", "A")
	if err != nil {
		t.Fatalf("new: %s %v", out, err)
	}
	got, err := s.Resolve(context.Background(), sessionctx.ResolveRequest{Identity: sessionctx.Identity{Value: "A", Present: true}})
	if err != nil || got.Change == "X" || !strings.Contains(got.Change, "add-login") {
		t.Fatalf("new binding: %+v %v", got, err)
	}
	active, _ := state.Active(root)
	if active != "X" {
		t.Fatal("new changed global:", active)
	}
}

func TestNewSelect_FailedBindingReportsCreatedChange(t *testing.T) {
	root, _ := sessionProject(t)
	p := filepath.Join(root, ".mulix", "sessions", fmt.Sprintf("%x.json", sha256.Sum256([]byte("A"))))
	if err := os.MkdirAll(p, 0700); err != nil {
		t.Fatal(err)
	}
	out, err := runPresetCLI(t, "new", "Partial New", "--session-id", "A")
	if err == nil || !strings.Contains(out, "created change") {
		t.Fatalf("partial new: %q %v", out, err)
	}
	changes, err := state.List(root)
	if err != nil || len(changes) != 3 {
		t.Fatalf("created change removed: %v %v", changes, err)
	}
}

func TestContextCLI_RepairDoesNotLoadInvalidBinding(t *testing.T) {
	root, _ := sessionProject(t)
	t.Setenv("MULIX_SESSION_ID", "")
	if _, err := runPresetCLI(t, "version"); err != nil {
		t.Fatal(err)
	}
	if _, err := runPresetCLI(t, "context", "mode", "legacy"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".mulix", "context.json"))
	if err != nil || !strings.Contains(string(data), "legacy") {
		t.Fatalf("mode repair: %s %v", data, err)
	}
}
