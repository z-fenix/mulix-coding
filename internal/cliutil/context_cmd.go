package cliutil

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "context", Short: "Inspect or manage local session context"}
	var jsonOutput bool
	var field string
	show := &cobra.Command{Use: "show", Args: cobra.NoArgs, Short: "Show effective context and its source", RunE: func(cmd *cobra.Command, args []string) error {
		if jsonOutput && field != "" {
			return fmt.Errorf("--json and --field are mutually exclusive")
		}
		if field != "" && field != "runtime-dir" {
			return fmt.Errorf("unsupported context field %q", field)
		}
		_, r, err := resolveCommandContext(cmd, "", sessionctx.Inspect)
		if err != nil {
			return err
		}
		if field == "runtime-dir" {
			fmt.Println(r.RuntimeDir)
			return nil
		}
		if jsonOutput {
			data, err := json.Marshal(r)
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}
		printContextSummary(r)
		fmt.Printf("change:        %s\nphase:         %s\nruntime_dir:   %s\n", r.Change, r.Phase, r.RuntimeDir)
		return nil
	}}
	show.Flags().BoolVar(&jsonOutput, "json", false, "print stable JSON")
	show.Flags().StringVar(&field, "field", "", "print one field (runtime-dir)")
	mode := &cobra.Command{Use: "mode <legacy|session>", Args: cobra.ExactArgs(1), Short: "Set project context mode without changing bindings", RunE: func(cmd *cobra.Command, args []string) error {
		s, err := sessionctx.Open(".")
		if err != nil {
			return err
		}
		if err := s.SetMode(cmd.Context(), sessionctx.Mode(args[0])); err != nil {
			return err
		}
		fmt.Printf("context mode: %s (active and session bindings unchanged)\n", args[0])
		return nil
	}}
	unbind := &cobra.Command{Use: "unbind", Args: cobra.NoArgs, Short: "Remove this session binding and resume global fallback", RunE: func(cmd *cobra.Command, args []string) error {
		s, err := sessionctx.Open(".")
		if err != nil {
			return err
		}
		id, err := sessionIdentity(cmd, os.LookupEnv)
		if err != nil {
			return err
		}
		removed, err := s.Unbind(cmd.Context(), id)
		if err != nil {
			return err
		}
		fmt.Printf("session binding removed=%v (session=%s)\n", removed, id.Value)
		r, err := s.Resolve(cmd.Context(), sessionctx.ResolveRequest{Identity: id})
		if err != nil {
			return fmt.Errorf("unbind finished; fallback context invalid: %w", err)
		}
		printContextSummary(r)
		fmt.Printf("change:        %s\n", r.Change)
		return nil
	}}
	cmd.AddCommand(show, mode, unbind)
	return cmd
}
