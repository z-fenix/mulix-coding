// Package services persists the task list as a single JSON file.
package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"example.com/todo-cli/src/models"
)

// Load reads the task store at path, returning an empty list when the
// file does not exist yet.
func Load(path string) ([]models.Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("services: reading %s: %w", path, err)
	}
	var tasks []models.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("services: parsing %s: %w", path, err)
	}
	return tasks, nil
}

// Save writes the task list to path as pretty-printed JSON.
func Save(path string, tasks []models.Task) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("services: creating dir for %s: %w", path, err)
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("services: encoding tasks: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("services: writing %s: %w", path, err)
	}
	return nil
}
