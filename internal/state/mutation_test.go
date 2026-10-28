package state

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/z-fenix/mulix-coding/internal/flow"
)

func TestUpdateChild(t *testing.T) {
	if os.Getenv("MULIX_UPDATE_CHILD") != "1" {
		return
	}
	op := os.Getenv("MULIX_UPDATE_OP")
	_, err := Update(context.Background(), os.Getenv("MULIX_UPDATE_ROOT"), "X", func(s *flow.State) error {
		fmt.Println("loaded")
		if op == "hold-spec" || op == "hold-transition" {
			_, _ = io.Copy(io.Discard, os.Stdin)
		}
		switch op {
		case "hold-spec":
			s.SpecPath = "spec"
		case "set-tasks":
			s.TasksPath = "tasks"
		case "hold-transition", "transition":
			next, err := flow.Apply(*s, "spec-complete")
			if err != nil {
				return err
			}
			*s = next
		}
		return nil
	}, nil)
	if err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
func updateChild(t *testing.T, root, op string) (*exec.Cmd, io.WriteCloser, *bufio.Reader) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestUpdateChild$")
	cmd.Env = append(os.Environ(), "MULIX_UPDATE_CHILD=1", "MULIX_UPDATE_ROOT="+root, "MULIX_UPDATE_OP="+op)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd, in, bufio.NewReader(out)
}
func updateReady(t *testing.T, r *bufio.Reader) {
	t.Helper()
	ready := make(chan string, 1)
	go func() { line, _ := r.ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if line != "loaded\n" {
			t.Fatalf("not loaded: %q", line)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("child timeout")
	}
}
func mutationFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, id := range []string{"X", "Y"} {
		if err := Save(root, flow.New(id, "")); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func TestUpdate_SerializesReadModifyWriteAcrossProcesses(t *testing.T) {
	root := mutationFixture(t)
	first, in, r := updateChild(t, root, "hold-spec")
	updateReady(t, r)
	second, in2, _ := updateChild(t, root, "set-tasks")
	_ = in2.Close()
	_ = in.Close()
	if err := first.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := second.Wait(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root, "X")
	if err != nil || got.SpecPath != "spec" || got.TasksPath != "tasks" {
		t.Fatalf("lost update: %+v %v", got, err)
	}
}
func TestUpdate_ReloadsPhaseBeforeTransition(t *testing.T) {
	root := mutationFixture(t)
	first, in, r := updateChild(t, root, "hold-transition")
	updateReady(t, r)
	second, in2, _ := updateChild(t, root, "transition")
	_ = in2.Close()
	_ = in.Close()
	if err := first.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := second.Wait(); err == nil {
		t.Fatal("reused stale phase")
	}
	got, _ := Load(root, "X")
	if got.Phase != flow.PhaseClarify {
		t.Fatalf("phase: %s", got.Phase)
	}
}
func TestUpdate_DifferentChangesDoNotShareLongGuardLock(t *testing.T) {
	root := mutationFixture(t)
	_, in, r := updateChild(t, root, "hold-spec")
	updateReady(t, r)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := Update(ctx, root, "Y", func(s *flow.State) error { s.TasksPath = "independent"; return nil }, nil)
	_ = in.Close()
	if err != nil {
		t.Fatal("independent change blocked:", err)
	}
}
func TestUpdate_AfterSaveFailureReportsSavedState(t *testing.T) {
	root := mutationFixture(t)
	failure := errors.New("cleanup failed")
	result, err := Update(context.Background(), root, "X", func(s *flow.State) error { s.Archived = true; return nil }, func(flow.State) error { return failure })
	if !errors.Is(err, failure) || !result.Saved || !result.State.Archived {
		t.Fatalf("partial success lost: %+v %v", result, err)
	}
	loaded, _ := Load(root, "X")
	if !loaded.Archived {
		t.Fatal("saved state rolled back")
	}
}
func TestUpdate_RejectsIdentityMutation(t *testing.T) {
	root := mutationFixture(t)
	if _, err := Update(context.Background(), root, "X", func(s *flow.State) error { s.Change = "Y"; return nil }, nil); err == nil {
		t.Fatal("changed locked identity")
	}
}
