// Package cli implements todo-cli's subcommands.
package cli

import (
	"fmt"
	"slices"
	"time"

	"example.com/todo-cli/src/models"
	"example.com/todo-cli/src/services"
)

// Add creates a new task with description and priority, and saves it to
// store. An empty priority defaults to models.DefaultPriority. A
// non-empty priority outside models.ValidPriorities is rejected before
// anything is written.
func Add(store *services.Store, description, priority string) error {
	if priority != "" && !slices.Contains(models.ValidPriorities, priority) {
		return fmt.Errorf("invalid priority %q: must be one of %v", priority, models.ValidPriorities)
	}

	tasks, err := store.Load()
	if err != nil {
		return err
	}

	task := models.Task{
		ID:          fmt.Sprintf("%d", len(tasks)+1),
		Description: description,
		CreatedAt:   time.Now(),
		Priority:    priority,
	}
	tasks = append(tasks, task)

	return store.Save(tasks)
}
