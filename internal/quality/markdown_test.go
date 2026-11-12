package quality

import (
	"reflect"
	"strings"
	"testing"
)

func TestMarkdownFenceParsingAndCheckboxNormalization(t *testing.T) {
	for _, marker := range []string{"````", "~~~"} {
		t.Run(marker, func(t *testing.T) {
			md := "## Task 1: Actual\n" + marker + "markdown\n## Task 99: Quoted\n- [ ] **Step 1: Quoted example**\n" + marker + "\n- [ ] **Step 1: Actual step**\n"
			if got := TaskNumbers(md); !reflect.DeepEqual(got, []int{1}) {
				t.Fatalf("quoted tasks counted: %v", got)
			}
			edited := strings.Replace(md, "- [ ] **Step 1: Actual", "- [X] **Step 1: Actual", 1)
			if string(normalizedPlan([]byte(md))) != string(normalizedPlan([]byte(edited))) {
				t.Fatal("actual checkbox was not normalized after long/tilde fence")
			}
			quoted := strings.Replace(md, "- [ ] **Step 1: Quoted", "- [X] **Step 1: Quoted", 1)
			if string(normalizedPlan([]byte(md))) == string(normalizedPlan([]byte(quoted))) {
				t.Fatal("quoted content edit was ignored")
			}
		})
	}
}

func TestFingerprintRejectsExcludedMappedFile(t *testing.T) {
	p := fixture(t)
	c, _ := LoadContract(p)
	c.Cases[0].File = ".mulix/.runtime/001-x/report.md"
	put(t, p.Root, c.Cases[0].File, "input must not be silently omitted")
	if _, err := Fingerprint(p, c, 1); err == nil {
		t.Fatal("excluded test input accepted without being bound")
	}
}
