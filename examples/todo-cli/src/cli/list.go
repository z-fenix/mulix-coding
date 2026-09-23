package cli

import (
	"fmt"
	"os"

	"example.com/todo-cli/src/models"
	"example.com/todo-cli/src/services"
)

// runList implements `todo list`: tasks ordered high → medium → low,
// same-priority tasks oldest first (the sort is stable).
func runList() int {
	tasks, err := services.Load(storePath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "todo: %v\n", err)
		return 1
	}
	models.SortByPriority(tasks)
	for _, t := range tasks {
		marker := " "
		if t.Done {
			marker = "x"
		}
		fmt.Printf("[%s] %d. %s (%s)\n", marker, t.ID, t.Description, t.PriorityOrDefault())
	}
	return 0
}
