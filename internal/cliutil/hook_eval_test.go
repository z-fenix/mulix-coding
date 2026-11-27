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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/hook"
	"github.com/z-fenix/mulix-coding/internal/lockfile"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
	"github.com/z-fenix/mulix-coding/internal/tracker"
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
func TestHookCLIHelper(t *testing.T) {
	if os.Getenv("MULIX_TEST_HOOK_PROCESS") != "1" {
		return
	}
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"hook"})
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestHookCLILegacyModeRejectsInvalidNativeArtifacts(t *testing.T) {
	root := chdirTemp(t)
	initMulixRoot(t, root)
	if err := tracker.SaveConfig(root, tracker.Target{Kind: "markdown"}); err != nil {
		t.Fatal(err)
	}
	if out, err := runPresetCLI(t, "new", "native hook"); err != nil {
		t.Fatalf("new: %s %v", out, err)
	}
	id, err := state.Active(root)
	if err != nil {
		t.Fatal(err)
	}
	invoke := func() (int, string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestHookCLIHelper$")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "MULIX_TEST_HOOK_PROCESS=1")
		cmd.Stdin = bytes.NewReader(hookPayload(t, root, "", "Write", "src/example.go"))
		out, err := cmd.CombinedOutput()
		if err == nil {
			return 0, string(out)
		}
		if exit, ok := err.(*exec.ExitError); ok {
			return exit.ExitCode(), string(out)
		}
		t.Fatal(err)
		return -1, ""
	}
	spec := filepath.Join(root, ".mulix", ".runtime", id, "spec.md")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), spec); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	code, out := invoke()
	if code != 2 || !strings.Contains(out, "unsafe runtime artifact") {
		t.Errorf("linked native spec: exit %d output %s", code, out)
	}
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	p := state.PathFor(root, id)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Replace(string(data), "runtime-issues-v1", "unknown-layout", 1)), 0644); err != nil {
		t.Fatal(err)
	}
	code, out = invoke()
	if code != 2 || !strings.Contains(out, "unknown artifact_layout") {
		t.Errorf("unknown native layout: exit %d output %s", code, out)
	}
	if out := evaluatePreToolUse(context.Background(), root, hookPayload(t, root, "", "Write", "src/example.go")); !errors.Is(out.Cause, state.ErrInvalidArtifacts) {
		t.Errorf("native artifact sentinel lost through context: %v", out.Cause)
	}
	malformed := strings.Replace(string(data), "artifact_layout: runtime-issues-v1", "artifact_layout: [runtime-issues-v1]", 1)
	if err := os.WriteFile(p, []byte(malformed), 0644); err != nil {
		t.Fatal(err)
	}
	code, out = invoke()
	if code != 2 || !strings.Contains(out, "invalid native artifact metadata") {
		t.Errorf("invalid layout shape: exit %d output %s", code, out)
	}
	for _, tc := range []struct{ name, content string }{
		{"malformed-native", string(data) + "tasks_dir: [\n"},
		{"malformed-layout-only", "artifact_layout: [\n"},
		{"malformed-tasks-only", "tasks_dir: [\n"},
		{"malformed-tracker-only", "tracker: [\n"},
		{"invalid-schema", strings.Replace(string(data), "schema: mulix.state.v2", "schema: invalid", 1)},
		{"invalid-phase", strings.Replace(string(data), "phase: specify", "phase: invalid", 1)},
	} {
		if tc.content == string(data) {
			t.Fatalf("%s fixture unchanged", tc.name)
		}
		if err := os.WriteFile(p, []byte(tc.content), 0644); err != nil {
			t.Fatal(err)
		}
		code, out = invoke()
		if code != 2 || !strings.Contains(out, "invalid native artifact metadata") {
			t.Errorf("%s: exit %d output %s", tc.name, code, out)
		}
	}
	quoted := string(data)
	for _, key := range []string{"artifact_layout", "tasks_dir", "tracker"} {
		quoted = strings.Replace(quoted, "\n"+key+":", "\n\""+key+"\":", 1)
	}
	quoted = "  " + strings.ReplaceAll(quoted, "\n", "\n  ") + "broken: [\n"
	if err := os.Mkdir(filepath.Join(filepath.Dir(p), "issues", ".probe"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(quoted), 0644); err != nil {
		t.Fatal(err)
	}
	code, out = invoke()
	if code != 2 || !strings.Contains(out, "invalid native artifact metadata") {
		t.Errorf("malformed indented quoted native state with issues/.probe directory: exit %d output %s", code, out)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	code, out = invoke()
	if code != 2 || !strings.Contains(out, "invalid native artifact metadata") {
		t.Errorf("missing native state: exit %d output %s", code, out)
	}

	legacy := flow.New("legacy-malformed", "")
	legacy.Phase = flow.PhaseBuild
	if err := state.Save(root, legacy); err != nil {
		t.Fatal(err)
	}
	if err := state.SetActive(root, legacy.Change); err != nil {
		t.Fatal(err)
	}
	p = state.PathFor(root, legacy.Change)
	issues := filepath.Join(filepath.Dir(p), "issues")
	if err := os.Mkdir(issues, 0755); err != nil {
		t.Fatal(err)
	}
	code, out = invoke()
	if code != 0 {
		t.Errorf("valid legacy state misclassified: exit %d output %s", code, out)
	}
	if err := os.Remove(issues); err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []string{"schema: [\n", "# artifact_layout: runtime-issues-v1\nschema: [\n", "nested:\n  tracker: x\nschema: [\n"} {
		if err := os.WriteFile(p, []byte(legacy), 0644); err != nil {
			t.Fatal(err)
		}
		code, out = invoke()
		if code != 0 {
			t.Errorf("malformed legacy state must fail open: exit %d output %s", code, out)
		}
	}
	if err := state.SetActive(root, "missing"); err != nil {
		t.Fatal(err)
	}
	code, out = invoke()
	if code != 0 {
		t.Errorf("legacy generic load failure no longer fail-open: exit %d %s", code, out)
	}
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
