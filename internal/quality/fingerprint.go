package quality

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
)

// Snapshot includes physical project identity: copying a receipt into another
// checkout is not evidence that commands ran there.
type Snapshot struct {
	Project       string `yaml:"project" json:"project"`
	Change        string `yaml:"change" json:"change"`
	Contract      string `yaml:"contract" json:"contract"`
	Source        string `yaml:"source" json:"source"`
	Tests         string `yaml:"tests" json:"tests"`
	Revision      string `yaml:"revision,omitempty" json:"revision,omitempty"`
	Karate        string `yaml:"karate,omitempty" json:"karate,omitempty"`
	KarateSources string `yaml:"karate_sources,omitempty" json:"karate_sources,omitempty"`
}

func Digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func normalizedPlan(data []byte) []byte {
	re := regexp.MustCompile(`^(- )\[[ X]\]( \*\*Step [0-9]+:.*)$`)
	var out []string
	fence := ""
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		t := strings.TrimSpace(line)
		if fence == "" && fenceStart(t) != "" {
			fence = fenceStart(t)
		} else if fence != "" && fenceEnd(t, fence) {
			fence = ""
		} else if fence == "" {
			line = re.ReplaceAllString(line, "${1}[ ]${2}")
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}
func excluded(p Project, rel string) bool {
	if rel == p.SpecPath || rel == p.TasksPath || rel == "docs/changes/"+p.Change+"/report.md" {
		return true
	}
	// Runtime output stays out of the implementation hash, even on platforms
	// that resolve case aliases to the same directory.
	if lower := strings.ToLower(rel); lower == ".mulix/.runtime" || strings.HasPrefix(lower, ".mulix/.runtime/") {
		return true
	}
	for _, prefix := range []string{".git", ".superpowers", ".idea", ".mulix/.locks", ".mulix/sessions", ".mulix/.installed"} {
		if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
			return true
		}
	}
	return rel == ".mulix/context.json" || rel == ".mulix/active" || rel == ".mulix/install-state.json"
}
func sourcePaths(p Project) ([]string, string, error) {
	cmd := exec.Command("git", "-C", p.Root, "rev-parse", "--is-inside-work-tree")
	if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == "true" {
		cmd = exec.Command("git", "-C", p.Root, "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ".")
		out, err := cmd.Output()
		if err != nil {
			return nil, "", err
		}
		rev, _ := exec.Command("git", "-C", p.Root, "rev-parse", "--verify", "HEAD").Output()
		return strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00"), strings.TrimSpace(string(rev)), nil
	}
	var paths []string
	err := filepath.WalkDir(p.Root, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(p.Root, full)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if excluded(p, rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			paths = append(paths, rel)
		}
		return nil
	})
	return paths, "", err
}
func stampKarateSource(h io.Writer, rel string, data []byte) {
	// Bind pathname and length as well as contents; the staged copies have
	// different filesystem modes from their sources by design.
	fmt.Fprintf(h, "%s\x00%d\x00", rel, len(data))
	h.Write(data)
	h.Write([]byte{0})
}

func hashFiles(p Project, paths []string, allowMissing bool, karate io.Writer) (string, error) {
	slices.Sort(paths)
	paths = slices.Compact(paths)
	h := sha256.New()
	for _, rel := range paths {
		runtimeTest := karate != nil && runtimeTestGroup(p, rel) != ""
		if rel == "" || (excluded(p, rel) && !runtimeTest) {
			continue
		}
		if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, "../") {
			return "", fmt.Errorf("source path escapes project: %q", rel)
		}
		info, err := os.Lstat(filepath.Join(p.Root, filepath.FromSlash(rel)))
		if os.IsNotExist(err) && allowMissing {
			fmt.Fprintf(h, "%s\x00deleted\x00", rel)
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("source input must be a regular non-linked file: %s", rel)
		}
		data, err := ReadProjectFile(p.Root, rel)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%o\x00%d\x00", rel, info.Mode().Perm(), len(data))
		h.Write(data)
		h.Write([]byte{0})
		if runtimeTest {
			stampKarateSource(karate, rel, data)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ArtifactInput identifies an exact file used by a native run, not a path
// resolved again when rendering a report. It is omitted from legacy receipts.
type ArtifactInput struct {
	Path   string `yaml:"path" json:"path"`
	SHA256 string `yaml:"sha256" json:"sha256"`
}

type nativeContractVersion struct {
	digest string
	inputs []ArtifactInput
	policy Policy
}

func versionNativeContract(p Project, spec []byte, docs []TaskDocument, policyData []byte, policy Policy) *nativeContractVersion {
	h := sha256.New()
	io.WriteString(h, "mulix.runtime-issues-contract.v1\x00")
	v := &nativeContractVersion{policy: policy}
	frame := func(rel string, data []byte) {
		// Length-frame both fields: moving a file or changing any Markdown
		// byte changes the contract, independently of the source fingerprint.
		fmt.Fprintf(h, "%d:%s%d:", len(rel), rel, len(data))
		h.Write(data)
		v.inputs = append(v.inputs, ArtifactInput{Path: rel, SHA256: Digest(data)})
	}
	frame(p.SpecPath, spec)
	for _, d := range docs {
		frame(d.Path, d.Markdown)
	}
	frame(PolicyPath, policyData)
	v.digest = hex.EncodeToString(h.Sum(nil))
	return v
}

func Fingerprint(p Project, c Contract, task int) (Snapshot, error) {
	var s Snapshot
	root, err := filepath.Abs(p.Root)
	if err != nil {
		return s, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return s, err
	}
	s.Project = Digest([]byte(root))
	s.Change = p.Change
	if p.TasksDir != "" {
		_, live, err := readNativeContract(p)
		if err != nil {
			return s, err
		}
		if !reflect.DeepEqual(c, live) {
			return s, fmt.Errorf("native acceptance contract changed after it was read; retry with the current inputs")
		}
		s.Contract = live.native.digest
	} else {
		spec, err := ReadProjectFile(root, p.SpecPath)
		if err != nil {
			return s, err
		}
		plan, err := ReadProjectFile(root, p.TasksPath)
		if err != nil {
			return s, err
		}
		policy, err := ReadProjectFile(root, PolicyPath)
		if err != nil {
			return s, err
		}
		s.Contract = Digest([]byte(p.SpecPath + "\x00" + string(spec) + "\x00" + p.TasksPath + "\x00" + string(normalizedPlan(plan)) + "\x00" + string(policy)))
	}
	paths, rev, err := sourcePaths(p)
	if err != nil {
		return s, err
	}
	s.Revision = rev
	s.Source, err = hashFiles(p, paths, true, nil)
	if err != nil {
		return s, err
	}
	var tests []string
	for _, tc := range c.SelectedCases(task) {
		if excluded(p, tc.File) && !karateCasePath(p, tc, tc.Task == 0) {
			return s, fmt.Errorf("mapped test input is excluded workflow output: %s", tc.File)
		}
		tests = append(tests, tc.File)
	}
	// Legacy policy lookup keeps using the physical root. Native selection
	// must use the parsed policy bound to the contract we just checked.
	project := p
	project.Root = root
	policyConfig, err := executionPolicy(project, c)
	if err != nil {
		return s, err
	}
	stagedInputs := sha256.New()
	karateBound := false
	for _, suite := range policyConfig.Suites {
		if suite.Format != KarateFormat || !suiteSelected(c, suite.ID, task) {
			continue
		}
		selection, e := karateInputs(p, c, suite, task)
		if e != nil {
			return s, e
		}
		karateBound = true
		tests = append(tests, selection.Features...)
		tests = append(tests, selection.Helpers...)
	}
	s.Tests, err = hashFiles(p, tests, false, stagedInputs)
	if karateBound && err == nil {
		s.KarateSources = hex.EncodeToString(stagedInputs.Sum(nil))
	}
	return s, err
}
