package cliutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/lockfile"
	"github.com/z-fenix/mulix-coding/internal/quality"
	"github.com/z-fenix/mulix-coding/internal/state"
	"gopkg.in/yaml.v3"
)

// This subprocess is an event-producing test adapter. Coordination files live
// outside the project so the synchronization itself is not a source mutation.
func TestQualityStageChild(t *testing.T) {
	sync := os.Getenv("MULIX_QUALITY_SYNC")
	if sync == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(sync, "ready"), []byte("ready"), 0600); err != nil {
		os.Exit(2)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(sync, "release")); err == nil {
			fmt.Println(`{"name":"fixture#TestValue","status":"pass"}`)
			os.Exit(0)
		}
		time.Sleep(time.Millisecond * 5)
	}
	os.Exit(2)
}

func waitQualityReady(t *testing.T, sync string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(sync, "ready")); err == nil {
			return
		}
		time.Sleep(time.Millisecond * 5)
	}
	t.Fatal("quality subprocess did not start")
}

func TestQualityCLIInterruptedOrChangedExecutionNeverLeavesPass(t *testing.T) {
	for _, scenario := range []string{"cancel", "phase", "source", "report-write", "superseded"} {
		t.Run(scenario, func(t *testing.T) {
			root := chdirTemp(t)
			initMulixRoot(t, root)
			s := setupQualityCLI(t, root, "quality")
			sync := t.TempDir()
			t.Setenv("MULIX_QUALITY_SYNC", sync)
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			policy := quality.Policy{Schema: "mulix.quality.v1", Suites: []quality.Suite{{ID: "unit", Command: []string{exe, "-test.run=^TestQualityStageChild$"}, CWD: ".", Format: "json", TimeoutSeconds: 10}}}
			data, err := yaml.Marshal(policy)
			if err != nil {
				t.Fatal(err)
			}
			qualityCLIFile(t, root, quality.PolicyPath, string(data))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := NewRootCmd()
			cmd.SetArgs([]string{"test", "run", "--task", "1", "--stage", "green"})
			done := make(chan error, 1)
			go func() { done <- cmd.ExecuteContext(ctx) }()
			waitQualityReady(t, sync)
			loaded, err := state.Load(root, s.Change)
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded.QualityRuns) != 1 || loaded.QualityRuns[0].Success {
				t.Fatal("attempt not invalidated before process start")
			}
			switch scenario {
			case "cancel":
				cancel()
			case "phase":
				_, err = state.Update(context.Background(), root, s.Change, func(current *flow.State) error { current.Phase = flow.PhaseVerify; return nil }, nil)
			case "source":
				qualityCLIFile(t, root, "value.go", "package fixture\n// changed during execution\n")
			case "report-write":
				err = os.Mkdir(filepath.Join(root, ".mulix/.runtime/quality/sdd/tasks/task-1-report.md"), 0700)
			case "superseded":
				_, err = state.Update(context.Background(), root, s.Change, func(current *flow.State) error { current.QualityRuns[0].ID = "newer-attempt"; return nil }, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(sync, "release"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("invalidated execution succeeded")
				}
			case <-time.After(8 * time.Second):
				t.Fatal("CLI failed to persist/return promptly")
			}
			loaded, err = state.Load(root, s.Change)
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded.QualityRuns) != 1 || loaded.QualityRuns[0].Success {
				t.Fatal("interrupted attempt left a successful receipt")
			}
			if scenario == "cancel" && loaded.QualityRuns[0].Finished.IsZero() {
				t.Fatal("cancellation prevented failure persistence")
			}
			if scenario == "superseded" && loaded.QualityRuns[0].ID != "newer-attempt" {
				t.Fatal("old execution overwrote newer attempt")
			}
		})
	}
}

// TestQualityReceiptRetryChild asks for the same state mutation a retry uses
// before running its suites. The parent is waiting for this subprocess inside
// fingerprinting, so a held read lock cannot be released until the probe exits.
func TestQualityReceiptRetryChild(t *testing.T) {
	root := os.Getenv("MULIX_RECEIPT_FIXTURE_ROOT")
	if root == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := state.Update(ctx, root, "issue", func(s *flow.State) error {
		replaceRun(s, quality.RunRecord{ID: "pending-retry", Task: 1, Stage: "check", Error: "execution started but has not completed"})
		return nil
	}, nil)
	result := "retry started"
	if errors.Is(err, lockfile.ErrTimeout) {
		result = "retry blocked"
	} else if err != nil {
		result = err.Error()
	}
	if err := os.WriteFile(os.Getenv("MULIX_RECEIPT_PROBE_RESULT"), []byte(result), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestIssueTaskReceiptChecksUnderChangeLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Git probe uses a POSIX executable wrapper")
	}
	root := chdirTemp(t)
	s := setupSingleIssueCheck(t, root)
	qualityCommand(t, "test", "run", "--task", "1", "--stage", "check")
	run := latestIssueCheck(t, root, s)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	probes := t.TempDir()
	result := filepath.Join(probes, "result")
	git := filepath.Join(probes, "git")
	if err := os.WriteFile(git, []byte(`#!/bin/sh
[ "$3" = rev-parse ] && [ "$4" = --is-inside-work-tree ] || exit 1
"$MULIX_RECEIPT_TEST_BINARY" -test.run='^TestQualityReceiptRetryChild$' > /dev/null || exit 2
printf 'false\n'
`), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MULIX_RECEIPT_FIXTURE_ROOT", root)
	t.Setenv("MULIX_RECEIPT_TEST_BINARY", exe)
	t.Setenv("MULIX_RECEIPT_PROBE_RESULT", result)
	t.Setenv("PATH", probes+string(os.PathListSeparator)+os.Getenv("PATH"))
	out := qualityCommand(t, "test", "receipt", "--task", "1")
	status, err := os.ReadFile(result)
	if err != nil || string(status) != "retry blocked" {
		t.Fatalf("receipt returned with status %q (%v): %s", status, err, out)
	}
	if out != "Task 1: current check receipt "+run.ID+"\n" {
		t.Fatalf("unexpected receipt output: %q", out)
	}
}
