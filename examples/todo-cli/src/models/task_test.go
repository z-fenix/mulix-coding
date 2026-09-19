package models

import (
	"testing"
	"time"
)

func TestPriorityOrDefault_EmptyIsMedium(t *testing.T) {
	task := Task{}
	if got := task.PriorityOrDefault(); got != "medium" {
		t.Fatalf("PriorityOrDefault() = %q, want %q", got, "medium")
	}
}

func TestPriorityOrDefault_ExplicitValuePreserved(t *testing.T) {
	task := Task{Priority: "low"}
	if got := task.PriorityOrDefault(); got != "low" {
		t.Fatalf("PriorityOrDefault() = %q, want %q", got, "low")
	}
}

func TestSortByPriority_OrdersHighMediumLow(t *testing.T) {
	now := time.Now()
	tasks := []Task{
		{ID: "1", Priority: "low", CreatedAt: now},
		{ID: "2", Priority: "high", CreatedAt: now.Add(time.Second)},
		{ID: "3", Priority: "medium", CreatedAt: now.Add(2 * time.Second)},
	}

	SortByPriority(tasks)

	want := []string{"2", "3", "1"}
	for i, id := range want {
		if tasks[i].ID != id {
			t.Fatalf("position %d: got id %q, want %q (order: %v)", i, tasks[i].ID, id, ids(tasks))
		}
	}
}

func TestSortByPriority_TiesBreakOldestFirst(t *testing.T) {
	now := time.Now()
	tasks := []Task{
		{ID: "newer", Priority: "high", CreatedAt: now.Add(time.Minute)},
		{ID: "older", Priority: "high", CreatedAt: now},
	}

	SortByPriority(tasks)

	if tasks[0].ID != "older" || tasks[1].ID != "newer" {
		t.Fatalf("expected oldest-first tie break, got order %v", ids(tasks))
	}
}

func ids(tasks []Task) []string {
	out := make([]string, len(tasks))
	for i, task := range tasks {
		out[i] = task.ID
	}
	return out
}
