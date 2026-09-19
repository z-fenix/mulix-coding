package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mulix-dev/mulix-coding/internal/state"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "List every change and its current phase, marking the active one",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := state.FindRoot(".")
			if err != nil {
				return err
			}
			changes, err := state.List(root)
			if err != nil {
				return err
			}
			if len(changes) == 0 {
				fmt.Println("no changes yet (use `mulix new <title>` to create one)")
				return nil
			}
			active, _ := state.Active(root) // best-effort; "" if unset

			for _, id := range changes {
				s, err := state.Load(root, id)
				if err != nil {
					fmt.Printf("  %-30s <error: %v>\n", id, err)
					continue
				}
				marker := "  "
				if id == active {
					marker = "* "
				}
				fmt.Printf("%s%-30s phase=%-12s verify_failures=%d\n", marker, id, s.Phase, s.VerifyFailures)
			}
			return nil
		},
	}
}
