package quality

import (
	"fmt"
	"path"
	"reflect"
	"strings"
)

// Latest deliberately returns a later failed attempt instead of falling back
// to an older pass. Retrying a broken suite must invalidate its old green.
func Latest(records []RunRecord, task int, stage string) (RunRecord, bool) {
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Task == task && records[i].Stage == stage {
			return records[i], true
		}
	}
	return RunRecord{}, false
}
func validReceipt(c Contract, r RunRecord) error {
	if !r.Success || r.Error != "" {
		return fmt.Errorf("run %s is not successful: %s", r.ID, r.Error)
	}
	if err := validateStage(c, r.Task, r.Stage); err != nil {
		return err
	}
	return outcome(c, r)
}
func ValidateTaskEvidence(p Project, c Contract, records []RunRecord) error {
	for _, task := range c.Tasks {
		current, err := Fingerprint(p, c, task.Number)
		if err != nil {
			return err
		}
		stage := "check"
		if task.Kind == "code" {
			stage = "green"
		}
		green, ok := Latest(records, task.Number, stage)
		if !ok {
			return fmt.Errorf("task %d lacks runner-generated %s evidence; use mulix test run", task.Number, stage)
		}
		if err := validReceipt(c, green); err != nil {
			return err
		}
		if green.Snapshot.Project != current.Project || green.Snapshot.Change != p.Change || green.Snapshot.Contract != current.Contract {
			return fmt.Errorf("task %d evidence belongs to another project/change or obsolete acceptance contract", task.Number)
		}
		if task.Kind == "code" {
			red, ok := Latest(records, task.Number, "red")
			if !ok {
				return fmt.Errorf("task %d has no actual RED run", task.Number)
			}
			if err := validReceipt(c, red); err != nil {
				return err
			}
			if red.ID == green.ID || red.Finished.After(green.Started) || red.Snapshot.Project != green.Snapshot.Project || red.Snapshot.Change != green.Snapshot.Change || red.Snapshot.Contract != green.Snapshot.Contract || red.Snapshot.Tests != green.Snapshot.Tests {
				return fmt.Errorf("task %d needs RED before GREEN for the same project, contract and test files", task.Number)
			}
			if red.Snapshot.Source == green.Snapshot.Source {
				return fmt.Errorf("task %d has no implementation change between RED and GREEN", task.Number)
			}
		}
	}
	return nil
}
func ValidateReview(root, report string) error {
	data, err := ReadProjectFile(root, report)
	if err != nil {
		return err
	}
	var review struct {
		Reviewer string   `yaml:"reviewer"`
		Result   string   `yaml:"result"`
		Blocking []string `yaml:"blocking"`
	}
	if err := Block(data, "mulix-review", &review); err != nil {
		return err
	}
	if !concrete(review.Reviewer) || review.Result != "approved" || len(review.Blocking) != 0 {
		return fmt.Errorf("review requires an identified reviewer, approved result and no blocking findings")
	}
	return nil
}
func ValidateVerification(p Project, c Contract, records []RunRecord, report string) error {
	expected := path.Join("docs/changes", p.Change, "report.md")
	if report != expected {
		return fmt.Errorf("verification report must be %s", expected)
	}
	r, ok := Latest(records, 0, "verify")
	if !ok {
		return fmt.Errorf("no runner-generated verification; run mulix verify")
	}
	if err := validReceipt(c, r); err != nil {
		return err
	}
	current, err := Fingerprint(p, c, 0)
	if err != nil {
		return err
	}
	if r.Snapshot != current {
		return fmt.Errorf("verification is stale or belongs to another change/project; run mulix verify again")
	}
	policy, err := LoadPolicy(p.Root)
	if err != nil {
		return err
	}
	if len(r.Suites) != len(policy.Suites) {
		return fmt.Errorf("verification did not execute every mandatory suite")
	}
	for i, suite := range policy.Suites {
		actual := r.Suites[i]
		if actual.ID != suite.ID || actual.CWD != suite.CWD || actual.Format != suite.Format || !reflect.DeepEqual(actual.Command, suite.Command) {
			return fmt.Errorf("verification suite does not match current policy: %s", suite.ID)
		}
	}
	data, err := ReadProjectFile(p.Root, report)
	if err != nil {
		return err
	}
	if r.ReportDigest == "" || r.ReportDigest != Digest(data) {
		return fmt.Errorf("verification report changed after execution")
	}
	return ValidateReview(p.Root, report)
}

// Summary is retained in existing reports; full streams stay in private
// context scratch. Stream digests and per-case identities remain in state.
func Summary(r RunRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n### Quality run %s\n\n- Stage: %s; task: %d; success: %v\n- Started: %s\n- Finished: %s\n- Source: %s\n- Contract: %s\n", r.ID, r.Stage, r.Task, r.Success, r.Started.Format("2006-01-02T15:04:05.999999999Z07:00"), r.Finished.Format("2006-01-02T15:04:05.999999999Z07:00"), r.Snapshot.Source, r.Snapshot.Contract)
	for _, s := range r.Suites {
		fmt.Fprintf(&b, "- Suite %s: exit %d; passed %d, failed %d, skipped %d\n  - Command: %q (cwd %s)\n  - stdout SHA-256: %s\n  - stderr SHA-256: %s\n", s.ID, s.ExitCode, s.Passed, s.Failed, s.Skipped, s.Command, s.CWD, s.StdoutDigest, s.StderrDigest)
		if s.Error != "" {
			fmt.Fprintf(&b, "  - Error: %s\n", s.Error)
		}
	}
	if r.Error != "" {
		fmt.Fprintf(&b, "- Failure: %s\n", r.Error)
	}
	return b.String()
}
