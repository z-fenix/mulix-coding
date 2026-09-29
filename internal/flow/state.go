package flow

import (
	"slices"
	"time"
)

// VerifyResult tracks the outcome of the most recent verify phase run.
type VerifyResult string

const (
	VerifyPending VerifyResult = "pending"
	VerifyPass    VerifyResult = "pass"
	VerifyFail    VerifyResult = "fail"
)

// ArchiveConfirmation tracks whether the human has explicitly confirmed
// archival. It is never inferred from silence.
type ArchiveConfirmation string

const (
	ArchiveUnset     ArchiveConfirmation = ""
	ArchivePending   ArchiveConfirmation = "pending"
	ArchiveConfirmed ArchiveConfirmation = "confirmed"
)

// DesignTrack is the brainstorming path the design phase classified the
// change into: how much design process it needed before approval. There
// is no spike track: by the design phase the change has a clarified spec,
// so its output is code that ships, not a throwaway answer.
type DesignTrack string

const (
	TrackBounded       DesignTrack = "bounded"
	TrackArchitectural DesignTrack = "architectural"
)

// DesignTracks lists every valid track.
var DesignTracks = []DesignTrack{TrackBounded, TrackArchitectural}

// Valid reports whether t is a known track.
func (t DesignTrack) Valid() bool {
	return slices.Contains(DesignTracks, t)
}

// ExecutionMethod is how the build phase executes tasks.md, chosen by the
// human at the start of the build phase.
type ExecutionMethod string

const (
	// ExecSubagentDriven: a fresh implementer subagent per plan task, a
	// task review after each, a whole-branch review at the end.
	ExecSubagentDriven ExecutionMethod = "subagent-driven"
	// ExecInline: every plan task implemented in the session itself, one
	// whole-branch review at the end.
	ExecInline ExecutionMethod = "inline"
)

// ExecutionMethods lists every valid execution method.
var ExecutionMethods = []ExecutionMethod{ExecSubagentDriven, ExecInline}

// Valid reports whether m is a known execution method.
func (m ExecutionMethod) Valid() bool {
	return slices.Contains(ExecutionMethods, m)
}

// State is the full persisted record for one change/feature moving through
// the mulix workflow. One State lives at
// .mulix/.runtime/<change-id>/state.yaml, next to the brainstorming and
// execution artifacts the design and build phases produce for that
// change. Unknown fields on disk are treated as errors by the
// state package's loader (strict decoding): the state file is data the
// tooling trusts, not free-form notes.
type State struct {
	Schema string `yaml:"schema"`
	Change string `yaml:"change"`
	Branch string `yaml:"branch,omitempty"`

	Phase Phase `yaml:"phase"`

	SpecPath   string `yaml:"spec_path,omitempty"`
	TasksPath  string `yaml:"tasks_path,omitempty"`
	ReportPath string `yaml:"report_path,omitempty"`

	ClarifySkipped bool `yaml:"clarify_skipped,omitempty"`

	// DesignTrack, DesignPath, and DesignApproved record the design
	// phase's brainstorming outcome: the classified path, the approved
	// design document under .mulix/.runtime/<change>/specs/ (architectural
	// track only — a bounded design is approved in chat), and whether the
	// human explicitly approved it.
	DesignTrack    DesignTrack `yaml:"design_track,omitempty"`
	DesignPath     string      `yaml:"design_path,omitempty"`
	DesignApproved bool        `yaml:"design_approved,omitempty"`

	// ExecutionMethod is the human's choice, at the start of the build
	// phase, of how tasks.md's "## Task N" sections get executed.
	ExecutionMethod ExecutionMethod `yaml:"execution_method,omitempty"`

	VerifyResult   VerifyResult `yaml:"verify_result,omitempty"`
	VerifyFailures int          `yaml:"verify_failures"`

	ArchiveConfirmation ArchiveConfirmation `yaml:"archive_confirmation,omitempty"`
	Archived            bool                `yaml:"archived"`

	CreatedAt time.Time `yaml:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at"`
}

// New returns a fresh State for a newly created change, starting in the
// first phase of the workflow.
func New(change, branch string) State {
	now := time.Now().UTC()
	return State{
		Schema:       SchemaVersion,
		Change:       change,
		Branch:       branch,
		Phase:        PhaseSpecify,
		VerifyResult: VerifyPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// SchemaVersion is written into every state file and checked on load so a
// future incompatible layout fails loudly instead of silently.
const SchemaVersion = "mulix.state.v2"
