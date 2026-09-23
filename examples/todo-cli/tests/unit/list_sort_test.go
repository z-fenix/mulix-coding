package tests

import (
	"path/filepath"
	"strings"
	"testing"
)

// T007: todo list orders high → medium → low, ties oldest-first.

func TestList_OrdersHighMediumLow(t *testing.T) {
	store := filepath.Join(t.TempDir(), "tasks.json")

	// Add in low, high, medium order; list must not preserve this order.
	if _, code := runTodo(t, store, "add", "low task", "--priority", "low"); code != 0 {
		t.Fatalf("seed add failed")
	}
	if _, code := runTodo(t, store, "add", "high task", "--priority", "high"); code != 0 {
		t.Fatalf("seed add failed")
	}
	if _, code := runTodo(t, store, "add", "medium task", "--priority", "medium"); code != 0 {
		t.Fatalf("seed add failed")
	}

	out, code := runTodo(t, store, "list")
	if code != 0 {
		t.Fatalf("list exit = %d, want 0 (output: %s)", code, out)
	}
	hi := strings.Index(out, "high task")
	med := strings.Index(out, "medium task")
	lo := strings.Index(out, "low task")
	if hi == -1 || med == -1 || lo == -1 {
		t.Fatalf("missing task lines in list output: %s", out)
	}
	if !(hi < med && med < lo) {
		t.Fatalf("expected high < medium < low order, got: %s", out)
	}
}

func TestList_TiesBreakOldestFirst(t *testing.T) {
	store := filepath.Join(t.TempDir(), "tasks.json")

	if _, code := runTodo(t, store, "add", "first high", "--priority", "high"); code != 0 {
		t.Fatalf("seed add failed")
	}
	if _, code := runTodo(t, store, "add", "second high", "--priority", "high"); code != 0 {
		t.Fatalf("seed add failed")
	}

	out, code := runTodo(t, store, "list")
	if code != 0 {
		t.Fatalf("list exit = %d, want 0 (output: %s)", code, out)
	}
	first := strings.Index(out, "first high")
	second := strings.Index(out, "second high")
	if first == -1 || second == -1 || first > second {
		t.Fatalf("expected oldest same-priority task listed first, got: %s", out)
	}
}
