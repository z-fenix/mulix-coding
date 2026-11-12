// Package quality implements local, source-bound test execution and acceptance
// contracts. Receipts reduce accidental false claims; they are not signatures
// or a sandbox against a writer who controls the project filesystem.
package quality

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"gopkg.in/yaml.v3"
)

const PolicyPath = ".mulix/quality.yaml"

type Project struct{ Root, Change, SpecPath, TasksPath string }
type Criterion struct {
	ID          string `yaml:"id" json:"id"`
	Requirement string `yaml:"requirement" json:"requirement"`
	Expected    string `yaml:"expected" json:"expected"`
}
type Task struct {
	Number int    `yaml:"number" json:"number"`
	Kind   string `yaml:"kind" json:"kind"`
	Reason string `yaml:"reason,omitempty" json:"reason,omitempty"`
}
type Case struct {
	ID        string `yaml:"id" json:"id"`
	Criterion string `yaml:"criterion" json:"criterion"`
	Task      int    `yaml:"task" json:"task"`
	Point     string `yaml:"point" json:"point"`
	Given     string `yaml:"given" json:"given"`
	When      string `yaml:"when" json:"when"`
	Then      string `yaml:"then" json:"then"`
	Suite     string `yaml:"suite" json:"suite"`
	Test      string `yaml:"test" json:"test"`
	File      string `yaml:"file" json:"file"`
}
type Contract struct {
	Criteria []Criterion `yaml:"criteria" json:"criteria"`
	Tasks    []Task      `yaml:"tasks" json:"tasks"`
	Cases    []Case      `yaml:"cases" json:"cases"`
}
type Suite struct {
	ID             string   `yaml:"id" json:"id"`
	Command        []string `yaml:"command" json:"command"`
	CWD            string   `yaml:"cwd" json:"cwd"`
	Format         string   `yaml:"format" json:"format"`
	TimeoutSeconds int      `yaml:"timeout_seconds" json:"timeout_seconds"`
}
type Policy struct {
	Schema string  `yaml:"schema" json:"schema"`
	Suites []Suite `yaml:"suites" json:"suites"`
}

func ReadProjectFile(root, rel string) ([]byte, error) {
	p, err := fileutil.ControlPath(root, rel, false)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}
func decodeStrict(data []byte, value any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("exactly one YAML document is required")
	}
	return nil
}

// Block extracts exactly one explicitly tagged YAML fence. Ordinary examples
// inside another Markdown fence do not count as machine-readable contracts.
func Block(md []byte, tag string, value any) error {
	var blocks []string
	var fence string
	var selected bool
	var lines []string
	for _, line := range strings.Split(string(md), "\n") {
		line = strings.TrimSuffix(line, "\r")
		t := strings.TrimSpace(line)
		if fence == "" {
			if marker := fenceStart(t); marker != "" {
				fence = marker
				selected = strings.TrimSpace(t[len(marker):]) == tag
				lines = nil
			}
		} else if fenceEnd(t, fence) {
			if selected {
				blocks = append(blocks, strings.Join(lines, "\n"))
			}
			fence = ""
			selected = false
		} else if selected {
			lines = append(lines, line)
		}
	}
	if len(blocks) != 1 || (selected && fence != "") {
		return fmt.Errorf("expected exactly one complete %s YAML fence", tag)
	}
	if err := decodeStrict([]byte(blocks[0]), value); err != nil {
		return fmt.Errorf("%s: %w", tag, err)
	}
	return nil
}
func concrete(s string) bool {
	t := strings.TrimSpace(s)
	upper := strings.ToUpper(t)
	if t == "" {
		return false
	}
	for _, marker := range []string{"TBD", "TODO", "NEEDS CLARIFICATION"} {
		if upper == marker || strings.HasPrefix(upper, marker+":") || strings.Contains(upper, "["+marker) {
			return false
		}
	}
	// A whole bracketed template slot is a placeholder; a literal JSON array
	// can be an independently observable expected result. Domain words such as
	// todo in ordinary prose are not placeholder markers.
	return !(strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") && !json.Valid([]byte(t)))
}

var frPattern = regexp.MustCompile(`\bFR-[0-9]{3,}\b`)
var acPattern = regexp.MustCompile(`^AC-[0-9]{3,}$`)
var casePattern = regexp.MustCompile(`^TC-[0-9]{3,}$`)

func ValidateSpec(p Project) ([]Criterion, error) {
	data, err := ReadProjectFile(p.Root, p.SpecPath)
	if err != nil {
		return nil, err
	}
	var spec struct {
		Criteria []Criterion `yaml:"criteria"`
	}
	if err := Block(data, "mulix-acceptance", &spec); err != nil {
		return nil, err
	}
	if len(spec.Criteria) == 0 {
		return nil, fmt.Errorf("acceptance criteria are empty")
	}
	ids := map[string]bool{}
	covered := map[string]bool{}
	for _, c := range spec.Criteria {
		if !acPattern.MatchString(c.ID) || ids[c.ID] || !frPattern.MatchString(c.Requirement) || !concrete(c.Expected) {
			return nil, fmt.Errorf("invalid or duplicate acceptance criterion %q", c.ID)
		}
		ids[c.ID] = true
		covered[c.Requirement] = true
	}
	requirements := frPattern.FindAllString(string(data), -1)
	for _, id := range requirements {
		if !covered[id] {
			return nil, fmt.Errorf("requirement %s has no acceptance criterion", id)
		}
	}
	// A criterion must reference a requirement in prose, not invent one solely
	// by mentioning it inside the acceptance block.
	proseRequirements := map[string]bool{}
	for _, id := range frPattern.FindAllString(proseOnly(string(data)), -1) {
		proseRequirements[id] = true
	}
	for id := range covered {
		if !proseRequirements[id] {
			return nil, fmt.Errorf("criterion references absent requirement %s", id)
		}
	}
	return spec.Criteria, nil
}

// TaskNumbers is shared by the acceptance contract and phase guards.
func TaskNumbers(md string) []int {
	var out []int
	re := regexp.MustCompile(`^#{2,}[ \t]+Task[ \t]+([0-9]+)([^0-9]|$)`)
	for line := range strings.SplitSeq(proseOnly(md), "\n") {
		line = strings.TrimRight(line, "\r")
		if m := re.FindStringSubmatch(line); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil {
				out = append(out, n)
			}
		}
	}
	return out
}
func LoadContract(p Project) (Contract, error) {
	var c Contract
	criteria, err := ValidateSpec(p)
	if err != nil {
		return c, err
	}
	c.Criteria = criteria
	data, err := ReadProjectFile(p.Root, p.TasksPath)
	if err != nil {
		return c, err
	}
	var matrix struct {
		Tasks []Task `yaml:"tasks"`
		Cases []Case `yaml:"cases"`
	}
	if err := Block(data, "mulix-tests", &matrix); err != nil {
		return c, err
	}
	c.Tasks = matrix.Tasks
	c.Cases = matrix.Cases
	numbers := TaskNumbers(string(data))
	if len(c.Tasks) == 0 || len(c.Tasks) != len(numbers) {
		return c, fmt.Errorf("test matrix tasks must match all Task headings")
	}
	taskKinds := map[int]string{}
	for i, task := range c.Tasks {
		if task.Number != i+1 || numbers[i] != task.Number {
			return c, fmt.Errorf("matrix and plan tasks must be numbered 1..n in order")
		}
		switch task.Kind {
		case "code":
		case "verification", "documentation":
			if !concrete(task.Reason) {
				return c, fmt.Errorf("non-code task %d needs an explicit reason", task.Number)
			}
		default:
			return c, fmt.Errorf("unknown kind for task %d", task.Number)
		}
		taskKinds[task.Number] = task.Kind
	}
	ids := map[string]bool{}
	known := map[string]bool{}
	covered := map[string]bool{}
	taskCovered := map[int]bool{}
	for _, a := range c.Criteria {
		known[a.ID] = true
	}
	for _, tc := range c.Cases {
		if !casePattern.MatchString(tc.ID) || ids[tc.ID] {
			return c, fmt.Errorf("invalid or duplicate test case %q", tc.ID)
		}
		ids[tc.ID] = true
		if !known[tc.Criterion] || taskKinds[tc.Task] == "" {
			return c, fmt.Errorf("case %s has an unknown criterion or task", tc.ID)
		}
		for _, text := range []string{tc.Point, tc.Given, tc.When, tc.Then} {
			if !concrete(text) {
				return c, fmt.Errorf("case %s needs a concrete point, precondition, action and expectation", tc.ID)
			}
		}
		// Exact emitted names and physical paths are identifiers, not prose:
		// TODO can be a real test name and todo_test.go a real source file.
		for _, identity := range []string{tc.Suite, tc.Test, tc.File} {
			if strings.TrimSpace(identity) == "" {
				return c, fmt.Errorf("case %s needs a suite, exact test identity and test file", tc.ID)
			}
		}
		if excluded(p, tc.File) {
			return c, fmt.Errorf("case %s maps a workflow output instead of a test input: %s", tc.ID, tc.File)
		}
		if _, err := fileutil.ControlPath(p.Root, tc.File, false); err != nil {
			return c, err
		}
		covered[tc.Criterion] = true
		taskCovered[tc.Task] = true
	}
	for _, a := range c.Criteria {
		if !covered[a.ID] {
			return c, fmt.Errorf("criterion %s has no test case", a.ID)
		}
	}
	for _, task := range c.Tasks {
		if !taskCovered[task.Number] {
			return c, fmt.Errorf("task %d has no executable case", task.Number)
		}
	}
	return c, nil
}
func SafeDir(root, rel string) (string, error) {
	if rel == "." {
		return filepath.Abs(root)
	}
	if rel == "" || filepath.IsAbs(rel) || strings.ContainsAny(rel, `\:`) {
		return "", fmt.Errorf("invalid suite cwd %q", rel)
	}
	p, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid suite cwd %q", rel)
		}
		p = filepath.Join(p, part)
		info, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("suite cwd is not a physical directory: %s", p)
		}
	}
	return p, nil
}
func LoadPolicy(root string) (Policy, error) {
	var p Policy
	data, err := ReadProjectFile(root, PolicyPath)
	if err != nil {
		return p, fmt.Errorf("quality policy: %w", err)
	}
	if err := decodeStrict(data, &p); err != nil {
		return p, err
	}
	if p.Schema != "mulix.quality.v1" || len(p.Suites) == 0 {
		return p, fmt.Errorf("quality policy needs schema mulix.quality.v1 and at least one suite")
	}
	seen := map[string]bool{}
	for i := range p.Suites {
		s := &p.Suites[i]
		if !regexpSuiteID.MatchString(s.ID) || seen[s.ID] || len(s.Command) == 0 || strings.TrimSpace(s.Command[0]) == "" {
			return p, fmt.Errorf("suite needs unique id and non-empty argv")
		}
		seen[s.ID] = true
		if s.Format != "go-json" && s.Format != "json" {
			return p, fmt.Errorf("suite %s format must be go-json or json", s.ID)
		}
		if _, err := SafeDir(root, s.CWD); err != nil {
			return p, err
		}
		if s.TimeoutSeconds < 0 || s.TimeoutSeconds > 7200 {
			return p, fmt.Errorf("suite %s timeout must be 1..7200 seconds (0 uses 300)", s.ID)
		}
		if s.TimeoutSeconds == 0 {
			s.TimeoutSeconds = 300
		}
	}
	return p, nil
}
func (c Contract) Task(number int) (Task, bool) {
	for _, t := range c.Tasks {
		if t.Number == number {
			return t, true
		}
	}
	return Task{}, false
}
func (c Contract) SelectedCases(task int) []Case {
	var out []Case
	for _, tc := range c.Cases {
		if task == 0 || tc.Task == task {
			out = append(out, tc)
		}
	}
	return out
}
