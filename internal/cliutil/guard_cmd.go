package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/guard"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

func newGuardCmd() *cobra.Command {
	var change string
	cmd := &cobra.Command{Use: "guard <event>", Short: "Read-only guard checks without transitioning state", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, r, err := resolveCommandContext(cmd, change, sessionctx.Inspect)
		if err != nil {
			return err
		}
		if err := requireChange(r); err != nil {
			return err
		}
		s := *r.State
		event := flow.Event(args[0])
		if _, ok := flow.Find(s.Phase, event); !ok {
			return fmt.Errorf("event %q is not legal from phase %q", event, s.Phase)
		}
		report := guard.Run(store.Root(), s, event)
		printContextSummary(r)
		printGuardReport(report)
		if !report.Passed() {
			return fmt.Errorf("guard checks failed for event %q", event)
		}
		return nil
	}}
	cmd.Flags().StringVar(&change, "change", "", "change id (defaults to effective context)")
	return cmd
}
