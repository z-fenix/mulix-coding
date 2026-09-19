// Package models defines the Task type shared by the todo-cli commands.
package models

import (
	"sort"
	"time"
)

// Task is a single to-do item.
type Task struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Done        bool      `json:"done"`
	CreatedAt   time.Time `json:"created_at"`

	// Priority is one of "low", "medium", "high". Empty string (including
	// tasks stored before this field existed) means "use the default";
	// callers should read PriorityOrDefault() rather than this field
	// directly whenever the resolved value matters (sorting, display).
	Priority string `json:"priority,omitempty"`
}

// DefaultPriority is used whenever a task has no explicit priority set,
// including tasks stored on disk before the priority field existed.
const DefaultPriority = "medium"

// ValidPriorities are the only priority values todo-cli accepts.
var ValidPriorities = []string{"low", "medium", "high"}

// PriorityOrDefault returns t.Priority, or DefaultPriority if it's empty.
// This is the single place the "missing priority means medium" rule is
// applied; callers that care about the resolved priority (sorting,
// display) should use this instead of reading Priority directly.
func (t Task) PriorityOrDefault() string {
	if t.Priority == "" {
		return DefaultPriority
	}
	return t.Priority
}

// priorityRank orders priorities from most to least urgent, for sorting.
var priorityRank = map[string]int{"high": 3, "medium": 2, "low": 1}

// SortByPriority sorts tasks high -> medium -> low, breaking ties within
// the same priority by creation order (oldest first). It sorts in place
// and is stable, so equal-priority tasks retain their relative order
// when they also share a creation time.
func SortByPriority(tasks []Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		ra, rb := priorityRank[a.PriorityOrDefault()], priorityRank[b.PriorityOrDefault()]
		if ra != rb {
			return ra > rb
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
}
