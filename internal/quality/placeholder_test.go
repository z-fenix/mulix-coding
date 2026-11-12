package quality

import (
	"strings"
	"testing"
)

func TestContractAllowsTodoDomainValues(t *testing.T) {
	for _, scenario := range []string{"expectation", "module", "suite", "file", "test-identity"} {
		t.Run(scenario, func(t *testing.T) {
			p := fixture(t)
			switch scenario {
			case "expectation":
				put(t, p.Root, p.SpecPath, strings.Replace(specFixture, "The requested value is returned unchanged.", "Creating a todo returns its identifier.", 1))
			case "module":
				put(t, p.Root, p.TasksPath, strings.Replace(planFixture, "fixture#TestValue", "example.org/todo-cli#TestValue", 1))
			case "suite":
				put(t, p.Root, p.TasksPath, strings.Replace(planFixture, "suite: unit", "suite: todo", 1))
				put(t, p.Root, PolicyPath, strings.Replace(policyFixture, "id: unit", "id: todo", 1))
			case "file":
				put(t, p.Root, "todo_test.go", "package fixture\n")
				put(t, p.Root, p.TasksPath, strings.Replace(planFixture, "file: value_test.go", "file: todo_test.go", 1))
			case "test-identity":
				put(t, p.Root, p.TasksPath, strings.Replace(planFixture, "test: fixture#TestValue", "test: TODO", 1))
			}
			c, err := LoadContract(p)
			if err != nil {
				t.Fatalf("legitimate todo domain value rejected: %v", err)
			}
			policy, err := LoadPolicy(p.Root)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateMapping(c, policy); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestConcreteRecognizesExplicitPlaceholders(t *testing.T) {
	for _, text := range []string{"", "TBD", "TODO", "TODO: define the expected value", "[expected observable outcome]", "[NEEDS CLARIFICATION: retention policy]"} {
		if concrete(text) {
			t.Errorf("placeholder accepted: %q", text)
		}
	}
	for _, text := range []string{"Creating a todo returns its identifier.", "The literal result is TODO.", "[1, 2]", "[\"todo\"]"} {
		if !concrete(text) {
			t.Errorf("observable result rejected: %q", text)
		}
	}
}
