package unit

import (
	"os"
	"path/filepath"
	"testing"

	"example.com/todo-cli/src/cli"
	"example.com/todo-cli/src/services"
)

func TestAdd_ExplicitPriorityIsStored(t *testing.T) {
	store := services.NewStore(filepath.Join(t.TempDir(), "tasks.json"))

	if err := cli.Add(store, "buy milk", "high"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	tasks, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Priority != "high" {
		t.Fatalf("Priority = %q, want %q", tasks[0].Priority, "high")
	}
}

func TestAdd_OmittedPriorityDefaultsToMedium(t *testing.T) {
	store := services.NewStore(filepath.Join(t.TempDir(), "tasks.json"))

	if err := cli.Add(store, "buy milk", ""); err != nil {
		t.Fatalf("Add: %v", err)
	}

	tasks, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := tasks[0].PriorityOrDefault(); got != "medium" {
		t.Fatalf("PriorityOrDefault() = %q, want %q", got, "medium")
	}
}

func TestRun_AddWithTrailingPriorityFlagIsParsedCorrectly(t *testing.T) {
	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	// Regression test: the CLI's usage is "todo add <description>
	// --priority <level>" (flag trailing the positional description),
	// which the stdlib flag package does not parse correctly (it stops
	// consuming at the first non-flag argument). This exercises the
	// actual argv split, not just the cli.Add function directly.
	if code := cli.Run([]string{"add", "buy milk", "--priority", "high"}); code != 0 {
		t.Fatalf("Run(add ... --priority high) exit code = %d, want 0", code)
	}

	store := services.NewStore("tasks.json")
	tasks, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Priority != "high" {
		t.Fatalf("expected 1 task with priority high, got %+v", tasks)
	}
}

func TestAdd_InvalidPriorityIsRejectedAndNothingIsWritten(t *testing.T) {
	store := services.NewStore(filepath.Join(t.TempDir(), "tasks.json"))

	err := cli.Add(store, "buy milk", "urgent")
	if err == nil {
		t.Fatal("expected an error for an invalid priority value")
	}

	tasks, loadErr := store.Load()
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected no task to be written on invalid priority, got %d", len(tasks))
	}
}
