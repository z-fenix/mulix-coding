package cliutil

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/hook"
	"github.com/z-fenix/mulix-coding/internal/lockfile"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func hookPayload(t *testing.T, root, id, tool, path string) []byte {
	t.Helper()
	m := map[string]any{"hook_event_name": "PreToolUse", "cwd": root, "tool_name": tool, "tool_input": map[string]string{"file_path": path}}
	if id != "" {
		m["session_id"] = id
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestHookEval_SessionBindingAndDynamicFallback(t *testing.T) {
	root, s := sessionProject(t)
	x, _ := state.Load(root, "X")
	x.Phase = flow.PhaseDesign
	saveStateForTest(t, root, x)
	y, _ := state.Load(root, "Y")
	y.Phase = flow.PhaseBuild
	saveStateForTest(t, root, y)
	selectForSession(t, s, "A", "X")
	selectForSession(t, s, "B", "Y")
	for _, tc := range []struct {
		id    string
		allow bool
	}{{"A", false}, {"B", true}, {"C", false}} {
		out := evaluatePreToolUse(context.Background(), root, hookPayload(t, root, tc.id, "Write", filepath.Join(root, "src", "code.go")))
		if out.Decision.Allow != tc.allow {
			t.Fatalf("%s: %+v", tc.id, out)
		}
	}
	if err := state.SetActive(root, "Y"); err != nil {
		t.Fatal(err)
	}
	out := evaluatePreToolUse(context.Background(), root, hookPayload(t, root, "C", "Edit", "src/code.go"))
	if !out.Decision.Allow || out.Resolved.Source != sessionctx.GlobalFallback {
		t.Fatalf("fallback: %+v", out)
	}
}
func TestHookEval_InvalidBindingNeverFallsBack(t *testing.T) {
	root, s := sessionProject(t)
	selectForSession(t, s, "A", "X")
	y, _ := state.Load(root, "Y")
	y.Phase = flow.PhaseBuild
	saveStateForTest(t, root, y)
	_ = state.SetActive(root, "Y")
	p := filepath.Join(root, ".mulix", "sessions", fmt.Sprintf("%x.json", sha256.Sum256([]byte("A"))))
	if err := os.WriteFile(p, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	out := evaluatePreToolUse(context.Background(), root, hookPayload(t, root, "A", "Write", "src/code.go"))
	if out.Decision.Allow || out.Cause == nil || out.Resolved.Source != sessionctx.SessionBinding {
		t.Fatalf("fell back: %+v", out)
	}
	for _, tc := range []struct{ tool, path string }{{"Bash", "src/code.go"}, {"Write", ""}} {
		out := evaluatePreToolUse(context.Background(), root, hookPayload(t, root, "A", tc.tool, tc.path))
		if !out.Decision.Allow {
			t.Fatalf("expanded tool scope: %+v", out)
		}
	}
}
func TestHookEval_NoActiveAndLegacyErrors(t *testing.T) {
	root, s := sessionProject(t)
	_ = state.ClearActive(root)
	payload := hookPayload(t, root, "B", "Write", "src/code.go")
	if out := evaluatePreToolUse(context.Background(), root, payload); !out.Decision.Allow {
		t.Fatal(out)
	}
	_ = state.SetActive(root, "missing")
	if out := evaluatePreToolUse(context.Background(), root, payload); out.Decision.Allow {
		t.Fatal("bad active allowed", out)
	}
	if err := s.SetMode(context.Background(), sessionctx.Legacy); err != nil {
		t.Fatal(err)
	}
	if out := evaluatePreToolUse(context.Background(), root, payload); !out.Decision.Allow || out.Cause == nil {
		t.Fatal("legacy fail-open lost", out)
	}
	if err := os.WriteFile(filepath.Join(root, ".mulix", "context.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if out := evaluatePreToolUse(context.Background(), root, payload); out.Decision.Allow {
		t.Fatal("bad config downgraded", out)
	}
}
func TestHookEval_RequestCWDWinsAndMissingCWDUsesProcess(t *testing.T) {
	root, _ := sessionProject(t)
	outside := t.TempDir()
	if out := evaluatePreToolUse(context.Background(), outside, hookPayload(t, root, "B", "Write", "src/code.go")); out.Decision.Allow || out.Resolved.Root != root {
		t.Fatalf("request cwd ignored: %+v", out)
	}
	if out := evaluatePreToolUse(context.Background(), root, hookPayload(t, "", "B", "Write", "src/code.go")); out.Decision.Allow {
		t.Fatal("missing cwd did not use process", out)
	}
	t.Setenv("MULIX_SESSION_ID", "other")
	out := evaluatePreToolUse(context.Background(), root, hookPayload(t, root, "B", "Write", "src/code.go"))
	if out.Resolved.SessionID != "B" {
		t.Fatal("env overrode hook identity", out)
	}
	out = evaluatePreToolUse(context.Background(), root, hookPayload(t, filepath.Join(outside, "missing"), "B", "Write", "src/code.go"))
	if out.Resolved.Root == root {
		t.Fatal("invalid cwd switched project", out)
	}
}
func TestHookEval_MalformedAndNullIdentityKnownSession(t *testing.T) {
	root, _ := sessionProject(t)
	x, _ := state.Load(root, "X")
	x.Phase = flow.PhaseBuild
	saveStateForTest(t, root, x)
	for _, payload := range [][]byte{[]byte("{"), []byte(`{"session_id":null,"tool_name":"Write","tool_input":{"file_path":"x"}}`), []byte(`{"session_id":"","tool_name":"Write","tool_input":{"file_path":"x"}}`)} {
		if out := evaluatePreToolUse(context.Background(), root, payload); out.Decision.Allow {
			t.Fatalf("malformed session allowed: %+v", out)
		}
	}
}
func TestHookEval_LockTimeoutBlocks(t *testing.T) {
	root, _ := sessionProject(t)
	ready, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- lockfile.With(context.Background(), root, lockfile.ContextKey, func() error { close(ready); <-release; return nil })
	}()
	<-ready
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	out := evaluatePreToolUse(ctx, root, hookPayload(t, root, "A", "Write", "src/code.go"))
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if out.Decision.Allow || !errors.Is(out.Cause, lockfile.ErrTimeout) {
		t.Fatalf("lock failure opened gate: %+v", out)
	}
}
func TestDecideContext_ProtectsControlPathsBeforeBuildAllowance(t *testing.T) {
	root, _ := sessionProject(t)
	for _, empty := range []bool{false, true} {
		r := sessionctx.Resolved{Root: root, Mode: sessionctx.Session}
		if !empty {
			s := flow.New("X", "")
			s.Phase = flow.PhaseBuild
			r.State = &s
			r.Change = "X"
			r.Phase = s.Phase
		}
		for _, path := range []string{".mulix/context.json", ".mulix/context.json/x", ".mulix/active", ".mulix/sessions", ".mulix/sessions/a.json", ".mulix/.locks/context.lock"} {
			req := hook.Request{ToolName: "Write", ToolInput: hook.ToolInput{FilePath: filepath.Join(root, filepath.FromSlash(path))}}
			if d := hook.DecideContext(root, r, req); d.Allow {
				t.Fatalf("control file allowed: %s empty=%v", path, empty)
			}
		}
	}
}

type failingHookWriter struct{}

func (failingHookWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestEmitHook_DenyRenderFailureStillBlocks(t *testing.T) {
	badRender := func(hook.Decision) ([]byte, error) { return nil, errors.New("encode failed") }
	if code := emitHook(&bytes.Buffer{}, hook.Decision{Allow: false}, true, badRender); code != 2 {
		t.Fatal("render fail-open", code)
	}
	if code := emitHook(failingHookWriter{}, hook.Decision{Allow: false}, true, hook.Render); code != 2 {
		t.Fatal("writer fail-open", code)
	}
}
