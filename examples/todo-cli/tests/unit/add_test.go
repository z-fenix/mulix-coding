package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T005: add's --priority flag — valid values, the medium default,
// rejection of invalid values, and position-independent parsing.

func TestRun_AddWithPriorityStoresLevel(t *testing.T) {
	for _, level := range []string{"low", "medium", "high"} {
		t.Run(level, func(t *testing.T) {
			store := filepath.Join(t.TempDir(), "tasks.json")

			out, code := runTodo(t, store, "add", "buy milk", "--priority", level)
			if code != 0 {
				t.Fatalf("add exit = %d, want 0 (output: %s)", code, out)
			}

			data, err := os.ReadFile(store)
			if err != nil {
				t.Fatalf("store was not written: %v", err)
			}
			var tasks []map[string]any
			if err := json.Unmarshal(data, &tasks); err != nil {
				t.Fatalf("parsing store: %v", err)
			}
			if len(tasks) != 1 {
				t.Fatalf("expected 1 task, got %d", len(tasks))
			}
			if got := tasks[0]["priority"]; got != level {
				t.Fatalf("stored priority = %v, want %q", got, level)
			}
		})
	}
}

func TestRun_AddWithoutPriorityDefaultsToMedium(t *testing.T) {
	store := filepath.Join(t.TempDir(), "tasks.json")

	out, code := runTodo(t, store, "add", "buy milk")
	if code != 0 {
		t.Fatalf("add exit = %d, want 0 (output: %s)", code, out)
	}

	data, err := os.ReadFile(store)
	if err != nil {
		t.Fatalf("store was not written: %v", err)
	}
	if !strings.Contains(string(data), `"priority": "medium"`) {
		t.Fatalf("expected stored task to default to medium, got: %s", data)
	}
}

func TestRun_AddWithInvalidPriorityRejectsAndWritesNothing(t *testing.T) {
	store := filepath.Join(t.TempDir(), "tasks.json")

	out, code := runTodo(t, store, "add", "bad input", "--priority", "urgent")
	if code == 0 {
		t.Fatalf("expected non-zero exit for invalid --priority (output: %s)", out)
	}
	if _, err := os.Stat(store); !os.IsNotExist(err) {
		t.Fatalf("invalid --priority must not write a task, store stat err = %v", err)
	}
}

func TestRun_AddWithTrailingPriorityFlagIsParsedCorrectly(t *testing.T) {
	// Regression guard for the usage shape the CLI documents:
	// description first, flag last. A flag package that stops parsing at
	// the first positional argument silently drops --priority here.
	store := filepath.Join(t.TempDir(), "tasks.json")

	out, code := runTodo(t, store, "add", "buy milk", "--priority", "high")
	if code != 0 {
		t.Fatalf("add exit = %d, want 0 (output: %s)", code, out)
	}
	data, err := os.ReadFile(store)
	if err != nil {
		t.Fatalf("store was not written: %v", err)
	}
	if !strings.Contains(string(data), `"high"`) {
		t.Fatalf("expected --priority high after the description to be honored, got: %s", data)
	}
}
