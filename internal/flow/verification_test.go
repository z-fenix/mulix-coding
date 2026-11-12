package flow

import (
	"github.com/z-fenix/mulix-coding/internal/quality"
	"testing"
)

func TestApplyNewVerificationCycleInvalidatesOnlyFinalEvidence(t *testing.T) {
	for _, tc := range []struct {
		phase Phase
		event Event
	}{{PhaseBuild, EventBuildComplete}, {PhaseVerify, EventVerifyFail}, {PhaseArchive, EventVerifyFail}} {
		t.Run(string(tc.phase)+"/"+string(tc.event), func(t *testing.T) {
			s := New("x", "")
			s.Phase = tc.phase
			// Flow fixtures are not runner receipts; only the invalidation effect and
			// Apply's copy semantics are under test here.
			s.QualityRuns = []quality.RunRecord{{ID: "task-history", Task: 1, Stage: "green", Success: true}, {ID: "old-final", Stage: "verify", Success: true}}
			next, err := Apply(s, tc.event)
			if err != nil {
				t.Fatal(err)
			}
			if next.QualityRuns[1].Success || next.QualityRuns[1].Error == "" {
				t.Fatal("old final verification remained reusable")
			}
			if !next.QualityRuns[0].Success {
				t.Fatal("historical task provenance was invalidated")
			}
			if !s.QualityRuns[1].Success || s.QualityRuns[1].Error != "" {
				t.Fatal("Apply mutated its input's receipt slice")
			}
			if next.VerifyResult != VerifyPending {
				t.Fatal("new cycle did not reset result")
			}
		})
	}
}
