package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"example.com/todo-cli/src/models"
	"example.com/todo-cli/src/services"
)

// validPriorities is the set of accepted --priority values, in
// display order.
var validPriorities = map[string]bool{
	models.PriorityLow:    true,
	models.PriorityMedium: true,
	models.PriorityHigh:   true,
}

// parseAddArgs extracts the task description and the --priority value
// (when given) from add's arguments. The flag may appear anywhere among
// the arguments — before, between, or after the description — so this
// scans rather than using flag.FlagSet, which stops parsing at the
// first non-flag token and would silently drop a trailing --priority.
func parseAddArgs(args []string) (description, priority string, err error) {
	priority = models.PriorityMedium
	for i := 0; i < len(args); i++ {
		if args[i] == "--priority" {
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--priority requires a value (%s, %s or %s)", models.PriorityLow, models.PriorityMedium, models.PriorityHigh)
			}
			i++
			priority = args[i]
			continue
		}
		if description == "" {
			description = args[i]
		} else {
			return "", "", fmt.Errorf("unexpected extra argument %q", args[i])
		}
	}
	if description == "" {
		return "", "", fmt.Errorf("add requires a task description")
	}
	if !validPriorities[priority] {
		return "", "", fmt.Errorf("invalid --priority %q (want %s, %s or %s)", priority, models.PriorityLow, models.PriorityMedium, models.PriorityHigh)
	}
	return description, priority, nil
}

// runAdd implements `todo add <description> [--priority low|medium|high]`.
func runAdd(args []string) int {
	description, priority, err := parseAddArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "todo: %v\n", err)
		return 2
	}

	path := storePath()
	tasks, err := services.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "todo: %v\n", err)
		return 1
	}
	task := models.Task{
		ID:          models.NextID(tasks),
		Description: description,
		CreatedAt:   time.Now(),
		Priority:    priority,
	}
	tasks = append(tasks, task)
	if err := services.Save(path, tasks); err != nil {
		fmt.Fprintf(os.Stderr, "todo: %v\n", err)
		return 1
	}
	fmt.Printf("added task %d (%s): %s\n", task.ID, strings.TrimSpace(task.Priority), task.Description)
	return 0
}
