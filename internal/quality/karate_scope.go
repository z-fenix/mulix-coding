package quality

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/z-fenix/mulix-coding/internal/fileutil"
	"github.com/z-fenix/mulix-coding/internal/layout"
)

const KarateFormat = "karate-jsonl"
const KarateVersion = "2.1.5.RC1"

// KaratePolicy is typed because paths, reporting flags and selection are owned
// by the runner. This suite never executes user-provided argv.
type KaratePolicy struct {
	Version     string   `yaml:"version" json:"version"`
	SHA256      string   `yaml:"sha256" json:"sha256"`
	Environment string   `yaml:"environment,omitempty" json:"environment,omitempty"`
	Env         []string `yaml:"env,omitempty" json:"env,omitempty"`
}

var fullSHA256 = regexp.MustCompile(`^[a-f0-9]{64}$`)
var envKeyPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,127}$`)

func validKarateEnv(names []string) bool {
	seen := map[string]bool{}
	for _, name := range names {
		upper := strings.ToUpper(name)
		if seen[upper] || !envKeyPattern.MatchString(name) || strings.HasPrefix(upper, "KARATE_") || strings.HasPrefix(upper, "MULIX_") || strings.HasPrefix(upper, "JAVA_") {
			return false
		}
		switch upper {
		case "PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "TMPDIR", "CLASSPATH", "JDK_JAVA_OPTIONS", "_JAVA_OPTIONS":
			return false
		}
		seen[upper] = true
	}
	return true
}

func validSuite(s Suite) bool {
	if !regexpSuiteID.MatchString(s.ID) {
		return false
	}
	if s.Format != KarateFormat {
		return s.Karate == nil && (s.Format == "go-json" || s.Format == "json") && len(s.Command) > 0 && strings.TrimSpace(s.Command[0]) != ""
	}
	k := s.Karate
	return k != nil && k.Version == KarateVersion && fullSHA256.MatchString(k.SHA256) &&
		strings.Trim(k.SHA256, "0") != "" && len(s.Command) == 0 && s.CWD == "" &&
		(k.Environment == "" || regexpSuiteID.MatchString(k.Environment)) && validKarateEnv(k.Env)
}

func runtimeTestsRoot(p Project) string {
	return layout.ChangeRuntimePath(p.Change, layout.TestsDir)
}

func runtimeTestGroup(p Project, rel string) string {
	rel, ok := strings.CutPrefix(rel, runtimeTestsRoot(p)+"/")
	if !ok {
		return ""
	}
	return layout.RuntimeTestGroup(rel)
}

func karateCasePath(p Project, tc Case, final bool) bool {
	group := "task-" + strconv.Itoa(tc.Task)
	if final {
		group = "final"
	}
	scenario, hasPath := strings.CutPrefix(tc.Test, tc.File+"#")
	return runtimeTestGroup(p, tc.File) == group && hasPath && strings.TrimSpace(scenario) != ""
}

func suiteSelected(c Contract, id string, task int) bool {
	if task == 0 {
		return true
	}
	for _, tc := range c.SelectedCases(task) {
		if tc.Suite == id {
			return true
		}
	}
	return false
}

type karateSelection struct {
	Features []string // project-relative, not scratch paths
	Helpers  []string
}

// karateInputs is shared by fingerprinting and execution. A task selects
// its mapped entry files; final validation discovers all legal entries and
// checks that each approved case is in that set. Helpers are never entries.
func karateInputs(p Project, c Contract, suite Suite, task int) (karateSelection, error) {
	var out karateSelection
	base := runtimeTestsRoot(p)
	collect := func(sub string, optional bool) error {
		dir := path.Join(base, sub)
		probe, err := fileutil.ControlPath(p.Root, path.Join(dir, ".mulix-probe"), false)
		if err != nil {
			return err
		}
		physical := filepath.Dir(probe)
		return filepath.WalkDir(physical, func(full string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if optional && full == physical && os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			inside, err := filepath.Rel(physical, full)
			if err != nil {
				return err
			}
			rel := path.Join(dir, filepath.ToSlash(inside))
			group := runtimeTestGroup(p, rel)
			if group == "" || !d.Type().IsRegular() {
				return fmt.Errorf("Karate tests must contain only regular authored sources: %s", rel)
			}
			if _, err := fileutil.ControlPath(p.Root, rel, false); err != nil {
				return err
			}
			if group == "helpers" {
				out.Helpers = append(out.Helpers, rel)
			} else {
				out.Features = append(out.Features, rel)
			}
			return nil
		})
	}
	if task == 0 {
		if err := collect("", false); err != nil {
			return out, err
		}
	} else if err := collect("helpers", true); err != nil {
		return out, err
	}
	for _, tc := range c.SelectedCases(task) {
		if tc.Suite != suite.ID {
			continue
		}
		if !karateCasePath(p, tc, tc.Task == 0) {
			return out, fmt.Errorf("Karate case %s must name an owned feature and exact path#scenario", tc.ID)
		}
		if task == 0 {
			if !slices.Contains(out.Features, tc.File) {
				return out, fmt.Errorf("Karate case %s has no owned feature file: %s", tc.ID, tc.File)
			}
			continue
		}
		full, err := fileutil.ControlPath(p.Root, tc.File, false)
		if err != nil {
			return out, err
		}
		if info, err := os.Lstat(full); err != nil || !info.Mode().IsRegular() {
			return out, fmt.Errorf("Karate case %s needs an existing regular feature: %s", tc.ID, tc.File)
		}
		out.Features = append(out.Features, tc.File)
	}
	slices.Sort(out.Features)
	out.Features = slices.Compact(out.Features)
	slices.Sort(out.Helpers)
	if len(out.Features) == 0 {
		return out, fmt.Errorf("Karate suite %s has no feature entries in this scope", suite.ID)
	}
	return out, nil
}
