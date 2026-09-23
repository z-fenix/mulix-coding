package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/todo-cli/src/cli"
)

// runTodo executes one todo command against a store at storePath and
// returns its combined stdout+stderr plus exit code, exercising the
// real argv/dispatch layer rather than internal helpers. Errors (like
// "task not found") go to stderr, so tests asserting on error text
// need both streams.
func runTodo(t *testing.T, storePath string, args ...string) (string, int) {
	t.Helper()
	t.Setenv("TODO_FILE", storePath)

	origStdout := os.Stdout
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	os.Stderr = w
	code := cli.Run(args)
	w.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr
	out, err := readAll(r)
	if err != nil {
		t.Fatalf("reading captured output: %v", err)
	}
	return out, code
}

// T003: a store file written before this feature exists — task JSON
// objects with no priority key at all — must load and list as medium.
func TestLoad_TaskWithoutPriorityFieldDefaultsToMedium(t *testing.T) {
	store := filepath.Join(t.TempDir(), "tasks.json")
	content := `[
  {
    "id": 1,
    "description": "old task",
    "created_at": "2025-01-01T00:00:00Z",
    "done": false
  }
]`
	if err := os.WriteFile(store, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out, code := runTodo(t, store, "list")
	if code != 0 {
		t.Fatalf("list exit = %d, want 0 (output: %s)", code, out)
	}
	if !strings.Contains(out, "medium") {
		t.Fatalf("expected pre-priority task to list as medium, got: %s", out)
	}
}
