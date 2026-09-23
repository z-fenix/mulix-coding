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
	"strconv"
	"strings"

	"github.com/mulix-dev/mulix-coding/internal/flow"
	"github.com/mulix-dev/mulix-coding/internal/scaffold"
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
	flow.EventBuildComplete:   {checkTasksAllChecked, checkTddEvidencePresent},
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

// checkTddEvidencePresent verifies that every checked task in tasks.md
// has an execution record: a non-empty task report at
// <tasks dir>/.runtime/sdd/task_<ID>_report.md whose "### Task"
// checklist shows the work actually done — at least one RED or GREEN
// phase ticked ("- [x]"), and no RED or GREEN phase left as an open
// "- [ ]" checkbox. REFACTOR is optional, so its checkbox state is
// unconstrained; a test-writing task's report legitimately ticks only
// its RED phase. tasks.md itself only describes the tasks (checkbox +
// one-line description); the report is where the TDD phases live. This
// is the structural half of the build phase's TDD discipline — the
// mulix-build skill drives the cycle, and this check makes sure the
// discipline left a trace before the change can advance to verify.
func checkTddEvidencePresent(root string, s flow.State) Result {
	const name = "tdd-evidence-present"
	if s.TasksPath == "" {
		return Result{Name: name, Pass: false, Next: "No tasks path recorded in state."}
	}
	tasksFull := s.TasksPath
	if !filepath.IsAbs(tasksFull) {
		tasksFull = filepath.Join(root, s.TasksPath)
	}
	data, err := os.ReadFile(tasksFull)
	if err != nil {
		return Result{Name: name, Pass: false, Next: fmt.Sprintf("Could not read %s: %v", s.TasksPath, err)}
	}

	sddDir := filepath.Join(filepath.Dir(filepath.ToSlash(s.TasksPath)), scaffold.RuntimeDirName, scaffold.RuntimeSddDirName)
	var missing []string
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- [x]") {
			continue
		}
		id := taskIDOf(trimmed)
		if id == trimmed {
			// No T-number ID: there is no report filename to even look
			// for, so the task cannot have left its execution record.
			missing = append(missing, trimmed)
			continue
		}
		reportPath := filepath.Join(sddDir, "task_"+id+"_report.md")
		reportFull := filepath.Join(root, reportPath)
		report, err := os.ReadFile(reportFull)
		if err != nil {
			missing = append(missing, id)
			continue
		}
		if len(report) == 0 || phaseTickProblem(string(report)) != "" {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		hint := fmt.Sprintf("%s has %d checked task(s) whose report is missing or shows unticked RED/GREEN phases, e.g. %q. Record the task's RED/GREEN/REFACTOR checklist in %s and tick each phase as it completes.", s.TasksPath, len(missing), missing[0], reportPathFor(sddDir, missing[0]))
		if s.DelegatedToSubagents {
			hint += " Delegating a task to a subagent does not exempt it — the dispatched subagent's report must still be there."
		}
		return Result{Name: name, Pass: false, Next: hint}
	}
	return Result{Name: name, Pass: true}
}

// phaseTickProblem inspects a task report's checkbox checklist and
// returns "" when the report is acceptable: at least one ticked RED or
// GREEN phase, and no RED or GREEN phase left unticked. REFACTOR is
// optional, so an unticked "- [ ] REFACTOR:" line is fine.
func phaseTickProblem(report string) string {
	ticked := 0
	for line := range strings.SplitSeq(report, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "- [x] RED:") || strings.HasPrefix(trimmed, "- [x] GREEN:"):
			ticked++
		case strings.HasPrefix(trimmed, "- [ ] RED:") || strings.HasPrefix(trimmed, "- [ ] GREEN:"):
			return "unticked RED/GREEN phase"
		}
	}
	if ticked == 0 {
		return "no ticked RED/GREEN phase"
	}
	return ""
}

// reportPathFor names the expected report file for an offender in the
// failure hint. For an ID-less task (the whole line is the "id") it
// just points at the sdd directory.
func reportPathFor(sddDir, id string) string {
	if strings.HasPrefix(id, "T") || strings.HasPrefix(id, "t") {
		return filepath.ToSlash(filepath.Join(sddDir, "task_"+id+"_report.md"))
	}
	return filepath.ToSlash(sddDir) + "/task_<ID>_report.md"
}

// taskIDOf extracts the leading task ID (T001, T042, ...) from a task
// line's description, falling back to the whole line when no ID matches —
// tasks are required to carry IDs, but the hint should still be useful if
// one doesn't.
func taskIDOf(trimmedTaskLine string) string {
	fields := strings.Fields(strings.TrimPrefix(trimmedTaskLine, "- [x]"))
	if len(fields) > 0 && (strings.HasPrefix(fields[0], "T") || strings.HasPrefix(fields[0], "t")) {
		if _, err := strconv.Atoi(strings.TrimPrefix(fields[0], "T")); err == nil {
			return fields[0]
		}
		if _, err := strconv.Atoi(strings.TrimPrefix(fields[0], "t")); err == nil {
			return fields[0]
		}
	}
	return trimmedTaskLine
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
