// Package flow implements the mulix phase state machine: a small, strict
// finite-state machine that governs a single change's progress through the
// spec-driven workflow. Phases and transitions are data, not convention, so
// both the guard CLI and the Claude Code PreToolUse hook can make the same
// allow/block decision from the same source of truth.
package flow

import "slices"

import "fmt"

// Phase is one step of the mulix workflow.
type Phase string

const (
	PhaseSpecify Phase = "specify"
	PhaseClarify Phase = "clarify"
	PhasePlan    Phase = "plan"
	PhaseTasks   Phase = "tasks"
	PhaseAnalyze Phase = "analyze"
	PhaseBuild   Phase = "build"
	PhaseVerify  Phase = "verify"
	PhaseArchive Phase = "archive"
)

// Phases lists every phase in workflow order. Order matters: it's used for
// validation and for rendering progress.
var Phases = []Phase{
	PhaseSpecify,
	PhaseClarify,
	PhasePlan,
	PhaseTasks,
	PhaseAnalyze,
	PhaseBuild,
	PhaseVerify,
	PhaseArchive,
}

// Valid reports whether p is a known phase.
func (p Phase) Valid() bool {
	return slices.Contains(Phases, p)
}

// Event is a named transition trigger. Events are the only way state may
// move between phases; there is no implicit "advance" operation.
type Event string

const (
	EventSpecComplete    Event = "spec-complete"
	EventClarifyComplete Event = "clarify-complete"
	EventClarifySkipped  Event = "clarify-skipped"
	EventPlanComplete    Event = "plan-complete"
	EventTasksComplete   Event = "tasks-complete"
	EventAnalyzeComplete Event = "analyze-complete"
	EventAnalyzeSkipped  Event = "analyze-skipped"
	EventBuildComplete   Event = "build-complete"
	EventVerifyPass      Event = "verify-pass"
	EventVerifyFail      Event = "verify-fail"
	EventArchived        Event = "archived"
)

// Transition describes one legal (from-phase, event) -> to-phase move, plus
// the guard references that must all pass before the transition may be
// applied with --apply. Guards themselves live in the guard package; this
// table only names them so guard and flow stay decoupled.
type Transition struct {
	Event     Event
	From      Phase
	To        Phase
	GuardRefs []string
	// Effect mutates state fields beyond the phase change itself (e.g.
	// bumping a failure counter). Nil means no extra effect.
	Effect func(s *State)
}

// Table is the full set of legal transitions.
var Table = []Transition{
	{
		Event:     EventSpecComplete,
		From:      PhaseSpecify,
		To:        PhaseClarify,
		GuardRefs: []string{"spec-artifact-present"},
	},
	{
		Event:     EventClarifyComplete,
		From:      PhaseClarify,
		To:        PhasePlan,
		GuardRefs: []string{"clarify-recorded"},
	},
	{
		Event:     EventClarifySkipped,
		From:      PhaseClarify,
		To:        PhasePlan,
		GuardRefs: []string{"clarify-skip-acknowledged"},
	},
	{
		Event:     EventPlanComplete,
		From:      PhasePlan,
		To:        PhaseTasks,
		GuardRefs: []string{"plan-artifacts-present"},
	},
	{
		Event:     EventTasksComplete,
		From:      PhaseTasks,
		To:        PhaseAnalyze,
		GuardRefs: []string{"tasks-artifact-present"},
	},
	{
		Event:     EventAnalyzeComplete,
		From:      PhaseAnalyze,
		To:        PhaseBuild,
		GuardRefs: []string{"analyze-report-present"},
	},
	{
		Event:     EventAnalyzeSkipped,
		From:      PhaseAnalyze,
		To:        PhaseBuild,
		GuardRefs: []string{"analyze-skip-acknowledged"},
	},
	{
		Event:     EventBuildComplete,
		From:      PhaseBuild,
		To:        PhaseVerify,
		GuardRefs: []string{"tasks-all-checked", "tdd-evidence-present"},
		Effect: func(s *State) {
			s.VerifyResult = VerifyPending
		},
	},
	{
		Event:     EventVerifyPass,
		From:      PhaseVerify,
		To:        PhaseArchive,
		GuardRefs: []string{"verification-report-present", "verify-result-pass"},
		Effect: func(s *State) {
			s.ArchiveConfirmation = ArchivePending
		},
	},
	{
		Event:     EventVerifyFail,
		From:      PhaseVerify,
		To:        PhaseBuild,
		GuardRefs: []string{"verification-failed"},
		Effect: func(s *State) {
			s.VerifyFailures++
			s.VerifyResult = VerifyPending
		},
	},
	{
		Event:     EventArchived,
		From:      PhaseArchive,
		To:        PhaseArchive,
		GuardRefs: []string{"archive-confirmed"},
		Effect: func(s *State) {
			s.Archived = true
		},
	},
}

// Find returns the transition matching the current phase and event, or
// false if no such transition exists.
func Find(from Phase, event Event) (Transition, bool) {
	for _, t := range Table {
		if t.From == from && t.Event == event {
			return t, true
		}
	}
	return Transition{}, false
}

// NextEvents returns every event that is legal from phase, in table order.
// CLI commands use this to tell the user (or agent) what they can do next
// without hardcoding phase-specific logic outside this package.
func NextEvents(phase Phase) []Event {
	var events []Event
	for _, t := range Table {
		if t.From == phase {
			events = append(events, t.Event)
		}
	}
	return events
}

// Apply is a pure function: given a state and an event, it returns the new
// state (a copy) or an error if the event is not legal from the current
// phase. It does not check guards — callers (the guard package, or the
// hook) are responsible for gating calls to Apply on guard results. This
// mirrors comet's applyClassicTransition: the transition table enforces
// *structure* (what moves are legal at all), guards enforce *readiness*
// (whether the artifacts/evidence justify the move right now).
func Apply(s State, event Event) (State, error) {
	t, ok := Find(s.Phase, event)
	if !ok {
		return State{}, fmt.Errorf("flow: no transition for event %q from phase %q", event, s.Phase)
	}
	next := s
	next.Phase = t.To
	if t.Effect != nil {
		t.Effect(&next)
	}
	return next, nil
}
