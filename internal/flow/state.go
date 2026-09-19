package flow

import "time"

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

// State is the full persisted record for one change/feature moving through
// the mulix workflow. One State lives at
// docs/changes/<change-id>/.runtime/state.yaml. Unknown fields on disk are
// treated as errors by the state package's loader (strict decoding): the
// state file is data the tooling trusts, not free-form notes.
type State struct {
	Schema string `yaml:"schema"`
	Change string `yaml:"change"`
	Branch string `yaml:"branch,omitempty"`

	Phase Phase `yaml:"phase"`

	SpecPath    string `yaml:"spec_path,omitempty"`
	PlanPath    string `yaml:"plan_path,omitempty"`
	TasksPath   string `yaml:"tasks_path,omitempty"`
	AnalyzePath string `yaml:"analyze_path,omitempty"`
	ReportPath  string `yaml:"report_path,omitempty"`

	ClarifySkipped bool `yaml:"clarify_skipped,omitempty"`
	AnalyzeSkipped bool `yaml:"analyze_skipped,omitempty"`

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
const SchemaVersion = "mulix.state.v1"
