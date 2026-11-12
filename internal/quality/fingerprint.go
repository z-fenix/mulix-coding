package quality

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Snapshot includes physical project identity: copying a receipt into another
// checkout is not evidence that commands ran there.
type Snapshot struct {
	Project  string `yaml:"project" json:"project"`
	Change   string `yaml:"change" json:"change"`
	Contract string `yaml:"contract" json:"contract"`
	Source   string `yaml:"source" json:"source"`
	Tests    string `yaml:"tests" json:"tests"`
	Revision string `yaml:"revision,omitempty" json:"revision,omitempty"`
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
	for _, prefix := range []string{".git", ".superpowers", ".idea", ".mulix/.runtime", ".mulix/.locks", ".mulix/sessions", ".mulix/.installed"} {
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
func hashFiles(p Project, paths []string, allowMissing bool) (string, error) {
	slices.Sort(paths)
	paths = slices.Compact(paths)
	h := sha256.New()
	for _, rel := range paths {
		if rel == "" || excluded(p, rel) {
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
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
	paths, rev, err := sourcePaths(p)
	if err != nil {
		return s, err
	}
	s.Revision = rev
	s.Source, err = hashFiles(p, paths, true)
	if err != nil {
		return s, err
	}
	var tests []string
	for _, tc := range c.SelectedCases(task) {
		if excluded(p, tc.File) {
			return s, fmt.Errorf("mapped test input is excluded workflow output: %s", tc.File)
		}
		tests = append(tests, tc.File)
	}
	s.Tests, err = hashFiles(p, tests, false)
	return s, err
}
