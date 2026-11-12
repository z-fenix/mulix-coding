package guard

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/quality"
)

// TaskNumbers returns the numbers of every "## Task N" heading in a
// tasks.md, in document order, ignoring headings inside ``` fences.
func TaskNumbers(tasksMD string) []int {
	return quality.TaskNumbers(tasksMD)
}

// ledgerCompleteRe matches a list item or legacy plain completion line:
// "- Task 3: complete (commits ..., ...)". Fix-round and ruling lines
// ("Task 3: fix round 1/5", "Task 3: Ruling: ...") are not matches.
var ledgerCompleteRe = regexp.MustCompile(`^(?:-[ \t]+)?Task[ \t]+([0-9]+):[ \t]+complete\b`)

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
