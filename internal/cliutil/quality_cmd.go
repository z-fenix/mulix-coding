package cliutil

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/guard"
	"github.com/z-fenix/mulix-coding/internal/layout"
	"github.com/z-fenix/mulix-coding/internal/lockfile"
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
	parent.AddCommand(run, newTestReceiptCmd())
	return parent
}

func newTestReceiptCmd() *cobra.Command {
	var change, plan string
	var task int
	var idOnly bool
	cmd := &cobra.Command{
		Use: "receipt", Short: "Check a current task receipt without running a suite", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if task < 1 {
				return fmt.Errorf("use --task N")
			}
			store, r, err := resolveCommandContext(cmd, change, sessionctx.Inspect)
			if err != nil {
				return err
			}
			if err := requireChange(r); err != nil {
				return err
			}
			root := store.Root()
			// Resolve selects the change, but its state snapshot can be replaced
			// by a retry. Read and check the current record under the same lock
			// that the runner holds when it invalidates or records an attempt.
			return lockfile.With(cmd.Context(), root, lockfile.ChangeKey(r.Change), func() error {
				if _, err := fileutil.ControlPath(root, layout.ChangeRuntimePath(r.Change, layout.StateFile), false); err != nil {
					return err
				}
				s, err := state.Load(root, r.Change)
				if err != nil {
					return err
				}
				if s.Change != r.Change {
					return fmt.Errorf("state identity %q does not match locked change %q", s.Change, r.Change)
				}
				if s.Phase != flow.PhaseBuild || s.Archived {
					return fmt.Errorf("task receipt checks for closure require an active build phase")
				}
				p := s.QualityProject(root)
				if cmd.Flags().Changed("plan-path") {
					if err := requireNativePlan(p, plan); err != nil {
						return err
					}
				}
				c, err := quality.LoadContract(p)
				if err != nil {
					return err
				}
				record, err := quality.ValidateTaskReceipt(p, c, s.QualityRuns, task)
				if err != nil {
					return err
				}
				if idOnly {
					fmt.Println(record.ID)
				} else {
					fmt.Printf("Task %d: current %s receipt %s\n", task, record.Stage, record.ID)
				}
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&task, "task", 0, "approved task number")
	cmd.Flags().StringVar(&change, "change", "", "change id (must match effective session context)")
	cmd.Flags().StringVar(&plan, "plan-path", "", "check the receipt only for this native plan directory")
	cmd.Flags().BoolVar(&idOnly, "id-only", false, "print only the validated receipt ID")
	_ = cmd.Flags().MarkHidden("plan-path")
	_ = cmd.Flags().MarkHidden("id-only")
	return cmd
}

func requireNativePlan(p quality.Project, plan string) error {
	if p.TasksDir == "" {
		return fmt.Errorf("current change does not use a native task directory")
	}
	// Resolve the supplied path from the caller's cwd, never from the Git
	// root. The canonical plan still goes through ControlPath when read.
	full, err := filepath.EvalSymlinks(plan)
	if err == nil && plan != "" {
		full, err = filepath.Abs(full)
		if err == nil {
			rel, err := filepath.Rel(p.Root, full)
			if err == nil && filepath.ToSlash(rel) == p.TasksDir {
				return nil
			}
		}
	}
	return fmt.Errorf("plan path %q is not the current change's task directory %s", plan, p.TasksDir)
}

func newTaskInputCmd() *cobra.Command {
	var plan string
	cmd := &cobra.Command{Use: "_task-input <number>", Hidden: true, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 || strconv.Itoa(n) != args[0] {
			return fmt.Errorf("task number must be a positive integer")
		}
		store, r, err := resolveCommandContext(cmd, "", sessionctx.Inspect)
		if err != nil {
			return err
		}
		if err := requireChange(r); err != nil {
			return err
		}
		if r.State.ArtifactLayout != flow.RuntimeIssuesLayout || r.Phase != flow.PhaseBuild || r.State.Archived {
			return fmt.Errorf("task input requires an active native build phase")
		}
		p := r.State.QualityProject(store.Root())
		if cmd.Flags().Changed("plan-path") {
			if err := requireNativePlan(p, plan); err != nil {
				return err
			}
		}
		docs, err := quality.ReadTaskDocuments(p)
		if err != nil {
			return err
		}
		for _, doc := range docs {
			if doc.Number == n {
				_, err := os.Stdout.Write(doc.Markdown)
				return err
			}
		}
		return fmt.Errorf("unknown task %d", n)
	}}
	cmd.Flags().StringVar(&plan, "plan-path", "", "require this exact native plan directory")
	return cmd
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
func appendQualityReport(root, rel, summary string, reviewed []byte) (string, error) {
	full, err := fileutil.ControlPath(root, rel, true)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(full)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if reviewed != nil && !bytes.Equal(data, reviewed) {
		return "", fmt.Errorf("reviewed report changed during verification")
	}
	if len(data) == 0 {
		data = []byte("# Execution evidence\n")
	}
	data = append(data, summary...)
	if err := fileutil.WriteFile(full, data, 0644); err != nil {
		return "", err
	}
	return quality.Digest(data), nil
}

func taskReceiptSummary(c quality.Contract, records []quality.RunRecord) string {
	var b strings.Builder
	b.WriteString("\n#### Task receipts checked for this verification\n\n")
	for _, task := range c.Tasks {
		if task.Kind == "code" {
			red, _ := quality.Latest(records, task.Number, "red")
			green, _ := quality.Latest(records, task.Number, "green")
			fmt.Fprintf(&b, "- Task %d: RED %s; GREEN %s\n", task.Number, red.ID, green.ID)
		} else {
			check, _ := quality.Latest(records, task.Number, "check")
			fmt.Fprintf(&b, "- Task %d: CHECK %s\n", task.Number, check.ID)
		}
	}
	return b.String()
}

func taskReportPath(root string, s flow.State) (string, error) {
	workspace, ok := guard.FindWorkspace(root, s)
	if !ok && s.ArtifactLayout == flow.RuntimeIssuesLayout {
		rel := layout.ChangeRuntimePath(s.Change, layout.SddDir, layout.RuntimeIssuesDir)
		// Create the SDD parent, but claim only a new workspace. An existing
		// directory without the exact marker must not be adopted.
		probe, err := fileutil.ControlPath(root, path.Dir(rel)+"/.probe", true)
		if err != nil {
			return "", err
		}
		dir := filepath.Join(filepath.Dir(probe), path.Base(rel))
		if err := os.Mkdir(dir, 0700); err != nil {
			if os.IsExist(err) {
				if _, ok := guard.FindWorkspace(root, s); ok {
					return rel, nil
				}
				return "", fmt.Errorf("existing SDD workspace is not bound to %s", s.PlanPath())
			}
			return "", err
		}
		if err := fileutil.WriteFile(filepath.Join(dir, "plan-path"), []byte(s.PlanPath()+"\n"), 0644); err != nil {
			_ = os.Remove(dir) // only remove the empty directory this call created
			return "", err
		}
		return rel, nil
	}
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
	p := initial.QualityProject(store.Root())
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
	report, err := quality.VerificationReportPath(p)
	if err != nil {
		return err
	}
	if task != 0 {
		workspace, err := taskReportPath(p.Root, initial)
		if err != nil {
			return err
		}
		report = path.Join(workspace, fmt.Sprintf("task-%d-report.md", task))
	}
	inputsMatch := func(s *flow.State) bool {
		return s.Phase == want && !s.Archived && s.SpecPath == p.SpecPath &&
			s.ArtifactLayout == initial.ArtifactLayout && s.PlanPath() == initial.PlanPath() &&
			(p.TasksDir == "" || s.ReportPath == p.ReportPath)
	}
	// Invalidate the old result before starting subprocesses. Interruptions and
	// failed final writes must never leave an earlier green usable.
	pending := quality.RunRecord{ID: "pending-" + rand.Text(), Task: task, Stage: stage, Started: time.Now().UTC(), Error: "execution started but has not completed"}
	_, err = state.Update(cmd.Context(), p.Root, r.Change, func(current *flow.State) error {
		if !inputsMatch(current) {
			return fmt.Errorf("change phase or inputs changed before execution")
		}
		replaceRun(current, pending)
		if task == 0 {
			current.VerifyResult = flow.VerifyPending
			if p.TasksDir == "" {
				current.ReportPath = report
			}
		}
		return nil
	}, nil)
	if err != nil {
		return err
	}
	var runErr error
	var reviewed []byte
	if task == 0 {
		runErr = quality.ValidateTaskEvidence(p, contract, initial.QualityRuns)
		if runErr == nil {
			if p.TasksDir != "" {
				reviewed, runErr = quality.ReadReviewedReport(p.Root, report)
			} else {
				runErr = quality.ValidateReview(p.Root, report)
			}
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
		if !inputsMatch(current) {
			return fmt.Errorf("change phase or inputs changed during execution; result not accepted")
		}
		if task != 0 && p.TasksDir != "" {
			if _, ok := guard.FindWorkspace(p.Root, *current); !ok {
				return fmt.Errorf("SDD workspace no longer belongs to %s", p.TasksDir)
			}
		}
		latest, ok := quality.Latest(current.QualityRuns, task, stage)
		if !ok || latest.ID != pending.ID {
			return fmt.Errorf("a newer attempt superseded this execution")
		}
		if record.Success {
			now, e := quality.ExecutionFingerprint(p, contract, task)
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
			if p.TasksDir == "" {
				if e := quality.ValidateReview(p.Root, report); e != nil {
					record.Success = false
					record.Error = e.Error()
					runErr = e
				}
			}
		}
		// The independent context permits persisting failure after cancellation;
		// it must not turn a cancelled successful subprocess into pass evidence.
		if e := cmd.Context().Err(); e != nil && record.Success {
			record.Success = false
			record.Error = fmt.Sprintf("execution cancelled before evidence persistence: %v", e)
			runErr = e
		}
		summary := quality.Summary(record)
		if record.Success && task == 0 && p.TasksDir != "" {
			summary += taskReceiptSummary(contract, current.QualityRuns)
		}
		digest, e := appendQualityReport(p.Root, report, summary, reviewed)
		if e != nil {
			if p.TasksDir != "" {
				record.Success = false
				record.Error = e.Error()
			}
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
