package cliutil

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

func sessionIdentity(cmd *cobra.Command, lookupEnv func(string) (string, bool)) (sessionctx.Identity, error) {
	id := sessionctx.Identity{}
	flag := cmd.Flags().Lookup("session-id")
	if flag == nil {
		flag = cmd.Root().PersistentFlags().Lookup("session-id")
	}
	if flag != nil && flag.Changed {
		id = sessionctx.Identity{Value: flag.Value.String(), Present: true}
	} else {
		id.Value, id.Present = lookupEnv("MULIX_SESSION_ID")
	}
	return id, sessionctx.ValidateIdentity(id)
}

func resolveCommandContext(cmd *cobra.Command, explicit string, access sessionctx.Access) (*sessionctx.Store, sessionctx.Resolved, error) {
	s, err := sessionctx.Open(".")
	if err != nil {
		return nil, sessionctx.Resolved{}, err
	}
	id, err := sessionIdentity(cmd, os.LookupEnv)
	if err != nil {
		return s, sessionctx.Resolved{}, err
	}
	r, err := s.Resolve(cmd.Context(), sessionctx.ResolveRequest{Identity: id, ExplicitChange: explicit, Access: access})
	return s, r, err
}

func requireChange(r sessionctx.Resolved) error {
	if r.State == nil {
		return fmt.Errorf("no active change set (use `mulix new` or `mulix state select`)")
	}
	return nil
}
func printContextSummary(r sessionctx.Resolved) {
	fmt.Printf("mode:          %s\nsession:       %s\nsource:        %s\n", r.Mode, r.SessionID, r.Source)
}
func printSelection(s sessionctx.Selection) {
	fmt.Printf("selected change %q (scope=%s mode=%s session=%s)\n", s.Change, s.Scope, s.Mode, s.SessionID)
	if s.Scope == "global" && s.Mode == sessionctx.Session && s.SessionID != "" {
		fmt.Println("existing session bindings are unchanged; unbound sessions follow this global active")
	}
}
