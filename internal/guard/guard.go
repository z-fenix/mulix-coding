// Package guard implements the phase-exit readiness checks that gate
// flow.Apply calls. Each check inspects the filesystem/state for concrete
// evidence (an artifact exists and is non-empty, tasks.md is fully checked
// off, a verification report was written) rather than trusting the agent's
// say-so. Guards never mutate state; Run only reports pass/fail plus a
// remediation hint.
package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mulix-dev/mulix-coding/internal/flow"
)

// Result is the outcome of one named check.
type Result struct {
	Name string
	Pass bool
	// Next is a short remediation hint shown when Pass is false.
	Next string
}

// Report is the full set of check results for one phase-exit attempt.
type Report struct {
	Event   flow.Event
	Results []Result
}

// Passed reports whether every check in the report passed.
func (r Report) Passed() bool {
	for _, res := range r.Results {
		if !res.Pass {
			return false
		}
	}
	return true
}

// Failures returns only the failed results, in order.
func (r Report) Failures() []Result {
	var out []Result
	for _, res := range r.Results {
		if !res.Pass {
			out = append(out, res)
		}
	}
	return out
}

// checkFunc runs one guard check against project root + state.
type checkFunc func(root string, s flow.State) Result

// checksByEvent mirrors flow.Table: for each event that exits a phase, the
// ordered list of checks that must all pass before flow.Apply may be
// called with that event.
var checksByEvent = map[flow.Event][]checkFunc{
	flow.EventSpecComplete:    {checkArtifactPresent("spec-artifact-present", func(s flow.State) string { return s.SpecPath })},
	flow.EventClarifyComplete: {checkClarifyRecorded},
	flow.EventClarifySkipped:  {checkClarifySkipAcknowledged},
	flow.EventPlanComplete:    {checkArtifactPresent("plan-artifacts-present", func(s flow.State) string { return s.PlanPath })},
	flow.EventTasksComplete:   {checkArtifactPresent("tasks-artifact-present", func(s flow.State) string { return s.TasksPath })},
	flow.EventAnalyzeComplete: {checkArtifactPresent("analyze-report-present", func(s flow.State) string { return s.AnalyzePath })},
	flow.EventAnalyzeSkipped:  {checkAnalyzeSkipAcknowledged},
	flow.EventBuildComplete:   {checkTasksAllChecked},
	flow.EventVerifyPass:      {checkArtifactPresent("verification-report-present", func(s flow.State) string { return s.ReportPath }), checkVerifyResultPass},
	flow.EventVerifyFail:      {checkVerifyResultFail},
	flow.EventArchived:        {checkArchiveConfirmed},
}

// Run executes every check registered for event against root/s and returns
// the full report. An event with no registered checks passes trivially
// (structural transitions with no readiness evidence to check, if any are
// ever added).
func Run(root string, s flow.State, event flow.Event) Report {
	checks := checksByEvent[event]
	report := Report{Event: event}
	for _, check := range checks {
		report.Results = append(report.Results, check(root, s))
	}
	return report
}

// --- individual checks ---

// checkArtifactPresent builds a check that verifies a state-recorded path
// exists on disk and is non-empty.
func checkArtifactPresent(name string, getPath func(flow.State) string) checkFunc {
	return func(root string, s flow.State) Result {
		p := getPath(s)
		if p == "" {
			return Result{Name: name, Pass: false, Next: fmt.Sprintf("%s: no path recorded in state yet.", name)}
		}
		full := p
		if !filepath.IsAbs(full) {
			full = filepath.Join(root, p)
		}
		info, err := os.Stat(full)
		if err != nil {
			return Result{Name: name, Pass: false, Next: fmt.Sprintf("%s: %s does not exist yet.", name, p)}
		}
		if info.Size() == 0 {
			return Result{Name: name, Pass: false, Next: fmt.Sprintf("%s: %s exists but is empty.", name, p)}
		}
		return Result{Name: name, Pass: true}
	}
}

func checkClarifyRecorded(root string, s flow.State) Result {
	if s.SpecPath == "" {
		return Result{Name: "clarify-recorded", Pass: false, Next: "No spec path recorded; cannot check for a Clarifications section."}
	}
	full := s.SpecPath
	if !filepath.IsAbs(full) {
		full = filepath.Join(root, full)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return Result{Name: "clarify-recorded", Pass: false, Next: fmt.Sprintf("Could not read %s: %v", s.SpecPath, err)}
	}
	if !strings.Contains(string(data), "## Clarifications") {
		return Result{
			Name: "clarify-recorded",
			Pass: false,
			Next: fmt.Sprintf("%s has no '## Clarifications' section. Ask up to 5 targeted questions and record the answers there, or use clarify-skipped instead.", s.SpecPath),
		}
	}
	return Result{Name: "clarify-recorded", Pass: true}
}

func checkClarifySkipAcknowledged(_ string, s flow.State) Result {
	if !s.ClarifySkipped {
		return Result{
			Name: "clarify-skip-acknowledged",
			Pass: false,
			Next: "Set clarify_skipped=true in state only after explicitly telling the human you are skipping clarification and why.",
		}
	}
	return Result{Name: "clarify-skip-acknowledged", Pass: true}
}

func checkAnalyzeSkipAcknowledged(_ string, s flow.State) Result {
	if !s.AnalyzeSkipped {
		return Result{
			Name: "analyze-skip-acknowledged",
			Pass: false,
			Next: "Set analyze_skipped=true in state only after explicitly telling the human you are skipping the consistency analysis and why.",
		}
	}
	return Result{Name: "analyze-skip-acknowledged", Pass: true}
}

func checkTasksAllChecked(root string, s flow.State) Result {
	if s.TasksPath == "" {
		return Result{Name: "tasks-all-checked", Pass: false, Next: "No tasks path recorded in state."}
	}
	full := s.TasksPath
	if !filepath.IsAbs(full) {
		full = filepath.Join(root, full)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return Result{Name: "tasks-all-checked", Pass: false, Next: fmt.Sprintf("Could not read %s: %v", s.TasksPath, err)}
	}
	unchecked := countUncheckedTaskLines(string(data))
	if unchecked > 0 {
		return Result{
			Name: "tasks-all-checked",
			Pass: false,
			Next: fmt.Sprintf("%s still has %d unchecked task(s).", s.TasksPath, unchecked),
		}
	}
	return Result{Name: "tasks-all-checked", Pass: true}
}

// countUncheckedTaskLines counts lines that are actually unchecked task
// items ("- [ ]" at the start of a line, ignoring leading whitespace),
// not every occurrence of that substring in the file. A naive substring
// count over the whole file is fooled by templates/docs that mention the
// checkbox syntax in prose (e.g. this package's own tasks-template.md
// explains the "- [ ]" convention in a comment) — those aren't tasks and
// must not block build-complete.
func countUncheckedTaskLines(data string) int {
	count := 0
	for line := range strings.SplitSeq(data, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- [ ]") {
			count++
		}
	}
	return count
}

func checkVerifyResultPass(_ string, s flow.State) Result {
	if s.VerifyResult != flow.VerifyPass {
		return Result{
			Name: "verify-result-pass",
			Pass: false,
			Next: fmt.Sprintf("verify_result is %q, expected %q. Run the project's build/test command and record the outcome before advancing.", s.VerifyResult, flow.VerifyPass),
		}
	}
	return Result{Name: "verify-result-pass", Pass: true}
}

func checkVerifyResultFail(_ string, s flow.State) Result {
	if s.VerifyResult != flow.VerifyFail {
		return Result{
			Name: "verification-failed",
			Pass: false,
			Next: fmt.Sprintf("verify_result is %q, not %q; verify-fail should only be used when verification actually failed.", s.VerifyResult, flow.VerifyFail),
		}
	}
	return Result{Name: "verification-failed", Pass: true}
}

func checkArchiveConfirmed(_ string, s flow.State) Result {
	if s.ArchiveConfirmation != flow.ArchiveConfirmed {
		return Result{
			Name: "archive-confirmed",
			Pass: false,
			Next: "archive_confirmation is not 'confirmed'. Present the merge/PR/keep/discard menu and get an explicit human choice before archiving.",
		}
	}
	return Result{Name: "archive-confirmed", Pass: true}
}
