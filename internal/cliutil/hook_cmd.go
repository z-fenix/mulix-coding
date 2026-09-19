package cliutil

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mulix-dev/mulix-coding/internal/hook"
	"github.com/mulix-dev/mulix-coding/internal/state"
)

// newHookCmd wires the Claude Code PreToolUse hook entry point: read the
// request JSON from stdin, decide, write the response JSON to stdout, and
// exit with the code Claude Code's hook contract expects (0 = allow,
// 2 = block). Any internal error (no root found, no active change, a
// corrupt state file) fails OPEN rather than crashing the tool call in a
// confusing way: it allows the write through and prints a warning to
// stderr, because a broken hook must never become a way to silently brick
// every Write/Edit call in the project.
func newHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "hook",
		Short:  "Claude Code PreToolUse hook entry point (reads JSON from stdin)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			req, err := hook.ParseRequest(os.Stdin)
			if err != nil {
				return failOpen(fmt.Errorf("hook: %w", err))
			}

			root, err := state.FindRoot(".")
			if err != nil {
				return failOpen(err)
			}
			change, err := state.Active(root)
			if err != nil {
				return failOpen(err)
			}
			s, err := state.Load(root, change)
			if err != nil {
				return failOpen(err)
			}

			decision := hook.Decide(root, s, req)
			out, err := hook.Render(decision)
			if err != nil {
				return failOpen(err)
			}
			fmt.Fprintln(os.Stdout, string(out))
			os.Exit(hook.ExitCode(decision))
			return nil
		},
	}
}

// failOpen reports why the hook could not evaluate its policy and then
// allows the tool call through, so a mulix installation/config problem
// degrades to "no enforcement" rather than "every write is blocked".
func failOpen(cause error) error {
	fmt.Fprintf(os.Stderr, "mulix hook: %v (allowing the call through)\n", cause)
	out, _ := hook.Render(hook.Decision{Allow: true})
	fmt.Fprintln(os.Stdout, string(out))
	os.Exit(0)
	return nil
}
