package models

import (
	"testing"
	"time"
)

func TestNextID_EmptyListStartsAtOne(t *testing.T) {
	if got := NextID(nil); got != 1 {
		t.Fatalf("NextID(nil) = %d, want 1", got)
	}
}

func TestNextID_OnePastHighestExisting(t *testing.T) {
	tasks := []Task{{ID: 3}, {ID: 7}}
	if got := NextID(tasks); got != 8 {
		t.Fatalf("NextID = %d, want 8", got)
	}
}

// T001: priority default + sort comparator tests (written before the
// Priority field exists — RED).

func TestPriorityOrDefault_EmptyStringIsMedium(t *testing.T) {
	task := Task{ID: 1, Description: "buy milk"}
	if got := task.PriorityOrDefault(); got != "medium" {
		t.Fatalf("PriorityOrDefault() = %q, want %q (pre-existing tasks without a priority field are medium)", got, "medium")
	}
}

func TestPriorityOrDefault_ExplicitValuesRoundTrip(t *testing.T) {
	for _, level := range []string{"low", "medium", "high"} {
		task := Task{ID: 1, Priority: level}
		if got := task.PriorityOrDefault(); got != level {
			t.Fatalf("PriorityOrDefault() = %q, want %q", got, level)
		}
	}
}

func TestSortByPriority_HighMediumLow(t *testing.T) {
	tasks := []Task{
		{ID: 1, Description: "low one", Priority: "low"},
		{ID: 2, Description: "high one", Priority: "high"},
		{ID: 3, Description: "medium one", Priority: "medium"},
	}
	SortByPriority(tasks)
	want := []string{"high one", "medium one", "low one"}
	for i, w := range want {
		if tasks[i].Description != w {
			t.Fatalf("tasks[%d].Description = %q, want %q (full order: %v)", i, tasks[i].Description, w, tasks)
		}
	}
}

func TestSortByPriority_TiesBreakOldestFirst(t *testing.T) {
	early := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	late := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	tasks := []Task{
		{ID: 2, Description: "second", CreatedAt: late, Priority: "high"},
		{ID: 1, Description: "first", CreatedAt: early, Priority: "high"},
	}
	SortByPriority(tasks)
	if tasks[0].Description != "first" {
		t.Fatalf("same-priority tie did not break oldest-first: %v", tasks)
	}
}
