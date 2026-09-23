package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T009: todo set-priority <id> <level>.

func TestRun_SetPriorityUpdatesExistingTask(t *testing.T) {
	store := filepath.Join(t.TempDir(), "tasks.json")

	if _, code := runTodo(t, store, "add", "buy milk", "--priority", "low"); code != 0 {
		t.Fatalf("seed add failed")
	}

	out, code := runTodo(t, store, "set-priority", "1", "high")
	if code != 0 {
		t.Fatalf("set-priority exit = %d, want 0 (output: %s)", code, out)
	}

	data, err := os.ReadFile(store)
	if err != nil {
		t.Fatalf("reading store: %v", err)
	}
	if !strings.Contains(string(data), `"high"`) {
		t.Fatalf("expected priority updated to high, got: %s", data)
	}
}

func TestRun_SetPriorityUnknownIdFailsWithoutModifyingStore(t *testing.T) {
	store := filepath.Join(t.TempDir(), "tasks.json")

	if _, code := runTodo(t, store, "add", "buy milk", "--priority", "low"); code != 0 {
		t.Fatalf("seed add failed")
	}
	before, err := os.ReadFile(store)
	if err != nil {
		t.Fatalf("reading store: %v", err)
	}

	out, code := runTodo(t, store, "set-priority", "99", "high")
	if code == 0 {
		t.Fatalf("expected non-zero exit for unknown id (output: %s)", out)
	}
	if !strings.Contains(strings.ToLower(out), "task not found") {
		t.Fatalf("expected a task not found error, got: %s", out)
	}

	after, err := os.ReadFile(store)
	if err != nil {
		t.Fatalf("reading store: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("store must be unmodified after a failed set-priority\nbefore: %s\nafter:  %s", before, after)
	}
}
