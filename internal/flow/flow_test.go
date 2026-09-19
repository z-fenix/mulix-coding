package flow

import "testing"

func TestApply_LegalTransitionAdvancesPhase(t *testing.T) {
	s := New("add-login", "feat/add-login")
	s.SpecPath = "specs/add-login/spec.md"

	next, err := Apply(s, EventSpecComplete)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.Phase != PhaseClarify {
		t.Fatalf("expected phase %q, got %q", PhaseClarify, next.Phase)
	}
	// Apply must not mutate the original.
	if s.Phase != PhaseSpecify {
		t.Fatalf("Apply mutated the original state's phase: %q", s.Phase)
	}
}

func TestApply_IllegalEventFromPhaseErrors(t *testing.T) {
	s := New("add-login", "feat/add-login")

	// Can't jump straight to verify-pass from specify.
	_, err := Apply(s, EventVerifyPass)
	if err == nil {
		t.Fatal("expected error for illegal transition, got nil")
	}
}

func TestApply_VerifyFailReturnsToBuildAndIncrementsCounter(t *testing.T) {
	s := New("add-login", "feat/add-login")
	s.Phase = PhaseVerify
	s.VerifyFailures = 2

	next, err := Apply(s, EventVerifyFail)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.Phase != PhaseBuild {
		t.Fatalf("expected phase %q, got %q", PhaseBuild, next.Phase)
	}
	if next.VerifyFailures != 3 {
		t.Fatalf("expected verify_failures=3, got %d", next.VerifyFailures)
	}
	if next.VerifyResult != VerifyPending {
		t.Fatalf("expected verify_result reset to pending, got %q", next.VerifyResult)
	}
}

func TestApply_VerifyPassSetsArchivePending(t *testing.T) {
	s := New("add-login", "feat/add-login")
	s.Phase = PhaseVerify

	next, err := Apply(s, EventVerifyPass)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.Phase != PhaseArchive {
		t.Fatalf("expected phase %q, got %q", PhaseArchive, next.Phase)
	}
	if next.ArchiveConfirmation != ArchivePending {
		t.Fatalf("expected archive_confirmation=pending, got %q", next.ArchiveConfirmation)
	}
}

func TestApply_BuildCompleteResetsVerifyResultToPending(t *testing.T) {
	s := New("add-login", "feat/add-login")
	s.Phase = PhaseBuild
	s.VerifyResult = VerifyFail

	next, err := Apply(s, EventBuildComplete)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if next.Phase != PhaseVerify {
		t.Fatalf("expected phase %q, got %q", PhaseVerify, next.Phase)
	}
	if next.VerifyResult != VerifyPending {
		t.Fatalf("expected verify_result reset to pending, got %q", next.VerifyResult)
	}
}

func TestFind_UnknownEventFromPhase(t *testing.T) {
	if _, ok := Find(PhaseArchive, EventSpecComplete); ok {
		t.Fatal("expected no transition, got one")
	}
}

func TestNextEvents_ListsAllLegalEventsFromPhase(t *testing.T) {
	events := NextEvents(PhaseClarify)
	want := map[Event]bool{EventClarifyComplete: true, EventClarifySkipped: true}
	if len(events) != len(want) {
		t.Fatalf("expected %d events, got %v", len(want), events)
	}
	for _, e := range events {
		if !want[e] {
			t.Fatalf("unexpected event %q in NextEvents(clarify)", e)
		}
	}
}

func TestNextEvents_TerminalPhaseHasNoOutboundEvents(t *testing.T) {
	// Archive only has a self-loop (archived), never leaves the phase via
	// a *different* From/To pair other than itself.
	events := NextEvents(PhaseArchive)
	if len(events) != 1 || events[0] != EventArchived {
		t.Fatalf("expected only [archived], got %v", events)
	}
}

func TestPhaseValid(t *testing.T) {
	if !PhaseBuild.Valid() {
		t.Fatal("expected PhaseBuild to be valid")
	}
	if Phase("bogus").Valid() {
		t.Fatal("expected bogus phase to be invalid")
	}
}
