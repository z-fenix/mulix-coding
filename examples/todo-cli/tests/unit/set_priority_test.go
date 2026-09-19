package unit

import (
	"path/filepath"
	"testing"

	"example.com/todo-cli/src/cli"
	"example.com/todo-cli/src/models"
	"example.com/todo-cli/src/services"
)

func TestSetPriority_UpdatesExistingTask(t *testing.T) {
	store := services.NewStore(filepath.Join(t.TempDir(), "tasks.json"))
	if err := store.Save([]models.Task{{ID: "1", Priority: "low"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := cli.SetPriority(store, "1", "high"); err != nil {
		t.Fatalf("SetPriority: %v", err)
	}

	tasks, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if tasks[0].Priority != "high" {
		t.Fatalf("Priority = %q, want %q", tasks[0].Priority, "high")
	}
}

func TestSetPriority_UnknownIDReturnsNotFoundAndLeavesStoreUnmodified(t *testing.T) {
	store := services.NewStore(filepath.Join(t.TempDir(), "tasks.json"))
	original := []models.Task{{ID: "1", Priority: "low"}}
	if err := store.Save(original); err != nil {
		t.Fatalf("Save: %v", err)
	}

	err := cli.SetPriority(store, "does-not-exist", "high")
	if err == nil {
		t.Fatal("expected an error for an unknown task id")
	}

	tasks, loadErr := store.Load()
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if tasks[0].Priority != "low" {
		t.Fatalf("expected store to be unmodified, got Priority = %q", tasks[0].Priority)
	}
}
