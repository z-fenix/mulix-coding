package lockfile

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLockChild(t *testing.T) {
	if os.Getenv("MULIX_LOCK_CHILD") != "1" {
		return
	}
	err := With(context.Background(), os.Getenv("MULIX_LOCK_ROOT"), ContextKey, func() error {
		fmt.Println("locked")
		_, err := io.Copy(io.Discard, os.Stdin)
		return err
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	os.Exit(0)
}

func holdingChild(t *testing.T, root string) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLockChild$")
	cmd.Env = append(os.Environ(), "MULIX_LOCK_CHILD=1", "MULIX_LOCK_ROOT="+root)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(out).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if line != "locked\n" {
			t.Fatalf("child not ready: %q", line)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("child startup timeout")
	}
	return cmd, in
}

func TestLock_MutualExclusionAcrossProcesses(t *testing.T) {
	root := t.TempDir()
	_, in := holdingChild(t, root)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	calls := 0
	err := With(ctx, root, ContextKey, func() error { calls++; return nil })
	if !errors.Is(err, ErrTimeout) || calls != 0 {
		t.Fatalf("entered held lock: calls=%d err=%v", calls, err)
	}
	_ = in.Close()
	if err := With(context.Background(), root, ContextKey, func() error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatalf("released lock: %d %v", calls, err)
	}
}

func TestLock_TimeoutDoesNotRunCallback(t *testing.T) {
	root := t.TempDir()
	err := With(context.Background(), root, ContextKey, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		calls := 0
		err := With(ctx, root, ContextKey, func() error { calls++; return nil })
		if !errors.Is(err, ErrTimeout) || calls != 0 {
			t.Fatalf("timeout: %d %v", calls, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLock_KilledOwnerReleasesAndFileRemainsReusable(t *testing.T) {
	root := t.TempDir()
	cmd, _ := holdingChild(t, root)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if err := With(context.Background(), root, ContextKey, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".mulix", ".locks", "context.lock")); err != nil {
		t.Fatal("lock file must remain:", err)
	}
}
