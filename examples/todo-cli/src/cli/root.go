// Package cli implements the todo command-line interface.
package cli

import (
	"fmt"
	"os"
)

// storePath is where the task list lives, overridable for tests via the
// TODO_FILE environment variable.
func storePath() string {
	if p := os.Getenv("TODO_FILE"); p != "" {
		return p
	}
	return "tasks.json"
}

// Run executes one todo command and returns the process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	switch args[0] {
	case "add":
		return runAdd(args[1:])
	case "list":
		return runList()
	case "set-priority":
		return runSetPriority(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "todo: unknown command %q\n", args[0])
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: todo add <description> [--priority low|medium|high] | todo list | todo set-priority <id> <low|medium|high>")
}
