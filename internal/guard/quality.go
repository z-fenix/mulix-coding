package guard

import (
	"fmt"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/quality"
)

func qualityResult(name string, err error) Result {
	if err != nil {
		return Result{Name: name, Next: err.Error()}
	}
	return Result{Name: name, Pass: true}
}
func checkAcceptanceCriteria(root string, s flow.State) Result {
	_, err := quality.ValidateSpec(s.QualityProject(root))
	return qualityResult("acceptance-criteria-present", err)
}
func checkAcceptanceMatrix(root string, s flow.State) Result {
	c, err := quality.LoadContract(s.QualityProject(root))
	if err == nil && s.ArtifactLayout != flow.RuntimeIssuesLayout {
		var p quality.Policy
		p, err = quality.LoadPolicy(root)
		if err == nil {
			err = quality.ValidateMapping(c, p)
		}
	}
	return qualityResult("acceptance-matrix-complete", err)
}
func checkTaskExecutionEvidence(root string, s flow.State) Result {
	p := s.QualityProject(root)
	c, err := quality.LoadContract(p)
	if err == nil {
		err = quality.ValidateTaskEvidence(p, c, s.QualityRuns)
	}
	return qualityResult("tdd-evidence-present", err)
}

// ValidateTaskClosure checks one native task against its current receipt and
// the owning plan's exact completion marker. Final phase guards may retain
// historical receipts after later tasks change source; close may not.
func ValidateTaskClosure(root string, s flow.State, n int) (quality.RunRecord, error) {
	if s.ArtifactLayout != flow.RuntimeIssuesLayout || s.Phase != flow.PhaseBuild || s.Archived {
		return quality.RunRecord{}, fmt.Errorf("task Issue closure requires an active native build phase")
	}
	p := s.QualityProject(root)
	c, err := quality.LoadContract(p)
	if err != nil {
		return quality.RunRecord{}, err
	}
	r, err := quality.ValidateTaskReceipt(p, c, s.QualityRuns, n)
	if err != nil {
		return quality.RunRecord{}, err
	}
	ws, ok := FindWorkspace(root, s)
	if !ok {
		return quality.RunRecord{}, fmt.Errorf("no execution workspace for %s; use the build executor's ledger", s.PlanPath())
	}
	ledger, err := readPlanLedger(root, s, ws)
	if err != nil {
		return quality.RunRecord{}, err
	}
	task, _ := c.Task(n) // ValidateTaskReceipt checked membership.
	if !completedReceiptTasks(ledger, []quality.Task{task}, []quality.RunRecord{r})[n] {
		return quality.RunRecord{}, fmt.Errorf("task %d needs a completion line with its current receipt: - Task %d: complete (receipt %s)", n, n, r.ID)
	}
	return r, nil
}

func checkVerificationEvidence(root string, s flow.State) Result {
	p := s.QualityProject(root)
	c, err := quality.LoadContract(p)
	if err == nil {
		err = quality.ValidateTaskEvidence(p, c, s.QualityRuns)
	}
	if err == nil {
		err = quality.ValidateVerification(p, c, s.QualityRuns, s.ReportPath)
	}
	return qualityResult("verification-evidence-current", err)
}
