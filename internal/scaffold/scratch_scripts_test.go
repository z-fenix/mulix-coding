package scaffold

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func scriptScratch(t *testing.T, store *sessionctx.Store, id string) string {
	t.Helper()
	r, err := store.Resolve(context.Background(), sessionctx.ResolveRequest{Identity: sessionctx.Identity{Value: id, Present: id != ""}})
	if err != nil || r.ScratchDir == "" {
		t.Fatalf("resolve script scratch: %+v %v", r, err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(r.ScratchDir) })
	return r.ScratchDir
}

func assertBrainScratch(t *testing.T, got brainSnapshot, scratch string) {
	t.Helper()
	prefix := filepath.ToSlash(filepath.Join(scratch, "brainstorm")) + "/"
	for _, p := range []string{got.Dir, got.Port, got.Token} {
		if !strings.HasPrefix(filepath.ToSlash(p), prefix) {
			t.Fatalf("companion escaped resolved scratch %s: %+v", scratch, got)
		}
	}
}

func TestSessionScripts_BrainstormUsesPrivateScratchOnly(t *testing.T) {
	temp := filepath.Join(t.TempDir(), "temporary files")
	if err := os.MkdirAll(temp, 0700); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, temp)
	}
	root, skills, store := sessionScriptFixture(t)
	scratch := scriptScratch(t, store, "A")
	before := runtimeFiles(t, root)
	script := filepath.Join(root, skills, "brainstorming", "scripts", "start-server.sh")
	first := brainProbe(t, root, script, "A")
	second := brainProbe(t, root, script, "A")
	assertBrainScratch(t, first, scratch)
	assertBrainScratch(t, second, scratch)
	if first.Dir == second.Dir {
		t.Fatal("service instances reused one session directory")
	}
	if !slices.Equal(before, runtimeFiles(t, root)) {
		t.Fatal("temporary companion created retained runtime files")
	}
	if runtime.GOOS != "windows" {
		for _, dir := range []string{scratch, filepath.Join(scratch, "brainstorm"), first.Dir, filepath.Join(first.Dir, "state")} {
			info, err := os.Stat(dir)
			if err != nil || info.Mode().Perm()&0077 != 0 {
				t.Fatalf("scratch is not owner-only: %s %v", dir, err)
			}
		}
	}
}

func TestSessionScripts_BrainstormRejectsSymlinkScratch(t *testing.T) {
	root, skills, store := sessionScriptFixture(t)
	scratch := scriptScratch(t, store, "A")
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "sentinel"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, scratch); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	script := filepath.Join(root, skills, "brainstorming", "scripts", "start-server.sh")
	installBrainProbe(t, script)
	stop := filepath.Join(t.TempDir(), "stop")
	if err := os.WriteFile(stop, nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := scriptCommand(ctx, root, "A", script, "--project-dir", filepath.ToSlash(root), "--foreground")
	cmd.Env = append(cmd.Env, "MULIX_PROBE_STOP="+stop)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("followed pre-existing scratch symlink: %s", out)
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 1 || entries[0].Name() != "sentinel" {
		t.Fatalf("wrote through scratch symlink: %v %v", entries, err)
	}
}

func TestSessionScripts_BrainstormWithoutChangeUsesScratch(t *testing.T) {
	root, skills, store := sessionScriptFixture(t)
	if err := state.ClearActive(root); err != nil {
		t.Fatal(err)
	}
	scratch := scriptScratch(t, store, "")
	before := runtimeFiles(t, root)
	got := brainProbe(t, root, filepath.Join(root, skills, "brainstorming", "scripts", "start-server.sh"), "")
	assertBrainScratch(t, got, scratch)
	if !slices.Equal(before, runtimeFiles(t, root)) {
		t.Fatal("no-change companion created retained runtime files")
	}
}

func TestSessionScripts_BrainstormRejectsRelativeTempRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix TMPDIR semantics")
	}
	for _, project := range []bool{true, false} {
		name := "standalone"
		if project {
			name = "project"
		}
		t.Run(name, func(t *testing.T) {
			root, skills, _ := sessionScriptFixture(t)
			script := filepath.Join(root, skills, "brainstorming", "scripts", "start-server.sh")
			installBrainProbe(t, script)
			temp := filepath.Join(root, "relative temporary files")
			if err := os.Mkdir(temp, 0700); err != nil {
				t.Fatal(err)
			}
			stop := filepath.Join(t.TempDir(), "stop")
			if err := os.WriteFile(stop, nil, 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"--foreground"}
			if project {
				args = append(args, "--project-dir", filepath.ToSlash(root))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := scriptCommand(ctx, root, "A", script, args...)
			cmd.Env = append(cmd.Env, "TMPDIR=relative temporary files", "MULIX_PROBE_STOP="+stop)
			out, err := cmd.CombinedOutput()
			if err == nil || !strings.Contains(string(out), "absolute") {
				t.Errorf("relative temp root did not stop startup: %v %s", err, out)
			}
			entries, err := os.ReadDir(temp)
			if err != nil || len(entries) != 0 {
				t.Errorf("invalid temp root created companion artifacts: %v %v", entries, err)
			}
		})
	}
}
