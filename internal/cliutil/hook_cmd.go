package cliutil

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/z-fenix/mulix-coding/internal/hook"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
)

// The default entry remains mulix hook, as installed by previous releases.
func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "hook", Short: "Claude Code hook entry point (JSON stdin)", Hidden: true, RunE: func(cmd *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		payload, readErr := io.ReadAll(os.Stdin)
		outcome := evaluatePreToolUse(cmd.Context(), cwd, payload)
		if readErr != nil {
			outcome = malformedHook(cmd.Context(), cwd, readErr)
		}
		if outcome.Cause != nil {
			fmt.Fprintf(os.Stderr, "mulix hook: %v (allow=%v)\n", outcome.Cause, outcome.Decision.Allow)
		}
		strict := outcome.Resolved.Mode == sessionctx.Session || (outcome.Resolved.Root != "" && outcome.Resolved.Mode == "")
		os.Exit(emitHook(os.Stdout, outcome.Decision, strict, hook.Render))
		return nil
	}}
	cmd.AddCommand(newSessionStartCmd())
	return cmd
}
