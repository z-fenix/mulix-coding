package scaffold

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
)

var sessionTestBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mulix-script-binary-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	name := "mulix"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	sessionTestBin = filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", sessionTestBin, "./cmd/mulix")
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "script CLI build: %v\n%s", err, out)
		_ = os.RemoveAll(dir)
		os.Exit(2)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func scriptEnv(id string) []string {
	env := make([]string, 0, len(os.Environ())+2)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if strings.EqualFold(key, "PATH") || strings.EqualFold(key, "MULIX_SESSION_ID") {
			continue
		}
		env = append(env, item)
	}
	env = append(env, "PATH="+filepath.Dir(sessionTestBin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if id != "" {
		env = append(env, "MULIX_SESSION_ID="+id)
	}
	return env
}
func scriptCommand(ctx context.Context, root, id, script string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "bash", append([]string{filepath.ToSlash(script)}, args...)...)
	cmd.Dir = root
	cmd.Env = scriptEnv(id)
	return cmd
}
func sessionScriptFixture(t *testing.T) (string, string, *sessionctx.Store) {
	t.Helper()
	root, skills := ledgerFixture(t, HostClaude)
	for _, id := range []string{"X", "Y"} {
		s := flow.New(id, "")
		s.Phase = flow.PhaseBuild
		if err := state.Save(root, s); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(root, "docs", "changes", id, "tasks.md")
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# Plan\n\n## Task 1: First\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := sessionctx.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(context.Background(), sessionctx.Session); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct{ id, change string }{{"A", "X"}, {"B", "Y"}} {
		target, err := s.PrepareSelection(context.Background(), sessionctx.Identity{Value: v.id, Present: true}, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Select(context.Background(), target, v.change); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.SetActive(root, "Y"); err != nil {
		t.Fatal(err)
	}
	return root, skills, s
}
func runtimeFiles(t *testing.T, root string) []string {
	t.Helper()
	base := filepath.Join(root, ".mulix", ".runtime")
	var paths []string
	if err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err == nil {
			paths = append(paths, rel)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return paths
}
func TestSessionScripts_TwoBindingsProduceSeparateRuntimeDirs(t *testing.T) {
	root, skills, _ := sessionScriptFixture(t)
	for _, v := range []struct{ id, change string }{{"A", "X"}, {"B", "Y"}} {
		cmd := scriptCommand(context.Background(), root, v.id, filepath.Join(root, skills, "subagent-driven-development", "scripts", "sdd-workspace"), "docs/changes/"+v.change+"/tasks.md")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("workspace: %v %s", err, out)
		}
		if !strings.Contains(filepath.ToSlash(string(out)), "/.mulix/.runtime/"+v.change+"/sdd/tasks") {
			t.Fatalf("wrong workspace: %s", out)
		}
		if _, err := os.Stat(filepath.Join(root, ".mulix", ".runtime", v.change, "sdd", "tasks", "plan-path")); err != nil {
			t.Fatal(err)
		}
	}
}
func TestSessionScripts_BadBindingFailsWithoutCreatingSharedArtifacts(t *testing.T) {
	root, skills, _ := sessionScriptFixture(t)
	p := filepath.Join(root, ".mulix", "sessions", fmt.Sprintf("%x.json", sha256.Sum256([]byte("A"))))
	if err := os.WriteFile(p, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	before := runtimeFiles(t, root)
	cmd := scriptCommand(context.Background(), root, "A", filepath.Join(root, skills, "subagent-driven-development", "scripts", "sdd-workspace"), "docs/changes/X/tasks.md")
	out, err := cmd.CombinedOutput()
	after := runtimeFiles(t, root)
	if err == nil || !slices.Equal(before, after) {
		t.Fatalf("script hid error or wrote artifacts: %v %s", err, out)
	}
}

// The probe replaces only the external server process in an installed temporary
// skill. The real Bash script and mulix CLI still determine every path/env value.
const brainProbeJS = `const fs = require('fs');
const snapshot = {dir:process.env.BRAINSTORM_DIR,port:process.env.BRAINSTORM_PORT_FILE,token:process.env.BRAINSTORM_TOKEN_FILE};
console.log(JSON.stringify(snapshot));
const timer=setInterval(()=>{if(fs.existsSync(process.env.MULIX_PROBE_STOP)){clearInterval(timer);process.exit(0)}},20);
`

type brainSnapshot struct {
	Dir   string `json:"dir"`
	Port  string `json:"port"`
	Token string `json:"token"`
}

func installBrainProbe(t *testing.T, script string) {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node unavailable")
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(script), "server.cjs"), []byte(brainProbeJS), 0600); err != nil {
		t.Fatal(err)
	}
}
func brainProbe(t *testing.T, root, script, id string) brainSnapshot {
	t.Helper()
	installBrainProbe(t, script)
	stop := filepath.Join(t.TempDir(), "stop")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	cmd := scriptCommand(ctx, root, id, script, "--project-dir", filepath.ToSlash(root), "--foreground")
	cmd.Env = append(cmd.Env, "MULIX_PROBE_STOP="+stop)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(stop, []byte("stop"), 0600)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			cancel()
			<-done
		}
		cancel()
	})
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(out).ReadString('\n'); ready <- line }()
	var snapshot brainSnapshot
	select {
	case line := <-ready:
		if err := json.Unmarshal([]byte(line), &snapshot); err != nil {
			t.Fatalf("probe output: %q %v", line, err)
		}
	case <-ctx.Done():
		t.Fatal("probe startup timeout")
	}
	return snapshot
}
func TestSessionScripts_GlobalSwitchOnlyAffectsNewInvocation(t *testing.T) {
	root, skills, store := sessionScriptFixture(t)
	script := filepath.Join(root, skills, "brainstorming", "scripts", "start-server.sh")
	if err := state.SetActive(root, "X"); err != nil {
		t.Fatal(err)
	}
	firstScratch := scriptScratch(t, store, "")
	first := brainProbe(t, root, script, "")
	if err := state.SetActive(root, "Y"); err != nil {
		t.Fatal(err)
	}
	secondScratch := scriptScratch(t, store, "")
	second := brainProbe(t, root, script, "")
	for _, item := range []struct {
		got     brainSnapshot
		scratch string
	}{{first, firstScratch}, {second, secondScratch}} {
		assertBrainScratch(t, item.got, item.scratch)
	}
	if firstScratch == secondScratch {
		t.Fatal("global switch did not scope the next invocation")
	}
	boundScratch := scriptScratch(t, store, "A")
	bound := brainProbe(t, root, script, "A")
	assertBrainScratch(t, bound, boundScratch)
}
func TestSessionScripts_BrainstormBadBindingCreatesNoArtifacts(t *testing.T) {
	root, skills, store := sessionScriptFixture(t)
	scratch := scriptScratch(t, store, "A")
	script := filepath.Join(root, skills, "brainstorming", "scripts", "start-server.sh")
	installBrainProbe(t, script)
	p := filepath.Join(root, ".mulix", "sessions", fmt.Sprintf("%x.json", sha256.Sum256([]byte("A"))))
	if err := os.WriteFile(p, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	stop := filepath.Join(t.TempDir(), "stop")
	if err := os.WriteFile(stop, nil, 0600); err != nil {
		t.Fatal(err)
	}
	before := runtimeFiles(t, root)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := scriptCommand(ctx, root, "A", script, "--project-dir", filepath.ToSlash(root), "--foreground")
	cmd.Env = append(cmd.Env, "MULIX_PROBE_STOP="+stop)
	out, err := cmd.CombinedOutput()
	after := runtimeFiles(t, root)
	if err == nil || !slices.Equal(before, after) {
		t.Fatalf("brainstorm hid error or wrote artifacts: %v %s", err, out)
	}
	if _, err := os.Lstat(scratch); !os.IsNotExist(err) {
		t.Fatalf("bad binding created scratch output: %v", err)
	}
}
func TestSessionScripts_NestedRootAndSpaceInPath(t *testing.T) {
	root, _, _ := sessionScriptFixture(t)
	inner := filepath.Join(root, "nested project")
	if _, err := Init(InitOptions{Root: inner, Hosts: []Host{HostDSH}}); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(inner, flow.New("inner", "")); err != nil {
		t.Fatal(err)
	}
	if err := state.SetActive(inner, "inner"); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(inner, "tasks.md")
	if err := os.WriteFile(plan, []byte("# Plan"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := scriptCommand(context.Background(), inner, "", filepath.Join(inner, ".dsh", "skills", "subagent-driven-development", "scripts", "sdd-workspace"), "tasks.md")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(filepath.ToSlash(string(out)), "nested project/.mulix/.runtime/inner/sdd/tasks") {
		t.Fatalf("nested root: %v %s", err, out)
	}
}
func TestVendorPatches_ResolveContextInsteadOfReadingActive(t *testing.T) {
	source := os.Getenv("MULIX_SUPERPOWERS_SOURCE")
	if source == "" {
		t.Skip("set MULIX_SUPERPOWERS_SOURCE to matching upstream for regeneration proof")
	}
	repo := t.TempDir()
	scripts := filepath.Join(repo, "scripts")
	if err := os.MkdirAll(scripts, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "sync-superpowers.sh"))
	if err != nil {
		t.Fatal(err)
	}
	copyScript := filepath.Join(scripts, "sync-superpowers.sh")
	if err := os.WriteFile(copyScript, data, 0700); err != nil {
		t.Fatal(err)
	}
	// The sync uses its own overlay; copy that too into the disposable repository.
	if err := os.CopyFS(filepath.Join(scripts, "superpowers-overrides"), os.DirFS(filepath.Join("..", "..", "scripts", "superpowers-overrides"))); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", filepath.ToSlash(copyScript), filepath.ToSlash(source))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("regeneration: %v %s", err, out)
	}
	root, _, store := sessionScriptFixture(t)
	sdd := filepath.Join(repo, "assets", "superpowers", "skills", "subagent-driven-development", "scripts", "sdd-workspace")
	out, err = scriptCommand(context.Background(), root, "A", sdd, "docs/changes/X/tasks.md").CombinedOutput()
	if err != nil || !strings.Contains(filepath.ToSlash(string(out)), "/.mulix/.runtime/X/sdd/tasks") {
		t.Fatalf("regenerated workspace: %v %s", err, out)
	}
	brain := brainProbe(t, root, filepath.Join(repo, "assets", "superpowers", "skills", "brainstorming", "scripts", "start-server.sh"), "A")
	assertBrainScratch(t, brain, scriptScratch(t, store, "A"))
}
