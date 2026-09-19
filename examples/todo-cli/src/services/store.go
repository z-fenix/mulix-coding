// Package services implements the on-disk task store: a single JSON file
// holding one array of tasks.
package services

import (
	"encoding/json"
	"fmt"
	"os"

	"example.com/todo-cli/src/models"
)

// Store persists tasks to a single JSON file on disk.
type Store struct {
	Path string
}

// NewStore returns a Store backed by the JSON file at path.
func NewStore(path string) *Store {
	return &Store{Path: path}
}

// Load reads all tasks from disk. A missing file is treated as an empty
// task list rather than an error, since a brand new todo-cli project has
// no store file yet.
func (s *Store) Load() ([]models.Task, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("store: reading %s: %w", s.Path, err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var tasks []models.Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("store: parsing %s: %w", s.Path, err)
	}
	return tasks, nil
}

// Save writes tasks to disk, overwriting whatever was there before.
func (s *Store) Save(tasks []models.Task) error {
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("store: encoding tasks: %w", err)
	}
	if err := os.WriteFile(s.Path, data, 0o644); err != nil {
		return fmt.Errorf("store: writing %s: %w", s.Path, err)
	}
	return nil
}
