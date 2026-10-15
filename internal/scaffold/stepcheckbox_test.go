package scaffold

import (
	"regexp"
	"testing"
)

func TestTaskTemplate_InstalledStepCheckboxesStartUnchecked(t *testing.T) {
	root := t.TempDir()
	if _, err := Init(InitOptions{Root: root, Hosts: []Host{HostDSH}}); err != nil {
		t.Fatal(err)
	}
	template := readProjectFile(t, root, ".mulix/templates/tasks-template.md")
	steps := regexp.MustCompile(`(?m)^- \[ \] \*\*Step ([1-5]):`).FindAllStringSubmatch(template, -1)
	if len(steps) != 5 {
		t.Fatalf("expected 5 unchecked Step headings, got %d", len(steps))
	}
	for i, step := range steps {
		if step[1] != []string{"1", "2", "3", "4", "5"}[i] {
			t.Fatalf("step order=%v", steps)
		}
	}
	if regexp.MustCompile(`(?m)^- \[X\] \*\*Step`).MatchString(template) {
		t.Fatal("template pre-checked an unexecuted step")
	}
}
