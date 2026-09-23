// Package models holds the todo-cli data model.
package models

import (
	"sort"
	"time"
)

// Priority levels a task can carry, from least to most urgent.
const (
	PriorityLow    = "low"
	PriorityMedium = "medium"
	PriorityHigh   = "high"
)

// Task is a single to-do item as stored on disk.
type Task struct {
	ID          int       `json:"id"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	Done        bool      `json:"done"`
	Priority    string    `json:"priority,omitempty"`
}

// NextID returns the ID a newly created task should get: one past the
// highest existing ID, or 1 for an empty list.
func NextID(tasks []Task) int {
	max := 0
	for _, t := range tasks {
		if t.ID > max {
			max = t.ID
		}
	}
	return max + 1
}

// PriorityOrDefault resolves the task's priority to one of the three
// levels, mapping an empty value (tasks stored before this feature
// existed, or a task constructed without one) to medium. All ordering
// and display paths go through this instead of reading Priority
// directly, so the default is applied in exactly one place.
func (t Task) PriorityOrDefault() string {
	switch t.Priority {
	case PriorityLow, PriorityMedium, PriorityHigh:
		return t.Priority
	default:
		return PriorityMedium
	}
}

// priorityRank maps a priority level to a sort weight; higher ranks
// sort earlier.
func priorityRank(level string) int {
	switch level {
	case PriorityHigh:
		return 3
	case PriorityMedium:
		return 2
	default:
		return 1
	}
}

// SortByPriority orders tasks high → medium → low in place, breaking
// ties within the same priority by creation order, oldest first
// (FR-006). The explicit created-at comparison makes the contract
// self-contained: the result is correct regardless of the input
// slice's order, not just when the caller happens to pass tasks in
// creation order.
func SortByPriority(tasks []Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		ri := priorityRank(tasks[i].PriorityOrDefault())
		rj := priorityRank(tasks[j].PriorityOrDefault())
		if ri != rj {
			return ri > rj
		}
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})
}
