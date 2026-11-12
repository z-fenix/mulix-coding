package guard

import (
	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/quality"
)

func qualityProject(root string, s flow.State) quality.Project {
	return quality.Project{Root: root, Change: s.Change, SpecPath: s.SpecPath, TasksPath: s.TasksPath}
}
func qualityResult(name string, err error) Result {
	if err != nil {
		return Result{Name: name, Next: err.Error()}
	}
	return Result{Name: name, Pass: true}
}
func checkAcceptanceCriteria(root string, s flow.State) Result {
	_, err := quality.ValidateSpec(qualityProject(root, s))
	return qualityResult("acceptance-criteria-present", err)
}
func checkAcceptanceMatrix(root string, s flow.State) Result {
	c, err := quality.LoadContract(qualityProject(root, s))
	if err == nil {
		var p quality.Policy
		p, err = quality.LoadPolicy(root)
		if err == nil {
			err = quality.ValidateMapping(c, p)
		}
	}
	return qualityResult("acceptance-matrix-complete", err)
}
func checkTaskExecutionEvidence(root string, s flow.State) Result {
	p := qualityProject(root, s)
	c, err := quality.LoadContract(p)
	if err == nil {
		err = quality.ValidateTaskEvidence(p, c, s.QualityRuns)
	}
	return qualityResult("tdd-evidence-present", err)
}
func checkVerificationEvidence(root string, s flow.State) Result {
	p := qualityProject(root, s)
	c, err := quality.LoadContract(p)
	if err == nil {
		err = quality.ValidateTaskEvidence(p, c, s.QualityRuns)
	}
	if err == nil {
		err = quality.ValidateVerification(p, c, s.QualityRuns, s.ReportPath)
	}
	return qualityResult("verification-evidence-current", err)
}
