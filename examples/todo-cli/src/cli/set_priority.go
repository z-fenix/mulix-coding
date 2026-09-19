package cli

import (
	"fmt"
	"slices"

	"example.com/todo-cli/src/models"
	"example.com/todo-cli/src/services"
)

// SetPriority updates the priority of the task with the given id.
// Returns an error and leaves the store unmodified if id doesn't match
// any task, or if priority isn't one of models.ValidPriorities.
func SetPriority(store *services.Store, id, priority string) error {
	if !slices.Contains(models.ValidPriorities, priority) {
		return fmt.Errorf("invalid priority %q: must be one of %v", priority, models.ValidPriorities)
	}

	tasks, err := store.Load()
	if err != nil {
		return err
	}

	for i, task := range tasks {
		if task.ID == id {
			tasks[i].Priority = priority
			return store.Save(tasks)
		}
	}
	return fmt.Errorf("task not found: %q", id)
}
