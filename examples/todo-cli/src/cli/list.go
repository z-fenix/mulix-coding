package cli

import (
	"example.com/todo-cli/src/models"
	"example.com/todo-cli/src/services"
)

// List returns every task from store, ordered high -> medium -> low
// priority, ties broken by creation order (oldest first).
func List(store *services.Store) ([]models.Task, error) {
	tasks, err := store.Load()
	if err != nil {
		return nil, err
	}
	models.SortByPriority(tasks)
	return tasks, nil
}
