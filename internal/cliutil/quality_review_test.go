package cliutil

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/quality"
	"github.com/z-fenix/mulix-coding/internal/state"
)

// Cancel a real context deterministically at the runner's post-suite check,
// rather than relying on a timer to hit the small final-suite/persistence gap.
type cancelAfterSuiteContext struct {
	context.Context
	cancel context.CancelFunc
}

func (c *cancelAfterSuiteContext) Err() error {
	pc, _, _, _ := runtime.Caller(1)
	if fn := runtime.FuncForPC(pc); fn != nil && strings.HasSuffix(fn.Name(), "/quality.Run") {
		c.cancel()
	}
	return c.Context.Err()
}
func prepareQualityVerification(t *testing.T, root string) {
	t.Helper()
	initMulixRoot(t, root)
	setupQualityCLI(t, root, "quality")
	realTaskEvidence(t, root)
	qualityCommand(t, "state", "transition", "build-complete")
	qualityCLIFile(t, root, "docs/changes/quality/report.md", "```mulix-review\nreviewer: independent test reviewer\nresult: approved\nblocking: []\n```\n")
}
func TestQualityCLICancelledAfterFinalSuiteCannotPersistPass(t *testing.T) {
	root := chdirTemp(t)
	prepareQualityVerification(t, root)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelAfterSuiteContext{Context: parent, cancel: cancel}
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"verify"})
	err := cmd.ExecuteContext(ctx)
	if parent.Err() == nil {
		t.Fatal("cancellation boundary was not reached")
	}
	if err == nil {
		t.Fatal("cancelled final-suite execution returned success")
	}
	loaded, err := state.Load(root, "quality")
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := quality.Latest(loaded.QualityRuns, 0, "verify")
	if !ok || receipt.Success || receipt.Error == "" || loaded.VerifyResult == flow.VerifyPass {
		t.Fatalf("cancelled execution persisted pass: %+v", loaded)
	}
	if _, err := runPresetCLI(t, "state", "transition", "verify-pass"); err == nil {
		t.Fatal("cancelled receipt accepted by normal gate")
	}
}
func TestQualityCLIVerificationCannotSurviveCorrectivePhaseCycle(t *testing.T) {
	for _, start := range []string{"verify", "archive"} {
		t.Run(start, func(t *testing.T) {
			root := chdirTemp(t)
			prepareQualityVerification(t, root)
			qualityCommand(t, "verify")
			before, err := state.Load(root, "quality")
			if err != nil {
				t.Fatal(err)
			}
			previous, _ := quality.Latest(before.QualityRuns, 0, "verify")
			if start == "archive" {
				qualityCommand(t, "state", "transition", "verify-pass")
			}
			qualityCommand(t, "state", "set", "verify_result", "fail")
			qualityCommand(t, "state", "transition", "verify-fail")
			qualityCommand(t, "state", "transition", "build-complete")
			qualityCommand(t, "state", "set", "verify_result", "pass")
			if _, err := runPresetCLI(t, "state", "transition", "verify-pass"); err == nil {
				t.Fatal("old final verification accepted after corrective phase cycle")
			}
			qualityCommand(t, "verify")
			after, err := state.Load(root, "quality")
			if err != nil {
				t.Fatal(err)
			}
			current, _ := quality.Latest(after.QualityRuns, 0, "verify")
			if current.ID == previous.ID {
				t.Fatal("fresh verification did not replace old receipt")
			}
			qualityCommand(t, "state", "transition", "verify-pass")
			qualityCommand(t, "state", "set", "archive_confirmation", "confirmed")
			qualityCommand(t, "state", "transition", "archived")
		})
	}
}
