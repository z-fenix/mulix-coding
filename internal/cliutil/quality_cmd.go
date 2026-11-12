package cliutil

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/guard"
	"github.com/z-fenix/mulix-coding/internal/quality"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func newTestCmd() *cobra.Command {
	var change, stage string
	var task int
	parent := &cobra.Command{Use: "test", Short: "Run configured tests and record actual task evidence"}
	run := &cobra.Command{Use: "run", Short: "Execute a task's red, green or non-code check stage", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if task < 1 || (stage != "red" && stage != "green" && stage != "check") {
			return fmt.Errorf("use --task N and --stage red|green|check")
		}
		return executeQuality(cmd, change, task, stage)
	}}
	run.Flags().IntVar(&task, "task", 0, "approved task number")
	run.Flags().StringVar(&stage, "stage", "", "red, green, or check for a reasoned non-code task")
	run.Flags().StringVar(&change, "change", "", "change id (must match effective session context)")
	parent.AddCommand(run)
	return parent
}
func newVerifyCmd() *cobra.Command {
	var change string
	cmd := &cobra.Command{Use: "verify", Short: "Run every mandatory suite and bind the delivery report to current inputs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error { return executeQuality(cmd, change, 0, "verify") }}
	cmd.Flags().StringVar(&change, "change", "", "change id (must match effective session context)")
	return cmd
}
func replaceRun(s *flow.State, record quality.RunRecord) {
	var records []quality.RunRecord
	for _, old := range s.QualityRuns {
		if old.Task != record.Task || old.Stage != record.Stage {
			records = append(records, old)
		}
	}
	s.QualityRuns = append(records, record)
}
func appendQualityReport(root, rel string, record quality.RunRecord) (string, error) {
	full, err := fileutil.ControlPath(root, rel, true)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(full)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if len(data) == 0 {
		data = []byte("# Execution evidence\n")
	}
	data = append(data, []byte(quality.Summary(record))...)
	if err := fileutil.WriteFile(full, data, 0644); err != nil {
		return "", err
	}
	return quality.Digest(data), nil
}
func taskReportPath(root string, s flow.State) (string, error) {
	workspace, ok := guard.FindWorkspace(root, s)
	if !ok {
		rel := path.Join(".mulix/.runtime", s.Change, "sdd", "tasks", "plan-path")
		full, err := fileutil.ControlPath(root, rel, true)
		if err != nil {
			return "", err
		}
		if _, err := os.Lstat(full); err == nil {
			return "", fmt.Errorf("existing SDD workspace belongs to another plan")
		} else if !os.IsNotExist(err) {
			return "", err
		}
		if err := fileutil.WriteFile(full, []byte(s.TasksPath+"\n"), 0644); err != nil {
			return "", err
		}
		workspace = filepath.Dir(full)
	}
	rel, err := filepath.Rel(root, workspace)
	return filepath.ToSlash(rel), err
}
func executeQuality(cmd *cobra.Command, change string, task int, stage string) error {
	store, r, err := resolveCommandContext(cmd, change, sessionctx.Mutate)
	if err != nil {
		return err
	}
	if err := requireChange(r); err != nil {
		return err
	}
	initial := *r.State
	want := flow.PhaseBuild
	if stage == "verify" {
		want = flow.PhaseVerify
		// Finishing can change the Git revision or source after verify-pass.
		// Recheck in place without bypassing archive's current-evidence guard.
		if initial.Phase == flow.PhaseArchive {
			want = flow.PhaseArchive
		}
	}
	if initial.Phase != want || initial.Archived {
		return fmt.Errorf("%s runs only in active %s phase", stage, want)
	}
	p := quality.Project{Root: store.Root(), Change: r.Change, SpecPath: initial.SpecPath, TasksPath: initial.TasksPath}
	contract, err := quality.LoadContract(p)
	if err != nil {
		return err
	}
	policy, err := quality.LoadPolicy(p.Root)
	if err != nil {
		return err
	}
	if err := quality.ValidateMapping(contract, policy); err != nil {
		return err
	}
	report := path.Join("docs/changes", r.Change, "report.md")
	if task != 0 {
		workspace, err := taskReportPath(p.Root, initial)
		if err != nil {
			return err
		}
		report = path.Join(workspace, fmt.Sprintf("task-%d-report.md", task))
	}
	// Invalidate the old result before starting subprocesses. Interruptions and
	// failed final writes must never leave an earlier green usable.
	pending := quality.RunRecord{ID: "pending-" + rand.Text(), Task: task, Stage: stage, Started: time.Now().UTC(), Error: "execution started but has not completed"}
	_, err = state.Update(cmd.Context(), p.Root, r.Change, func(current *flow.State) error {
		if current.Phase != want || current.Archived || current.SpecPath != initial.SpecPath || current.TasksPath != initial.TasksPath {
			return fmt.Errorf("change phase or inputs changed before execution")
		}
		replaceRun(current, pending)
		if task == 0 {
			current.VerifyResult = flow.VerifyPending
			current.ReportPath = report
		}
		return nil
	}, nil)
	if err != nil {
		return err
	}
	var runErr error
	if task == 0 {
		runErr = quality.ValidateTaskEvidence(p, contract, initial.QualityRuns)
		if runErr == nil {
			runErr = quality.ValidateReview(p.Root, report)
		}
	}
	record := pending
	if runErr == nil {
		record, runErr = quality.Run(cmd.Context(), p, contract, policy, task, stage, path.Join(r.ScratchDir, "verification"))
	} else {
		record.Finished = time.Now().UTC()
		record.Error = runErr.Error()
	}
	// Persist a failed/cancelled attempt using a bounded independent context.
	// The test command itself never runs under the change lock.
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(cmd.Context()), 5*time.Second)
	defer cancel()
	_, saveErr := state.Update(persistCtx, p.Root, r.Change, func(current *flow.State) error {
		if current.Phase != want || current.Archived || current.SpecPath != initial.SpecPath || current.TasksPath != initial.TasksPath {
			return fmt.Errorf("change phase or inputs changed during execution; result not accepted")
		}
		latest, ok := quality.Latest(current.QualityRuns, task, stage)
		if !ok || latest.ID != pending.ID {
			return fmt.Errorf("a newer attempt superseded this execution")
		}
		if record.Snapshot.Source != "" {
			now, e := quality.Fingerprint(p, contract, task)
			if e != nil || now != record.Snapshot {
				record.Success = false
				record.Error = "inputs changed before evidence persistence"
				runErr = fmt.Errorf("%s", record.Error)
			}
		}
		if record.Success && task == 0 {
			if e := quality.ValidateTaskEvidence(p, contract, current.QualityRuns); e != nil {
				record.Success = false
				record.Error = e.Error()
				runErr = e
			}
			if e := quality.ValidateReview(p.Root, report); e != nil {
				record.Success = false
				record.Error = e.Error()
				runErr = e
			}
		}
		// The independent context permits persisting failure after cancellation;
		// it must not turn a cancelled successful subprocess into pass evidence.
		if e := cmd.Context().Err(); e != nil && record.Success {
			record.Success = false
			record.Error = fmt.Sprintf("execution cancelled before evidence persistence: %v", e)
			runErr = e
		}
		digest, e := appendQualityReport(p.Root, report, record)
		if e != nil {
			return e
		}
		if task == 0 {
			record.ReportDigest = digest
		}
		replaceRun(current, record)
		if task == 0 {
			if record.Success {
				current.VerifyResult = flow.VerifyPass
			} else {
				current.VerifyResult = flow.VerifyFail
			}
		}
		return nil
	}, nil)
	fmt.Print(quality.Summary(record))
	if saveErr != nil {
		return fmt.Errorf("could not persist execution evidence (previous result remains invalidated): %w", saveErr)
	}
	if runErr != nil {
		return runErr
	}
	printContextSummary(r)
	return nil
}
