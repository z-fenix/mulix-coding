package cliutil

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func TestSessionCLIChild(t *testing.T) {
	if os.Getenv("MULIX_CLI_CHILD") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}
	cmd := NewRootCmd()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}
func sessionCLIProcess(root string, args ...string) *exec.Cmd {
	argv := append([]string{"-test.run=^TestSessionCLIChild$", "--"}, args...)
	cmd := exec.Command(os.Args[0], argv...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "MULIX_CLI_CHILD=1")
	return cmd
}
func TestSessionCLI_ConcurrentFieldUpdatesPreserveBoth(t *testing.T) {
	root, s := sessionProject(t)
	selectForSession(t, s, "A", "X")
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, kv := range [][2]string{{"spec_path", "spec-X"}, {"tasks_path", "tasks-X"}} {
		go func(kv [2]string) {
			<-start
			out, err := sessionCLIProcess(root, "state", "set", kv[0], kv[1], "--session-id", "A").CombinedOutput()
			if err != nil {
				err = fmt.Errorf("%w: %s", err, out)
			}
			done <- err
		}(kv)
	}
	close(start)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := state.Load(root, "X")
	if err != nil || loaded.SpecPath != "spec-X" || loaded.TasksPath != "tasks-X" {
		t.Fatalf("CLI lost a field: %+v %v", loaded, err)
	}
}
func TestSessionCLI_ArchiveRaceDoesNotClearNewActive(t *testing.T) {
	root, s := sessionProject(t)
	x, _ := state.Load(root, "X")
	x.Phase = flow.PhaseArchive
	x.ArchiveConfirmation = flow.ArchiveConfirmed
	saveStateForTest(t, root, x)
	selectForSession(t, s, "A", "X")
	// This is the dangerous interleaving: another selection has completed
	// before the archiver's cleanup, but still after A originally selected X.
	if err := state.SetActive(root, "Y"); err != nil {
		t.Fatal(err)
	}
	out, err := sessionCLIProcess(root, "state", "transition", "archived", "--session-id", "A").CombinedOutput()
	if err != nil {
		t.Fatalf("archive subprocess: %v %s", err, out)
	}
	active, err := state.Active(root)
	if err != nil || active != "Y" {
		t.Fatalf("archiver cleared another selection: %q %v", active, err)
	}
	if _, err := s.Resolve(context.Background(), sessionctx.ResolveRequest{Identity: sessionctx.Identity{Value: "A", Present: true}}); err == nil {
		t.Fatal("archived binding fell back")
	}
}
func TestSessionCLI_FailedBindingWriteReportsCreatedChange(t *testing.T) {
	root, _ := sessionProject(t)
	p := filepath.Join(root, ".mulix", "sessions", fmt.Sprintf("%x.json", sha256.Sum256([]byte("A"))))
	if err := os.MkdirAll(p, 0700); err != nil {
		t.Fatal(err)
	}
	out, err := sessionCLIProcess(root, "new", "CLI Partial", "--session-id", "A").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "created change") {
		t.Fatalf("lost created change evidence: %v %s", err, out)
	}
	ids, err := state.List(root)
	if err != nil || len(ids) != 3 {
		t.Fatalf("partial new destroyed state: %v %v", ids, err)
	}
}
