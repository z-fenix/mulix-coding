package cli

import (
	"fmt"
	"os"
	"strconv"

	"example.com/todo-cli/src/models"
	"example.com/todo-cli/src/services"
)

// runSetPriority implements `todo set-priority <id> <level>`: updates an
// existing task's priority, exiting non-zero with "task not found" for
// an unknown id and leaving the store unmodified in that case.
func runSetPriority(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: todo set-priority <id> <low|medium|high>")
		return 2
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "todo: invalid task id %q\n", args[0])
		return 2
	}
	level := args[1]
	if !validPriorities[level] {
		fmt.Fprintf(os.Stderr, "todo: invalid priority %q (want %s, %s or %s)\n", level, models.PriorityLow, models.PriorityMedium, models.PriorityHigh)
		return 2
	}

	path := storePath()
	tasks, err := services.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "todo: %v\n", err)
		return 1
	}
	for i, t := range tasks {
		if t.ID == id {
			tasks[i].Priority = level
			if err := services.Save(path, tasks); err != nil {
				fmt.Fprintf(os.Stderr, "todo: %v\n", err)
				return 1
			}
			fmt.Printf("task %d priority set to %s\n", id, level)
			return 0
		}
	}
	fmt.Fprintf(os.Stderr, "todo: task not found: %d\n", id)
	return 1
}
