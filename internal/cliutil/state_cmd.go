package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mulix-dev/mulix-coding/internal/flow"
	"github.com/mulix-dev/mulix-coding/internal/guard"
	"github.com/mulix-dev/mulix-coding/internal/state"
)

func newStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Inspect and transition a change's phase state",
	}
	cmd.AddCommand(
		newStateShowCmd(),
		newStateTransitionCmd(),
		newStateSelectCmd(),
		newStateSetFieldCmd(),
	)
	return cmd
}

func newStateShowCmd() *cobra.Command {
	var change string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the current state for a change (defaults to the active change)",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			id, err := resolveChange(root, change)
			if err != nil {
				return err
			}
			s, err := state.Load(root, id)
			if err != nil {
				return err
			}
			fmt.Printf("change:        %s\n", s.Change)
			fmt.Printf("phase:         %s\n", s.Phase)
			fmt.Printf("branch:        %s\n", s.Branch)
			fmt.Printf("spec_path:     %s\n", s.SpecPath)
			fmt.Printf("plan_path:     %s\n", s.PlanPath)
			fmt.Printf("tasks_path:    %s\n", s.TasksPath)
			fmt.Printf("report_path:   %s\n", s.ReportPath)
			fmt.Printf("verify_result: %s (failures: %d)\n", s.VerifyResult, s.VerifyFailures)
			fmt.Printf("archived:      %v\n", s.Archived)
			printNextEvents(s.Phase)
			return nil
		},
	}
	cmd.Flags().StringVar(&change, "change", "", "change id (defaults to the active change)")
	return cmd
}

func newStateTransitionCmd() *cobra.Command {
	var change string
	var force bool
	cmd := &cobra.Command{
		Use:   "transition <event>",
		Short: "Run guard checks for an event and, if they all pass, advance the change's phase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			id, err := resolveChange(root, change)
			if err != nil {
				return err
			}
			s, err := state.Load(root, id)
			if err != nil {
				return err
			}

			event := flow.Event(args[0])
			if _, ok := flow.Find(s.Phase, event); !ok {
				return fmt.Errorf("event %q is not legal from phase %q", event, s.Phase)
			}

			report := guard.Run(root, s, event)
			printGuardReport(report)
			if !report.Passed() && !force {
				return fmt.Errorf("guard checks failed for event %q (use --force to override; not recommended)", event)
			}

			next, err := flow.Apply(s, event)
			if err != nil {
				return err
			}
			if err := state.Save(root, next); err != nil {
				return err
			}
			if next.Phase == flow.PhaseArchive && next.Archived && state.ActiveIs(root, id) {
				if err := state.ClearActive(root); err != nil {
					return err
				}
				fmt.Printf("cleared active change (was %q; now archived)\n", id)
			}
			fmt.Printf("transitioned %q: %s -> %s\n", id, s.Phase, next.Phase)
			printNextEvents(next.Phase)
			return nil
		},
	}
	cmd.Flags().StringVar(&change, "change", "", "change id (defaults to the active change)")
	cmd.Flags().BoolVar(&force, "force", false, "apply the transition even if guard checks fail (bypasses strong flow control; use only when you know why)")
	return cmd
}

func newStateSelectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "select <change>",
		Short: "Set the active change for commands that omit --change",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			if !state.Exists(root, args[0]) {
				return fmt.Errorf("no state found for change %q", args[0])
			}
			if err := state.SetActive(root, args[0]); err != nil {
				return err
			}
			fmt.Printf("active change set to %q\n", args[0])
			return nil
		},
	}
	return cmd
}

func newStateSetFieldCmd() *cobra.Command {
	var change string
	cmd := &cobra.Command{
		Use:   "set <field> <value>",
		Short: "Set one artifact-path or flag field on a change's state (does not change phase)",
		Long: "Set one of a small allow-list of fields on a change's state: spec_path, plan_path, " +
			"tasks_path, analyze_path, report_path, clarify_skipped, analyze_skipped, " +
			"verify_result, archive_confirmation. This never advances the phase; use `mulix state transition` for that.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			id, err := resolveChange(root, change)
			if err != nil {
				return err
			}
			s, err := state.Load(root, id)
			if err != nil {
				return err
			}
			if err := setField(&s, args[0], args[1]); err != nil {
				return err
			}
			if err := state.Save(root, s); err != nil {
				return err
			}
			fmt.Printf("set %s=%s on %q\n", args[0], args[1], id)
			return nil
		},
	}
	cmd.Flags().StringVar(&change, "change", "", "change id (defaults to the active change)")
	return cmd
}

func setField(s *flow.State, field, value string) error {
	switch field {
	case "spec_path":
		s.SpecPath = value
	case "plan_path":
		s.PlanPath = value
	case "tasks_path":
		s.TasksPath = value
	case "analyze_path":
		s.AnalyzePath = value
	case "report_path":
		s.ReportPath = value
	case "clarify_skipped":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		s.ClarifySkipped = b
	case "analyze_skipped":
		b, err := parseBool(value)
		if err != nil {
			return err
		}
		s.AnalyzeSkipped = b
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
