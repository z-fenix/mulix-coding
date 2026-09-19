package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mulix-dev/mulix-coding/internal/flow"
	"github.com/mulix-dev/mulix-coding/internal/scaffold"
	"github.com/mulix-dev/mulix-coding/internal/state"
)

func newNewCmd() *cobra.Command {
	var branch string

	cmd := &cobra.Command{
		Use:   "new <title>",
		Short: "Create a new change: an artifact directory plus a fresh state in the specify phase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}

			nc, err := scaffold.CreateChangeDir(root, args[0])
			if err != nil {
				return err
			}

			s := flow.New(nc.Change, branch)
			if err := state.Save(root, s); err != nil {
				return err
			}
			if err := state.SetActive(root, nc.Change); err != nil {
				return err
			}

			fmt.Printf("created change %q: spec at %s, artifacts at %s (phase: %s)\n", nc.Change, nc.SpecDir, nc.ChangeDir, s.Phase)
			printNextEvents(s.Phase)
			return nil
		},
	}
	cmd.Flags().StringVar(&branch, "branch", "", "git branch name to record in state (mulix does not create the branch itself)")
	return cmd
}
