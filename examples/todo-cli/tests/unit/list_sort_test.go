package unit

import (
	"path/filepath"
	"testing"
	"time"

	"example.com/todo-cli/src/cli"
	"example.com/todo-cli/src/models"
	"example.com/todo-cli/src/services"
)

func TestList_OrdersHighMediumLowWithOldestFirstTies(t *testing.T) {
	store := services.NewStore(filepath.Join(t.TempDir(), "tasks.json"))
	now := time.Now()

	tasks := []models.Task{
		{ID: "1", Description: "low one", Priority: "low", CreatedAt: now},
		{ID: "2", Description: "high older", Priority: "high", CreatedAt: now.Add(time.Second)},
		{ID: "3", Description: "medium one", Priority: "medium", CreatedAt: now.Add(2 * time.Second)},
		{ID: "4", Description: "high newer", Priority: "high", CreatedAt: now.Add(3 * time.Second)},
	}
	if err := store.Save(tasks); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := cli.List(store)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	wantOrder := []string{"2", "4", "3", "1"}
	if len(got) != len(wantOrder) {
		t.Fatalf("got %d tasks, want %d", len(got), len(wantOrder))
	}
	for i, id := range wantOrder {
		if got[i].ID != id {
			t.Fatalf("position %d: got id %q, want %q", i, got[i].ID, id)
		}
	}
}
