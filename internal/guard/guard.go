// Package guard implements the phase-exit readiness checks that gate
// flow.Apply calls. Each check inspects the filesystem/state for concrete
// evidence (an artifact exists and is non-empty, every task in tasks.md
// has a completion line in the execution ledger, a verification report
// was written) rather than trusting the agent's say-so. Guards never
// mutate state; Run only reports pass/fail plus a remediation hint.
package guard

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/layout"
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
	flow.EventDesignApproved:  {checkDesignApproved},
	flow.EventTasksComplete:   {checkTasksInChangeDir, checkTasksHaveSections},
	flow.EventBuildComplete:   {checkExecutionMethodChosen, checkPlanTasksComplete, checkTddEvidencePresent},
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

// resolve makes a state-recorded, root-relative path absolute.
func resolve(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, filepath.FromSlash(p))
}

// checkArtifactPresent builds a check that verifies a state-recorded path
// exists on disk and is non-empty.
func checkArtifactPresent(name string, getPath func(flow.State) string) checkFunc {
	return func(root string, s flow.State) Result {
		p := getPath(s)
		if p == "" {
			return Result{Name: name, Pass: false, Next: fmt.Sprintf("%s: no path recorded in state yet.", name)}
		}
		info, err := os.Stat(resolve(root, p))
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
	data, err := os.ReadFile(resolve(root, s.SpecPath))
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

// checkTasksHaveSections verifies tasks.md is an executable plan: at least
// one "## Task N" section, numbered 1..n without gaps or duplicates. The
// build phase's executors extract task briefs by that heading, and the
// build-complete guard matches ledger lines to it.
func checkTasksHaveSections(root string, s flow.State) Result {
	const name = "tasks-have-task-sections"
	if s.TasksPath == "" {
		return Result{Name: name, Pass: false, Next: "No tasks path recorded in state."}
	}
	data, err := os.ReadFile(resolve(root, s.TasksPath))
	if err != nil {
		return Result{Name: name, Pass: false, Next: fmt.Sprintf("Could not read %s: %v", s.TasksPath, err)}
	}
	tasks := TaskNumbers(string(data))
	if len(tasks) == 0 {
		return Result{Name: name, Pass: false, Next: fmt.Sprintf("%s has no \"## Task N: <name>\" sections. Every task is its own section, numbered from 1.", s.TasksPath)}
	}
	for i, n := range tasks {
		if n != i+1 {
			return Result{Name: name, Pass: false, Next: fmt.Sprintf("%s numbers its tasks %v; they must run 1..%d in order, each exactly once.", s.TasksPath, tasks, len(tasks))}
		}
	}
	return Result{Name: name, Pass: true}
}

// checkTasksInChangeDir verifies tasks.md exists, is non-empty, and lives
// in the change's own docs/changes/<change>/ directory — the one place the
// tasks phase may write it.
func checkTasksInChangeDir(root string, s flow.State) Result {
	const name = "tasks-artifact-present"
	if res := checkArtifactPresent(name, func(s flow.State) string { return s.TasksPath })(root, s); !res.Pass {
		return res
	}
	want := path.Join(layout.ChangesDir, s.Change)
	if !isUnder(path.Clean(filepath.ToSlash(s.TasksPath)), want) {
		return Result{Name: name, Pass: false, Next: fmt.Sprintf("tasks_path %s is outside %s/.", s.TasksPath, want)}
	}
	return Result{Name: name, Pass: true}
}

// checkDesignApproved verifies the design phase's brainstorming reached
// its gate: a classified track, an explicit human approval, and — for the
// architectural track — a non-empty written design doc inside the
// change's runtime specs directory.
func checkDesignApproved(root string, s flow.State) Result {
	const name = "design-approved"
	if !s.DesignTrack.Valid() {
		return Result{Name: name, Pass: false, Next: fmt.Sprintf("design_track is %q. Classify the change (bounded|architectural) out loud, then record it with `mulix state set design_track <track>`.", s.DesignTrack)}
	}
	if s.DesignTrack == flow.TrackArchitectural {
		want := layout.ChangeRuntimePath(s.Change, layout.DesignDir)
		if s.DesignPath == "" {
			return Result{Name: name, Pass: false, Next: fmt.Sprintf("The architectural track needs a written design doc under %s/, recorded with `mulix state set design_path <path>`.", want)}
		}
		if !isUnder(path.Clean(filepath.ToSlash(s.DesignPath)), want) {
			return Result{Name: name, Pass: false, Next: fmt.Sprintf("design_path %s is outside %s/.", s.DesignPath, want)}
		}
		info, err := os.Stat(resolve(root, s.DesignPath))
		if err != nil || info.Size() == 0 {
			return Result{Name: name, Pass: false, Next: fmt.Sprintf("Design doc %s is missing or empty.", s.DesignPath)}
		}
	}
	if !s.DesignApproved {
		return Result{Name: name, Pass: false, Next: "design_approved is false. Present the design (in chat for bounded, the written spec for architectural), wait for an explicit yes, then `mulix state set design_approved true`."}
	}
	return Result{Name: name, Pass: true}
}

func checkExecutionMethodChosen(_ string, s flow.State) Result {
	const name = "execution-method-chosen"
	if !s.ExecutionMethod.Valid() {
		return Result{Name: name, Pass: false, Next: fmt.Sprintf("execution_method is %q. At the start of build, recommend subagent-driven or inline, get the human's choice, then `mulix state set execution_method <method>`.", s.ExecutionMethod)}
	}
	return Result{Name: name, Pass: true}
}

// checkPlanTasksComplete verifies every "## Task N" section of tasks.md
// has a "Task N: complete" line in the plan's execution ledger. The ledger
// (progress.md in the plan's workspace under .mulix/.runtime/<change>/
// sdd/) is written by the build phase's executor; its first line must name
// tasks.md, so a ledger belonging to another plan never counts.
func checkPlanTasksComplete(root string, s flow.State) Result {
	const name = "plan-tasks-complete"
	tasks, ws, res := loadPlanAndWorkspace(root, s, name)
	if !res.Pass {
		return res
	}
	ledger, err := os.ReadFile(filepath.Join(ws, "progress.md"))
	if err != nil {
		return Result{Name: name, Pass: false, Next: fmt.Sprintf("No ledger at %s. The build phase's executor writes one line per completed task there.", slashRel(root, filepath.Join(ws, "progress.md")))}
	}
	first, _, _ := strings.Cut(string(ledger), "\n")
	if !strings.HasPrefix(strings.TrimSpace(first), "# SDD ledger") || !strings.HasSuffix(strings.TrimSpace(first), path.Base(filepath.ToSlash(s.TasksPath))) {
		return Result{Name: name, Pass: false, Next: fmt.Sprintf("Ledger %s does not start with \"# SDD ledger — plan: %s\".", slashRel(root, filepath.Join(ws, "progress.md")), s.TasksPath)}
	}
	done := CompletedTasks(string(ledger))
	var missing []int
	for _, n := range tasks {
		if !done[n] {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return Result{Name: name, Pass: false, Next: fmt.Sprintf("%d of %d task(s) have no \"Task N: complete\" ledger line yet: %v.", len(missing), len(tasks), missing)}
	}
	return Result{Name: name, Pass: true}
}

// checkTddEvidencePresent verifies every task left a TDD trace: a
// task-N-report.md in the plan's workspace carrying both a RED entry (the
// failing run, before implementation) and a GREEN entry (the passing run
// after). Both executors write it — the subagent-driven implementer by its
// report contract, the inline executor per the mulix-build skill.
func checkTddEvidencePresent(root string, s flow.State) Result {
	const name = "tdd-evidence-present"
	tasks, ws, res := loadPlanAndWorkspace(root, s, name)
	if !res.Pass {
		return res
	}
	var missing []string
	for _, n := range tasks {
		report := filepath.Join(ws, fmt.Sprintf("task-%d-report.md", n))
		data, err := os.ReadFile(report)
		if err != nil {
			missing = append(missing, fmt.Sprintf("Task %d (no report)", n))
			continue
		}
		if problem := tddEvidenceProblem(string(data)); problem != "" {
			missing = append(missing, fmt.Sprintf("Task %d (%s)", n, problem))
		}
	}
	if len(missing) > 0 {
		return Result{Name: name, Pass: false, Next: fmt.Sprintf("%d task(s) lack TDD evidence in %s/task-N-report.md, e.g. %s. Each report needs a \"RED:\" line (failing command and output before implementing) and a \"GREEN:\" line (passing command and output after).", len(missing), slashRel(root, ws), missing[0])}
	}
	return Result{Name: name, Pass: true}
}

// tddEvidenceProblem returns "" when report records both TDD phases: a
// line whose text (after list/emphasis markers) starts with "RED" and one
// that starts with "GREEN", each followed by content.
func tddEvidenceProblem(report string) string {
	var red, green bool
	for line := range strings.SplitSeq(report, "\n") {
		t := strings.TrimLeft(strings.TrimSpace(line), "-*# ")
		t = strings.TrimLeft(t, "*_")
		switch {
		case hasPhase(t, "RED"):
			red = true
		case hasPhase(t, "GREEN"):
			green = true
		}
	}
	switch {
	case !red && !green:
		return "no RED/GREEN evidence"
	case !red:
		return "no RED evidence"
	case !green:
		return "no GREEN evidence"
	}
	return ""
}

// hasPhase reports whether t is "<PHASE>" followed by a separator and some
// content, e.g. "RED: go test ./... → FAIL ...".
func hasPhase(t, phase string) bool {
	rest, ok := strings.CutPrefix(t, phase)
	if !ok {
		return false
	}
	rest = strings.TrimLeft(rest, "*_ ")
	rest, ok = strings.CutPrefix(rest, ":")
	if !ok {
		rest, ok = strings.CutPrefix(rest, "—")
	}
	if !ok {
		rest, ok = strings.CutPrefix(rest, "-")
	}
	return ok && strings.TrimSpace(strings.TrimLeft(rest, "*_")) != ""
}

// loadPlanAndWorkspace reads tasks.md's task numbers and locates its
// execution workspace, reporting a failed Result under name if either is
// unavailable.
func loadPlanAndWorkspace(root string, s flow.State, name string) ([]int, string, Result) {
	if s.TasksPath == "" {
		return nil, "", Result{Name: name, Pass: false, Next: "No tasks path recorded in state."}
	}
	data, err := os.ReadFile(resolve(root, s.TasksPath))
	if err != nil {
		return nil, "", Result{Name: name, Pass: false, Next: fmt.Sprintf("Could not read %s: %v", s.TasksPath, err)}
	}
	tasks := TaskNumbers(string(data))
	if len(tasks) == 0 {
		return nil, "", Result{Name: name, Pass: false, Next: fmt.Sprintf("%s has no \"## Task N\" sections.", s.TasksPath)}
	}
	ws, ok := FindWorkspace(root, s)
	if !ok {
		return nil, "", Result{Name: name, Pass: false, Next: fmt.Sprintf("No execution workspace for %s under %s/. Run the build through the executor skill (it resolves the workspace with sdd-workspace).", s.TasksPath, layout.ChangeRuntimePath(s.Change, layout.SddDir))}
	}
	return tasks, ws, Result{Name: name, Pass: true}
}

// FindWorkspace returns the absolute path of tasks.md's plan workspace
// under .mulix/.runtime/<change>/sdd/: the subdirectory whose plan-path
// marker names tasks.md. The marker is git-root-relative (or absolute),
// so it's compared by path suffix against the root-relative TasksPath. A
// marker-less sdd/<tasks basename>/ is accepted as a fallback.
func FindWorkspace(root string, s flow.State) (string, bool) {
	base := filepath.Join(root, filepath.FromSlash(layout.ChangeRuntimePath(s.Change, layout.SddDir)))
	entries, err := os.ReadDir(base)
	if err != nil {
		return "", false
	}
	want := path.Clean(filepath.ToSlash(s.TasksPath))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		marker, err := os.ReadFile(filepath.Join(base, e.Name(), "plan-path"))
		if err != nil {
			continue
		}
		got := path.Clean(filepath.ToSlash(strings.TrimSpace(string(marker))))
		if got == want || strings.HasSuffix(got, "/"+want) {
			return filepath.Join(base, e.Name()), true
		}
	}
	fallback := filepath.Join(base, strings.TrimSuffix(path.Base(want), ".md"))
	if info, err := os.Stat(fallback); err == nil && info.IsDir() {
		if _, err := os.Stat(filepath.Join(fallback, "plan-path")); os.IsNotExist(err) {
			return fallback, true
		}
	}
	return "", false
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
			Next: "archive_confirmation is not 'confirmed'. Present the merge/PR/keep menu and get an explicit human choice before archiving.",
		}
	}
	return Result{Name: "archive-confirmed", Pass: true}
}

// isUnder reports whether rel is exactly prefix or nested under it,
// segment-aware.
func isUnder(rel, prefix string) bool {
	prefix = strings.TrimSuffix(prefix, "/")
	return rel == prefix || strings.HasPrefix(rel, prefix+"/")
}

// slashRel renders p relative to root with forward slashes, for hints.
func slashRel(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}
