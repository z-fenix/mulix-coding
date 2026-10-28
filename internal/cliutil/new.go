package cliutil

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/lockfile"
	"github.com/z-fenix/mulix-coding/internal/scaffold"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func newNewCmd() *cobra.Command {
	var branch string
	var global bool
	cmd := &cobra.Command{
		Use:   "new <title>",
		Short: "Create a dated change and select it for the session or global context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
			nc, err := scaffold.CreateChangeDir(store.Root(), args[0])
			if err != nil {
				return err
			}
			s := flow.New(nc.Change, branch)
			if err := lockfile.With(cmd.Context(), store.Root(), lockfile.ChangeKey(nc.Change), func() error { return state.Save(store.Root(), s) }); err != nil {
				return fmt.Errorf("allocated change %q; saving initial state failed: %w", nc.Change, err)
			}
			fmt.Printf("created change %q: spec at %s, artifacts at %s (phase: %s)\n", nc.Change, nc.SpecDir, nc.ChangeDir, s.Phase)
			selected, err := store.Select(cmd.Context(), target, nc.Change)
			if err != nil {
				return fmt.Errorf("created change %q but context selection failed: %w", nc.Change, err)
			}
			printSelection(selected)
			printNextEvents(s.Phase)
			return nil
		},
	}
	cmd.Flags().StringVar(&branch, "branch", "", "git branch name to record (mulix does not create the branch)")
	cmd.Flags().BoolVar(&global, "global", false, "update global active without changing session bindings")
	return cmd
}
