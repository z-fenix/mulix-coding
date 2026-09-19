package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mulix-dev/mulix-coding/internal/flow"
	"github.com/mulix-dev/mulix-coding/internal/guard"
	"github.com/mulix-dev/mulix-coding/internal/state"
)

func newGuardCmd() *cobra.Command {
	var change string
	cmd := &cobra.Command{
		Use:   "guard <event>",
		Short: "Read-only: run the guard checks for an event without transitioning state",
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
			if !report.Passed() {
				return fmt.Errorf("guard checks failed for event %q", event)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&change, "change", "", "change id (defaults to the active change)")
	return cmd
}
