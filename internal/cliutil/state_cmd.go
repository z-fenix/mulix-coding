package cliutil

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/guard"
	"github.com/z-fenix/mulix-coding/internal/layout"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func newStateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "state", Short: "Inspect and transition a change's phase state"}
	cmd.AddCommand(newStateShowCmd(), newStateTransitionCmd(), newStateSelectCmd(), newStateSetFieldCmd())
	return cmd
}

func newStateShowCmd() *cobra.Command {
	var change string
	cmd := &cobra.Command{Use: "show", Short: "Print state for the effective or explicitly requested change", RunE: func(cmd *cobra.Command, args []string) error {
		_, r, err := resolveCommandContext(cmd, change, sessionctx.Inspect)
		if err != nil {
			return err
		}
		if err := requireChange(r); err != nil {
			return err
		}
		s := *r.State
		printContextSummary(r)
		fmt.Printf("change:        %s\n", s.Change)
		fmt.Printf("phase:         %s\n", s.Phase)
		fmt.Printf("branch:        %s\n", s.Branch)
		fmt.Printf("runtime_dir:   %s\n", layout.ChangeRuntimeDir(s.Change))
		fmt.Printf("spec_path:     %s\n", s.SpecPath)
		fmt.Printf("tasks_path:    %s\n", s.TasksPath)
		fmt.Printf("design:        track=%s approved=%v path=%s\n", s.DesignTrack, s.DesignApproved, s.DesignPath)
		fmt.Printf("execution:     %s\n", s.ExecutionMethod)
		fmt.Printf("report_path:   %s\n", s.ReportPath)
		fmt.Printf("verify_result: %s (failures: %d)\n", s.VerifyResult, s.VerifyFailures)
		fmt.Printf("archived:      %v\n", s.Archived)
		printNextEvents(s.Phase)
		return nil
	}}
	cmd.Flags().StringVar(&change, "change", "", "change id (defaults to effective context)")
	return cmd
}

func writableState(r sessionctx.Resolved, current flow.State) error {
	if r.Mode == sessionctx.Session && r.Source != sessionctx.ExplicitChange && current.Archived {
		return fmt.Errorf("change %q is archived; select another change or unbind", current.Change)
	}
	return nil
}

func newStateTransitionCmd() *cobra.Command {
	var change string
	var force bool
	cmd := &cobra.Command{Use: "transition <event>", Short: "Check guards and advance state under the change lock", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, r, err := resolveCommandContext(cmd, change, sessionctx.Mutate)
		if err != nil {
			return err
		}
		if err := requireChange(r); err != nil {
			return err
		}
		from := r.Phase
		result, err := state.Update(cmd.Context(), store.Root(), r.Change, func(s *flow.State) error {
			if err := writableState(r, *s); err != nil {
				return err
			}
			from = s.Phase
			event := flow.Event(args[0])
			if _, ok := flow.Find(s.Phase, event); !ok {
				return fmt.Errorf("event %q is not legal from phase %q", event, s.Phase)
			}
			report := guard.Run(store.Root(), *s, event)
			printGuardReport(report)
			if !report.Passed() && !force {
				return fmt.Errorf("guard checks failed for event %q (use --force to override; not recommended)", event)
			}
			next, err := flow.Apply(*s, event)
			if err == nil {
				*s = next
			}
			return err
		}, func(next flow.State) error {
			if next.Phase != flow.PhaseArchive || !next.Archived {
				return nil
			}
			cleared, err := store.ClearActiveIf(cmd.Context(), next.Change)
			if cleared {
				fmt.Printf("cleared active change (was %q; now archived)\n", next.Change)
			}
			return err
		})
		if result.Saved {
			printContextSummary(r)
			fmt.Printf("transitioned %q: %s -> %s\n", r.Change, from, result.State.Phase)
			printNextEvents(result.State.Phase)
		}
		if err != nil && result.Saved {
			return fmt.Errorf("state saved; post-save cleanup failed: %w", err)
		}
		return err
	}}
	cmd.Flags().StringVar(&change, "change", "", "change id (must match a session's effective context)")
	cmd.Flags().BoolVar(&force, "force", false, "skip readiness guards for a legal transition (recovery only)")
	return cmd
}

func newStateSelectCmd() *cobra.Command {
	var global bool
	cmd := &cobra.Command{Use: "select <change>", Short: "Select for this session, or global active with --global", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, err := sessionctx.Open(".")
		if err != nil {
			return err
		}
		id, err := sessionIdentity(cmd, os.LookupEnv)
		if err != nil {
			return err
		}
		target, err := store.PrepareSelection(cmd.Context(), id, global)
		if err != nil {
			return err
		}
		selected, err := store.Select(cmd.Context(), target, args[0])
		if err != nil {
			return err
		}
		printSelection(selected)
		return nil
	}}
	cmd.Flags().BoolVar(&global, "global", false, "update global active without changing session bindings")
	return cmd
}

func newStateSetFieldCmd() *cobra.Command {
	var change string
	cmd := &cobra.Command{
		Use: "set <field> <value>", Short: "Set an allowed artifact-path or flag field without changing phase",
		Long: "Set spec_path, tasks_path, report_path, clarify_skipped, design_track, design_path, design_approved, execution_method, verify_result or archive_confirmation. Use transition to advance phases.",
		Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
			store, r, err := resolveCommandContext(cmd, change, sessionctx.Mutate)
			if err != nil {
				return err
			}
			if err := requireChange(r); err != nil {
				return err
			}
			_, err = state.Update(cmd.Context(), store.Root(), r.Change, func(s *flow.State) error {
				if err := writableState(r, *s); err != nil {
					return err
				}
				return setField(s, args[0], args[1])
			}, nil)
			if err != nil {
				return err
			}
			printContextSummary(r)
			fmt.Printf("set %s=%s on %q\n", args[0], args[1], r.Change)
			return nil
		},
	}
	cmd.Flags().StringVar(&change, "change", "", "change id (must match a session's effective context)")
	return cmd
}

func setField(s *flow.State, field, value string) error {
	switch field {
	case "spec_path":
		s.SpecPath = value
	case "tasks_path":
		s.TasksPath = value
	case "report_path":
		s.ReportPath = value
	case "clarify_skipped":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		s.ClarifySkipped = b
	case "design_track":
		if !flow.DesignTrack(value).Valid() {
			return fmt.Errorf("design_track must be one of %v, got %q", flow.DesignTracks, value)
		}
		s.DesignTrack = flow.DesignTrack(value)
	case "design_path":
		s.DesignPath = value
	case "design_approved":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		s.DesignApproved = b
	case "execution_method":
		if !flow.ExecutionMethod(value).Valid() {
			return fmt.Errorf("execution_method must be one of %v, got %q", flow.ExecutionMethods, value)
		}
		s.ExecutionMethod = flow.ExecutionMethod(value)
	case "verify_result":
		switch value {
		case string(flow.VerifyPending), string(flow.VerifyPass), string(flow.VerifyFail):
			s.VerifyResult = flow.VerifyResult(value)
		default:
			return fmt.Errorf("verify_result must be one of pending|pass|fail, got %q", value)
		}
	case "archive_confirmation":
		switch value {
		case string(flow.ArchivePending), string(flow.ArchiveConfirmed):
			s.ArchiveConfirmation = flow.ArchiveConfirmation(value)
		default:
			return fmt.Errorf("archive_confirmation must be one of pending|confirmed, got %q", value)
		}
	default:
		return fmt.Errorf("unknown or unsettable field %q", field)
	}
	return nil
}

func parseBool(value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("expected true|false, got %q", value)
	}
}
