package cli

import (
	"fmt"
	"os"

	"example.com/todo-cli/src/services"
)

// defaultStorePath is where the task store lives when no override is
// given. Kept relative so the example works from any directory it's run
// in without extra setup.
const defaultStorePath = "tasks.json"

// Run dispatches argv[0] to the matching subcommand. It's the single
// entry point cmd/todo's main() calls, kept in this package so tests can
// exercise the same dispatch logic if needed without spawning a process.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: todo <add|list|set-priority> ...")
		return 1
	}
	store := services.NewStore(defaultStorePath)

	switch args[0] {
	case "add":
		return runAdd(store, args[1:])
	case "list":
		return runList(store, args[1:])
	case "set-priority":
		return runSetPriority(store, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		return 1
	}
}

func runAdd(store *services.Store, args []string) int {
	description, priority, err := parseAddArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage: todo add <description> [--priority low|medium|high]")
		return 1
	}

	if err := Add(store, description, priority); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

// parseAddArgs splits args into the task description and an optional
// --priority value. It's hand-rolled rather than using the flag package
// because flag.Parse stops consuming at the first non-flag argument, and
// todo add's usage puts the description (a non-flag positional arg)
// before --priority, e.g. `todo add "buy milk" --priority high` — a
// stdlib FlagSet would leave --priority unparsed in that order.
func parseAddArgs(args []string) (description, priority string, err error) {
	var positional []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--priority" {
			i++
			if i >= len(args) {
				return "", "", fmt.Errorf("--priority requires a value")
			}
			priority = args[i]
			continue
		}
		positional = append(positional, args[i])
	}
	if len(positional) < 1 {
		return "", "", fmt.Errorf("missing task description")
	}
	return positional[0], priority, nil
}

func runList(store *services.Store, _ []string) int {
	tasks, err := List(store)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	for _, task := range tasks {
		fmt.Printf("[%s] %s (%s)\n", task.ID, task.Description, task.PriorityOrDefault())
	}
	return 0
}

func runSetPriority(store *services.Store, args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: todo set-priority <id> <low|medium|high>")
		return 1
	}
	if err := SetPriority(store, args[0], args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}
