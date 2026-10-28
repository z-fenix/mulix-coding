// Package cliutil wires the mulix Cobra command tree and holds small
// helpers shared by subcommands (resolving the project root, loading the
// active change, printing guard reports consistently).
package cliutil

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/guard"
)

// Version is set by the build (see cmd/mulix/main.go); kept here so the
// version command doesn't need its own flag/global wiring.
var Version = "dev"

// NewRootCmd builds the full mulix command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "mulix",
		Short:         "mulix-coding: spec-driven development with phase gates for Claude Code and DeepSeek Harness",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().String("session-id", "", "session identity (otherwise MULIX_SESSION_ID, then global context)")
	root.AddCommand(
		newContextCmd(),
		newInitCmd(),
		newUpdateCmd(),
		newNewCmd(),
		newStateCmd(),
		newGuardCmd(),
		newHookCmd(),
		newStatusCmd(),
		newPresetCmd(),
		newVersionCmd(),
	)
	return root
}

// printGuardReport renders a guard.Report the same way everywhere: a
// summary line, then one line per failed check with its remediation hint.
func printGuardReport(report guard.Report) {
	if report.Passed() {
		fmt.Printf("guard: all checks passed for event %q\n", report.Event)
		return
	}
	fmt.Printf("guard: %d/%d checks failed for event %q\n", len(report.Failures()), len(report.Results), report.Event)
	for _, f := range report.Failures() {
		fmt.Printf("  - %s: %s\n", f.Name, f.Next)
	}
}

// printNextEvents prints the events legal from the current phase, to tell
// the user (or agent) what they can do next.
func printNextEvents(phase flow.Phase) {
	events := flow.NextEvents(phase)
	if len(events) == 0 {
		fmt.Printf("no outbound events from phase %q\n", phase)
		return
	}
	fmt.Printf("next possible events from phase %q:\n", phase)
	for _, e := range events {
		fmt.Printf("  - %s\n", e)
	}
}
