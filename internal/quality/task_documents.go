package quality

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"github.com/z-fenix/mulix-coding/internal/layout"
)

// TaskDocument is one native task's approved input. Path is slash-separated
// and relative to the Mulix root; Markdown is the complete, unchanged file.
type TaskDocument struct {
	Number   int
	Path     string
	Markdown []byte
}

var issueFilename = regexp.MustCompile(`^([0-9]{2,})-[A-Za-z0-9]+(?:[-_][A-Za-z0-9]+)*\.md$`)
var taskHeading = regexp.MustCompile(`^ {0,3}#{1,6}[ \t]+Task(?:[ \t]+|$)`)
var issueTaskHeading = regexp.MustCompile(`^ {0,3}##[ \t]+Task[ \t]+([1-9][0-9]*):[ \t]+\S.*$`)

// ReadTaskDocuments returns the whole validated native plan in numeric order.
// It checks the same acceptance contract and suite mapping the runner uses;
// callers never receive a partial set or documents from another change.
func ReadTaskDocuments(p Project) ([]TaskDocument, error) {
	docs, _, err := readNativeContract(p)
	return docs, err
}

// ReadTaskSnapshotInputs reads the numbered native task files without parsing
// the acceptance contract or policy. Tracker sync compares these exact bytes
// and owning paths; local readiness and execution use ReadTaskDocuments.
func ReadTaskSnapshotInputs(p Project) ([]TaskDocument, error) {
	if p.Change == "" || p.Change == "." || p.Change == ".." || p.Change == layout.Shared || strings.ContainsAny(p.Change, "/\\:\x00") {
		return nil, fmt.Errorf("invalid native change identity %q", p.Change)
	}
	want := layout.ChangeRuntimePath(p.Change, layout.RuntimeIssuesDir)
	if p.TasksDir != want || p.TasksPath != "" {
		return nil, fmt.Errorf("task directory must be the change's own %s", want)
	}
	// ControlPath expects a file as the final component. Probe the directory
	// as a parent without creating anything or following a linked component.
	probe, err := fileutil.ControlPath(p.Root, p.TasksDir+"/.probe", false)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Dir(probe))
	if err != nil {
		return nil, err
	}

	var docs []TaskDocument
	seen := make(map[int]bool)
	for _, entry := range entries {
		name := entry.Name()
		// Only direct, visible Markdown files are plan inputs; editor backups
		// and other non-Markdown output never enter the contract.
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		m := issueFilename.FindStringSubmatch(name)
		if m == nil {
			return nil, fmt.Errorf("invalid task filename %q; use NN-short-name.md", name)
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid task filename %q; number must be a positive integer", name)
		}
		if seen[n] {
			return nil, fmt.Errorf("duplicate task number %d in %s", n, p.TasksDir)
		}
		seen[n] = true
		rel := p.TasksDir + "/" + name
		md, err := ReadProjectFile(p.Root, rel)
		if err != nil {
			return nil, err
		}
		docs = append(docs, TaskDocument{Number: n, Path: rel, Markdown: md})
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no task documents in %s; write at least 01-short-name.md", p.TasksDir)
	}
	slices.SortFunc(docs, func(a, b TaskDocument) int { return a.Number - b.Number })
	return docs, nil
}

func readNativeContract(p Project) ([]TaskDocument, Contract, error) {
	var c Contract
	docs, err := ReadTaskSnapshotInputs(p)
	if err != nil {
		return nil, c, err
	}
	if p.SpecPath != layout.ChangeRuntimePath(p.Change, layout.RuntimeSpecFile) {
		return nil, c, fmt.Errorf("acceptance spec must use the standard path for change %q", p.Change)
	}
	numbers := make([]int, 0, len(docs))
	for _, d := range docs {
		if err := mergeTaskDocument(&c, d); err != nil {
			return nil, c, fmt.Errorf("%s: %w", d.Path, err)
		}
		numbers = append(numbers, d.Number)
	}
	spec, err := ReadProjectFile(p.Root, p.SpecPath)
	if err != nil {
		return nil, c, err
	}
	c.Criteria, err = validateSpec(spec)
	if err != nil {
		return nil, c, err
	}
	c, err = validateContract(p, c, numbers)
	if err != nil {
		return nil, c, err
	}
	policyData, err := ReadProjectFile(p.Root, PolicyPath)
	if err != nil {
		return nil, c, fmt.Errorf("quality policy: %w", err)
	}
	policy, err := parsePolicy(p.Root, policyData)
	if err != nil {
		return nil, c, err
	}
	if err := validateMapping(c, policy, true); err != nil {
		return nil, c, err
	}
	c.native = versionNativeContract(p, spec, docs, policyData, policy)
	return docs, c, nil
}

func mergeTaskDocument(c *Contract, d TaskDocument) error {
	count := 0
	matching := false
	for line := range strings.SplitSeq(proseOnly(string(d.Markdown)), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if taskHeading.MatchString(line) {
			count++
			m := issueTaskHeading.FindStringSubmatch(line)
			matching = m != nil && m[1] == strconv.Itoa(d.Number)
		}
	}
	if count != 1 || !matching {
		return fmt.Errorf("expected exactly one Task %d heading (## Task %d: <title>) outside fences", d.Number, d.Number)
	}
	var matrix testMatrix
	if err := Block(d.Markdown, "mulix-tests", &matrix); err != nil {
		return err
	}
	if len(matrix.Tasks) != 1 || matrix.Tasks[0].Number != d.Number {
		return fmt.Errorf("mulix-tests must declare exactly task %d", d.Number)
	}
	for _, tc := range matrix.Cases {
		if tc.Task != d.Number {
			return fmt.Errorf("case %s has invalid task ownership: task %d must own its cases", tc.ID, d.Number)
		}
	}
	c.Tasks = append(c.Tasks, matrix.Tasks...)
	c.Cases = append(c.Cases, matrix.Cases...)
	c.VerifyCases = append(c.VerifyCases, matrix.VerifyCases...)
	return nil
}
