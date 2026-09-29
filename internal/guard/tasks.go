package guard

import (
	"regexp"
	"strconv"
	"strings"
)

// taskHeadingRe matches a task section heading: "## Task 3: Parser" (any
// heading level from ## down, matching how the build executors' task-brief
// extraction finds a task). Text inside fenced code blocks is excluded by
// the caller, so example plans quoted in a task don't count.
var taskHeadingRe = regexp.MustCompile(`^#{2,}[ \t]+Task[ \t]+([0-9]+)([^0-9]|$)`)

// TaskNumbers returns the numbers of every "## Task N" heading in a
// tasks.md, in document order, ignoring headings inside ``` fences.
func TaskNumbers(tasksMD string) []int {
	var out []int
	inFence := false
	for line := range strings.SplitSeq(tasksMD, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := taskHeadingRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		out = append(out, n)
	}
	return out
}

// ledgerCompleteRe matches a ledger completion line: "Task 3: complete
// (commits ..., ...)". Fix-round and ruling lines ("Task 3: fix round
// 1/5", "Task 3: Ruling: ...") are deliberately not matches.
var ledgerCompleteRe = regexp.MustCompile(`^Task[ \t]+([0-9]+):[ \t]+complete\b`)

// CompletedTasks returns the set of task numbers the ledger records as
// complete.
func CompletedTasks(ledger string) map[int]bool {
	done := map[int]bool{}
	for line := range strings.SplitSeq(ledger, "\n") {
		m := ledgerCompleteRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil {
			done[n] = true
		}
	}
	return done
}
