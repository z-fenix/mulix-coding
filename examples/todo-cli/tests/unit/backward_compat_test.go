package unit

import (
	"os"
	"path/filepath"
	"testing"

	"example.com/todo-cli/src/services"
)

// TestLoad_TaskWithoutPriorityFieldDefaultsToMedium covers FR-009:
// tasks written before the priority feature existed have no "priority"
// key at all in their stored JSON, not just an empty string. Loading one
// must not error, and PriorityOrDefault() on the result must be
// "medium".
func TestLoad_TaskWithoutPriorityFieldDefaultsToMedium(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")

	// Deliberately no "priority" key, simulating data written by a
	// version of todo-cli that predates this feature.
	legacyJSON := `[
		{"id": "1", "description": "old task", "done": false, "created_at": "2024-01-01T00:00:00Z"}
	]`
	if err := os.WriteFile(path, []byte(legacyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	store := services.NewStore(path)
	tasks, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if got := tasks[0].PriorityOrDefault(); got != "medium" {
		t.Fatalf("PriorityOrDefault() = %q, want %q", got, "medium")
	}
}
