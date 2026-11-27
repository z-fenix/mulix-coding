// Package guard implements the phase-exit readiness checks that gate
// flow.Apply calls. Each check inspects the filesystem/state for concrete
// evidence (artifacts are non-empty, every approved task has an execution
// ledger entry, a verification report
// was written) rather than trusting the agent's say-so. Guards never
// mutate state; Run only reports pass/fail plus a remediation hint.
package guard

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/layout"
	"github.com/z-fenix/mulix-coding/internal/quality"
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

// checksByName supplies the implementations; flow.Table alone decides
// which checks a transition needs and in what order.
var checksByName = map[string]checkFunc{
	"spec-artifact-present":         checkArtifactPresent("spec-artifact-present", func(s flow.State) string { return s.SpecPath }),
	"clarify-recorded":              checkClarifyRecorded,
	"clarify-skip-acknowledged":     checkClarifySkipAcknowledged,
	"design-approved":               checkDesignApproved,
	"tasks-artifact-present":        checkTasksInChangeDir,
	"tasks-have-task-sections":      checkTasksHaveSections,
	"execution-method-chosen":       checkExecutionMethodChosen,
	"plan-tasks-complete":           checkPlanTasksComplete,
	"tdd-evidence-present":          checkTddEvidencePresent,
	"verification-report-present":   checkArtifactPresent("verification-report-present", func(s flow.State) string { return s.ReportPath }),
	"verify-result-pass":            checkVerifyResultPass,
	"verification-failed":           checkVerifyResultFail,
	"archive-confirmed":             checkArchiveConfirmed,
	"acceptance-criteria-present":   checkAcceptanceCriteria,
	"acceptance-matrix-complete":    checkAcceptanceMatrix,
	"verification-evidence-current": checkVerificationEvidence,
}

// Run executes the legal transition's declared checks. Illegal events and
// unregistered references fail closed rather than silently omitting checks.
func Run(root string, s flow.State, event flow.Event) Report {
	report := Report{Event: event}
	transition, ok := flow.Find(s.Phase, event)
	if !ok {
		report.Results = append(report.Results, Result{
			Name: "legal-transition",
			Next: fmt.Sprintf("Event %q is not legal from phase %q.", event, s.Phase),
		})
		return report
	}
	for _, name := range transition.GuardRefs {
		check, ok := checksByName[name]
		if !ok {
			report.Results = append(report.Results, Result{
				Name: name,
				Next: fmt.Sprintf("Guard %q is not registered; fix the guard registry before transitioning.", name),
			})
			continue
		}
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

// checkTasksHaveSections verifies the plan has at least one task, with no
// gaps or duplicate headings. For native plans the validated reader also
// checks the headings and matrix ownership of each individual document.
func checkTasksHaveSections(root string, s flow.State) Result {
	const name = "tasks-have-task-sections"
	if s.ArtifactLayout == flow.RuntimeIssuesLayout {
		_, err := quality.ReadTaskDocuments(s.QualityProject(root))
		return qualityResult(name, err)
	}
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

// checkTasksInChangeDir locates the task artifact in the change's own paths.
func checkTasksInChangeDir(root string, s flow.State) Result {
	const name = "tasks-artifact-present"
	if s.ArtifactLayout == flow.RuntimeIssuesLayout {
		want := layout.ChangeRuntimePath(s.Change, layout.RuntimeIssuesDir)
		if s.PlanPath() != want {
			return qualityResult(name, fmt.Errorf("task directory must be the change's own %s", want))
		}
		probe, err := fileutil.ControlPath(root, want+"/.probe", false)
		if err == nil {
			// An existing non-directory component was rejected by ControlPath.
			_, err = os.Stat(filepath.Dir(probe))
		}
		return qualityResult(name, err)
	}
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

// checkPlanTasksComplete verifies every task has a completion line in the
// owning plan's ledger. Native identity names the entire task directory.
func checkPlanTasksComplete(root string, s flow.State) Result {
	const name = "plan-tasks-complete"
	tasks, ws, res := loadPlanAndWorkspace(root, s, name)
	if !res.Pass {
		return res
	}
	ledger, err := readPlanLedger(root, s, ws)
	if err != nil {
		return qualityResult(name, err)
	}
	var done map[int]bool
	if s.ArtifactLayout == flow.RuntimeIssuesLayout {
		done = completedReceiptTasks(ledger, tasks, s.QualityRuns)
	} else {
		done = CompletedTasks(ledger)
	}
	var missing []int
	for _, task := range tasks {
		if !done[task.Number] {
			missing = append(missing, task.Number)
		}
	}
	if len(missing) > 0 {
		if s.ArtifactLayout == flow.RuntimeIssuesLayout {
			return Result{Name: name, Pass: false, Next: fmt.Sprintf("Tasks %v need completion lines with their latest GREEN/CHECK receipt IDs: - Task N: complete (receipt <id>).", missing)}
		}
		return Result{Name: name, Pass: false, Next: fmt.Sprintf("%d of %d task(s) have no \"Task N: complete\" ledger line yet: %v.", len(missing), len(tasks), missing)}
	}
	return Result{Name: name, Pass: true}
}

func readPlanLedger(root string, s flow.State, ws string) (string, error) {
	ledgerPath := filepath.Join(ws, "progress.md")
	ledgerRel := slashRel(root, ledgerPath)
	var ledger []byte
	var err error
	if s.ArtifactLayout == flow.RuntimeIssuesLayout {
		ledger, err = quality.ReadProjectFile(root, ledgerRel)
	} else {
		ledger, err = os.ReadFile(ledgerPath)
	}
	if err != nil {
		return "", fmt.Errorf("no ledger at %s; the build phase's executor writes one line per completed task there: %w", ledgerRel, err)
	}
	first, _, _ := strings.Cut(string(ledger), "\n")
	identity := strings.TrimSpace(first)
	knownFormat := strings.HasPrefix(identity, "# SDD progress") || strings.HasPrefix(identity, "# SDD ledger")
	matches := knownFormat && strings.HasSuffix(identity, path.Base(filepath.ToSlash(s.TasksPath)))
	if s.ArtifactLayout == flow.RuntimeIssuesLayout {
		identity = strings.TrimSuffix(first, "\r")
		matches = identity == "# SDD progress — plan: "+s.PlanPath() || identity == "# SDD ledger — plan: "+s.PlanPath()
	}
	if !matches {
		return "", fmt.Errorf("progress %s does not start with \"# SDD progress — plan: %s\" (legacy \"# SDD ledger\" is also accepted)", ledgerRel, s.PlanPath())
	}
	return string(ledger), nil
}

// checkTddEvidencePresent requires runner-owned RED/GREEN receipts for code,
// or a reasoned CHECK receipt for a non-code task. Report prose is not evidence.
func checkTddEvidencePresent(root string, s flow.State) Result {
	return checkTaskExecutionEvidence(root, s)
}

// loadPlanAndWorkspace reads the plan's tasks and locates their execution workspace.
func loadPlanAndWorkspace(root string, s flow.State, name string) ([]quality.Task, string, Result) {
	var tasks []quality.Task
	if s.ArtifactLayout == flow.RuntimeIssuesLayout {
		contract, err := quality.LoadContract(s.QualityProject(root))
		if err != nil {
			return nil, "", qualityResult(name, err)
		}
		tasks = contract.Tasks
	} else {
		if s.TasksPath == "" {
			return nil, "", Result{Name: name, Pass: false, Next: "No tasks path recorded in state."}
		}
		data, err := os.ReadFile(resolve(root, s.TasksPath))
		if err != nil {
			return nil, "", Result{Name: name, Pass: false, Next: fmt.Sprintf("Could not read %s: %v", s.TasksPath, err)}
		}
		for _, n := range TaskNumbers(string(data)) {
			tasks = append(tasks, quality.Task{Number: n})
		}
		if len(tasks) == 0 {
			return nil, "", Result{Name: name, Pass: false, Next: fmt.Sprintf("%s has no \"## Task N\" sections.", s.TasksPath)}
		}
	}
	ws, ok := FindWorkspace(root, s)
	if !ok {
		return nil, "", Result{Name: name, Pass: false, Next: fmt.Sprintf("No execution workspace for %s under %s/. Run the build through the executor skill (it resolves the workspace with sdd-workspace).", s.PlanPath(), layout.ChangeRuntimePath(s.Change, layout.SddDir))}
	}
	return tasks, ws, Result{Name: name, Pass: true}
}

// FindWorkspace locates a plan's retained execution record. Native plans have
// one workspace bound by an exact Mulix-root-relative directory marker. Old
// single-file plans still use their historical marker and basename fallback.
func FindWorkspace(root string, s flow.State) (string, bool) {
	if s.ArtifactLayout == flow.RuntimeIssuesLayout {
		if s.PlanPath() != layout.ChangeRuntimePath(s.Change, layout.RuntimeIssuesDir) {
			return "", false
		}
		dir := layout.ChangeRuntimePath(s.Change, layout.SddDir, layout.RuntimeIssuesDir)
		marker, err := quality.ReadProjectFile(root, dir+"/plan-path")
		if err != nil || string(marker) != s.PlanPath()+"\n" {
			return "", false
		}
		return resolve(root, dir), true
	}
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
