package guard

import (
	"fmt"
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

var nativeReceiptID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// completedReceiptTasks binds native closure to the latest runner attempt.
// The separate tdd-evidence-present guard validates those attempts against
// the contract. Later tasks may advance source without undoing earlier work.
func completedReceiptTasks(ledger string, tasks []quality.Task, records []quality.RunRecord) map[int]bool {
	markers := make(map[string]int, len(tasks))
	for _, task := range tasks {
		stage := "check"
		if task.Kind == "code" {
			stage = "green"
		}
		r, ok := quality.Latest(records, task.Number, stage)
		if ok && r.Success && r.Error == "" && nativeReceiptID.MatchString(r.ID) {
			markers[fmt.Sprintf("- Task %d: complete (receipt %s)", task.Number, r.ID)] = task.Number
		}
	}
	done := make(map[int]bool, len(markers))
	for line := range strings.SplitSeq(ledger, "\n") {
		if n, ok := markers[strings.TrimSuffix(line, "\r")]; ok {
			done[n] = true
		}
	}
	return done
}
