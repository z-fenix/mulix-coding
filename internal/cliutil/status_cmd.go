package cliutil

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "List changes and distinguish effective and global selections", RunE: func(cmd *cobra.Command, args []string) error {
		store, r, contextErr := resolveCommandContext(cmd, "", sessionctx.Inspect)
		if store == nil {
			return contextErr
		}
		if contextErr != nil {
			fmt.Printf("context error: %v\n", contextErr)
		} else {
			printContextSummary(r)
		}
		// A valid bound session must not hide a broken global fallback. Resolve
		// without identity so validation and lock ordering match other callers.
		globalContext, globalErr := store.Resolve(cmd.Context(), sessionctx.ResolveRequest{Access: sessionctx.Inspect})
		global := globalContext.Change
		if globalErr != nil {
			fmt.Printf("global context error: %v\n", globalErr)
		}
		changes, err := state.List(store.Root())
		if err != nil {
			return errors.Join(contextErr, globalErr, err)
		}
		if len(changes) == 0 {
			fmt.Println("no changes yet (use `mulix new <title>` to create one)")
		}
		resultErr := errors.Join(contextErr, globalErr)
		for _, id := range changes {
			s, err := state.Load(store.Root(), id)
			if err != nil {
				fmt.Printf("  %-30s <error: %v>\n", id, err)
				resultErr = errors.Join(resultErr, err)
				continue
			}
			marker := "  "
			if contextErr == nil && id == r.Change {
				marker = "* "
			}
			note := ""
			if id == global {
				note = " global-active"
			}
			fmt.Printf("%s%-30s phase=%-12s verify_failures=%d%s\n", marker, id, s.Phase, s.VerifyFailures, note)
		}
		return resultErr
	}}
}
