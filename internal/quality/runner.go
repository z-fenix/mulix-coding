package quality

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

type TestResult struct {
	Name   string `yaml:"name" json:"name"`
	Status string `yaml:"status" json:"status"`
}
type SuiteResult struct {
	ID           string       `yaml:"id" json:"id"`
	Command      []string     `yaml:"command" json:"command"`
	CWD          string       `yaml:"cwd" json:"cwd"`
	Format       string       `yaml:"format" json:"format"`
	ExitCode     int          `yaml:"exit_code" json:"exit_code"`
	Passed       int          `yaml:"passed" json:"passed"`
	Failed       int          `yaml:"failed" json:"failed"`
	Skipped      int          `yaml:"skipped" json:"skipped"`
	Tests        []TestResult `yaml:"tests" json:"tests"`
	StdoutDigest string       `yaml:"stdout_sha256" json:"stdout_sha256"`
	StderrDigest string       `yaml:"stderr_sha256" json:"stderr_sha256"`
	Output       string       `yaml:"output,omitempty" json:"output,omitempty"`
	Error        string       `yaml:"error,omitempty" json:"error,omitempty"`
}
type RunRecord struct {
	ID                string        `yaml:"id" json:"id"`
	Task              int           `yaml:"task" json:"task"`
	Stage             string        `yaml:"stage" json:"stage"`
	Snapshot          Snapshot      `yaml:"snapshot" json:"snapshot"`
	Started           time.Time     `yaml:"started" json:"started"`
	Finished          time.Time     `yaml:"finished" json:"finished"`
	Executor          string        `yaml:"executor" json:"executor"`
	EnvironmentDigest string        `yaml:"environment_sha256" json:"environment_sha256"`
	Suites            []SuiteResult `yaml:"suites" json:"suites"`
	Success           bool          `yaml:"success" json:"success"`
	Error             string        `yaml:"error,omitempty" json:"error,omitempty"`
	ReportDigest      string        `yaml:"report_sha256,omitempty" json:"report_sha256,omitempty"`
}

const outputLimit = 16 << 20

var regexpSuiteID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type boundedFile struct {
	file *os.File
	left int
	err  error
}

func (w *boundedFile) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if len(p) > w.left {
		w.err = fmt.Errorf("test output exceeds %d bytes", outputLimit)
		return 0, w.err
	}
	n, err := w.file.Write(p)
	w.left -= n
	w.err = err
	return n, err
}

func parseResults(data []byte, format string) ([]TestResult, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	scanner.Buffer(make([]byte, 4096), 4<<20)
	results := map[string]string{}
	started := map[string]bool{}
	failedPackages := map[string]bool{}
	failedTestPackages := map[string]bool{}
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var name, status string
		if format == "go-json" {
			var e struct{ Action, Package, Test, Output string }
			if err := json.Unmarshal(line, &e); err != nil {
				return nil, fmt.Errorf("invalid go test JSON: %w", err)
			}
			if e.Action == "" || e.Package == "" {
				return nil, fmt.Errorf("incomplete go test event")
			}
			if e.Test == "" {
				if strings.Contains(e.Output, "\t(cached)") {
					return nil, fmt.Errorf("cached test results are not fresh execution; configure go test -count=1")
				}
				if e.Action == "fail" {
					failedPackages[e.Package] = true
				}
				continue
			}
			name = e.Package + "#" + e.Test
			switch e.Action {
			case "run":
				if started[name] {
					return nil, fmt.Errorf("duplicate test start %s (disable repeated runs)", name)
				}
				started[name] = true
				continue
			case "pass", "fail", "skip":
				status = e.Action
				if status == "fail" {
					failedTestPackages[e.Package] = true
				}
				if !started[name] {
					return nil, fmt.Errorf("test %s finished without a run event", name)
				}
			default:
				continue
			}
		} else {
			var e TestResult
			if err := json.Unmarshal(line, &e); err != nil {
				return nil, fmt.Errorf("invalid test JSON: %w", err)
			}
			name = e.Name
			status = e.Status
		}
		if name == "" || (status != "pass" && status != "fail" && status != "skip") {
			return nil, fmt.Errorf("invalid test result identity/status")
		}
		if _, ok := results[name]; ok {
			return nil, fmt.Errorf("duplicate test result %s (disable cached/repeated runs)", name)
		}
		if len(results) >= 20000 {
			return nil, fmt.Errorf("too many test results in one suite")
		}
		results[name] = status
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	var out []TestResult
	for name, status := range results {
		out = append(out, TestResult{Name: name, Status: status})
	}
	for pkg := range failedPackages {
		if !failedTestPackages[pkg] {
			return nil, fmt.Errorf("package/build failure without an executed failing test: %s", pkg)
		}
	}
	for name := range started {
		if _, ok := results[name]; !ok {
			return nil, fmt.Errorf("test %s did not finish", name)
		}
	}
	slices.SortFunc(out, func(a, b TestResult) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}
func executeSuite(ctx context.Context, p Project, s Suite, scratch, runID string) SuiteResult {
	r := SuiteResult{ID: s.ID, Command: append([]string(nil), s.Command...), CWD: s.CWD, Format: s.Format, ExitCode: -1}
	fail := func(err error) SuiteResult { r.Error = err.Error(); return r }
	cwd, err := SafeDir(p.Root, s.CWD)
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(scratch, 0700); err != nil {
		return fail(err)
	}
	stdout, err := os.OpenFile(filepath.Join(scratch, runID+"-"+s.ID+"-stdout.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fail(err)
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(filepath.Join(scratch, runID+"-"+s.ID+"-stderr.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fail(err)
	}
	defer stderr.Close()
	seconds := s.TimeoutSeconds
	if seconds == 0 {
		seconds = 300
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, s.Command[0], s.Command[1:]...)
	cmd.Dir = cwd
	cmd.WaitDelay = 2 * time.Second
	stdoutCapture := &boundedFile{file: stdout, left: outputLimit}
	stderrCapture := &boundedFile{file: stderr, left: outputLimit}
	cmd.Stdout = stdoutCapture
	cmd.Stderr = stderrCapture
	runErr := cmd.Run()
	if cmd.ProcessState != nil {
		r.ExitCode = cmd.ProcessState.ExitCode()
	}
	// Wait may prefer an ExitError over an I/O-copy error. Capture failures
	// remain fatal even when a failing test also exited nonzero.
	if stdoutCapture.err != nil {
		return fail(fmt.Errorf("stdout capture: %w", stdoutCapture.err))
	}
	if stderrCapture.err != nil {
		return fail(fmt.Errorf("stderr capture: %w", stderrCapture.err))
	}
	if runCtx.Err() != nil {
		return fail(fmt.Errorf("suite %s cancelled or timed out: %w", s.ID, runCtx.Err()))
	}
	if runErr != nil {
		if _, ok := runErr.(*exec.ExitError); !ok {
			return fail(runErr)
		}
	}
	if err := stdout.Close(); err != nil {
		return fail(err)
	}
	if err := stderr.Close(); err != nil {
		return fail(err)
	}
	out, err := os.ReadFile(stdout.Name())
	if err != nil {
		return fail(err)
	}
	errOut, err := os.ReadFile(stderr.Name())
	if err != nil {
		return fail(err)
	}
	r.StdoutDigest = Digest(out)
	r.StderrDigest = Digest(errOut)
	// Keep a bounded excerpt as well as stream digests in the retained record.
	excerpt := append(append([]byte(nil), out...), errOut...)
	if len(excerpt) > 4096 {
		excerpt = excerpt[len(excerpt)-4096:]
	}
	r.Output = string(excerpt)
	r.Tests, err = parseResults(out, s.Format)
	if err != nil {
		return fail(err)
	}
	for _, test := range r.Tests {
		switch test.Status {
		case "pass":
			r.Passed++
		case "fail":
			r.Failed++
		case "skip":
			r.Skipped++
		}
	}
	if len(r.Tests) == 0 || r.Passed+r.Failed == 0 {
		return fail(fmt.Errorf("suite %s executed no non-skipped tests", s.ID))
	}
	return r
}
func validateStage(c Contract, task int, stage string) error {
	if task == 0 {
		if stage != "verify" {
			return fmt.Errorf("task 0 is only valid for verification")
		}
		return nil
	}
	t, ok := c.Task(task)
	if !ok {
		return fmt.Errorf("unknown task %d", task)
	}
	if t.Kind == "code" {
		if stage != "red" && stage != "green" {
			return fmt.Errorf("code tasks require red/green stages")
		}
	} else if stage != "check" {
		return fmt.Errorf("non-code tasks require check, not invented RED/GREEN")
	}
	return nil
}
func ValidateMapping(c Contract, p Policy) error {
	known := map[string]bool{}
	for _, s := range p.Suites {
		if !regexpSuiteID.MatchString(s.ID) || known[s.ID] || len(s.Command) == 0 || s.Command[0] == "" || (s.Format != "go-json" && s.Format != "json") {
			return fmt.Errorf("invalid suite %q", s.ID)
		}
		known[s.ID] = true
	}
	for _, tc := range c.Cases {
		if !known[tc.Suite] {
			return fmt.Errorf("case %s references unknown suite %s", tc.ID, tc.Suite)
		}
	}
	return nil
}
func outcome(c Contract, r RunRecord) error {
	if r.ID == "" || r.Started.IsZero() || r.Finished.Before(r.Started) || len(r.Suites) == 0 {
		return fmt.Errorf("incomplete execution receipt")
	}
	results := map[string]map[string]string{}
	mappedFailure := false
	for _, suite := range r.Suites {
		if suite.Error != "" {
			return fmt.Errorf("%s: %s", suite.ID, suite.Error)
		}
		if _, ok := results[suite.ID]; ok {
			return fmt.Errorf("duplicate suite receipt %s", suite.ID)
		}
		statuses := map[string]string{}
		passed, failed, skipped := 0, 0, 0
		for _, test := range suite.Tests {
			if _, ok := statuses[test.Name]; ok {
				return fmt.Errorf("duplicate test receipt")
			}
			statuses[test.Name] = test.Status
			switch test.Status {
			case "pass":
				passed++
			case "fail":
				failed++
			case "skip":
				skipped++
			default:
				return fmt.Errorf("invalid test status")
			}
		}
		if passed+failed == 0 || passed != suite.Passed || failed != suite.Failed || skipped != suite.Skipped {
			return fmt.Errorf("suite %s has no tests or inconsistent counts", suite.ID)
		}
		if r.Stage == "red" {
			if (suite.ExitCode == 0 && failed > 0) || (suite.ExitCode != 0 && failed == 0) || suite.ExitCode < 0 {
				return fmt.Errorf("suite %s has an infrastructure/exit mismatch, not a test RED", suite.ID)
			}
		} else if suite.ExitCode != 0 || failed > 0 {
			return fmt.Errorf("suite %s failed (exit %d)", suite.ID, suite.ExitCode)
		}
		results[suite.ID] = statuses
	}
	for _, tc := range c.SelectedCases(r.Task) {
		status := results[tc.Suite][tc.Test]
		if status == "" || status == "skip" {
			return fmt.Errorf("mandatory case %s (%s) was absent or skipped", tc.ID, tc.Test)
		}
		if status == "fail" {
			mappedFailure = true
			if r.Stage != "red" {
				return fmt.Errorf("case %s failed", tc.ID)
			}
		}
	}
	if r.Stage == "red" && !mappedFailure {
		return fmt.Errorf("RED requires an actually failing mapped test")
	}
	return nil
}
func Run(ctx context.Context, p Project, c Contract, policy Policy, task int, stage, scratch string) (record RunRecord, err error) {
	record.Task = task
	record.Stage = stage
	record.Started = time.Now().UTC()
	record.Executor = runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH
	env := os.Environ()
	slices.Sort(env)
	record.EnvironmentDigest = Digest([]byte(strings.Join(env, "\x00")))
	defer func() {
		record.Finished = time.Now().UTC()
		if err != nil {
			record.Error = err.Error()
			record.Success = false
		}
	}()
	var id [16]byte
	if _, err = io.ReadFull(rand.Reader, id[:]); err != nil {
		return record, err
	}
	record.ID = hex.EncodeToString(id[:])
	if err = validateStage(c, task, stage); err != nil {
		return record, err
	}
	actualPolicy, e := LoadPolicy(p.Root)
	if e != nil {
		return record, e
	}
	if !reflect.DeepEqual(actualPolicy, policy) {
		return record, fmt.Errorf("execution policy does not match .mulix/quality.yaml")
	}
	if err = ValidateMapping(c, policy); err != nil {
		return record, err
	}
	record.Snapshot, err = Fingerprint(p, c, task)
	if err != nil {
		return record, err
	}
	selected := map[string]bool{}
	for _, tc := range c.SelectedCases(task) {
		selected[tc.Suite] = true
	}
	for _, suite := range policy.Suites {
		if task != 0 && !selected[suite.ID] {
			continue
		}
		record.Suites = append(record.Suites, executeSuite(ctx, p, suite, scratch, record.ID))
		if err = ctx.Err(); err != nil {
			return record, err
		}
	}
	after, e := Fingerprint(p, c, task)
	if e != nil {
		return record, e
	}
	if after != record.Snapshot {
		return record, fmt.Errorf("inputs changed during test execution; no successful evidence recorded")
	}
	record.Finished = time.Now().UTC()
	if err = outcome(c, record); err != nil {
		return record, err
	}
	if err = ctx.Err(); err != nil {
		return record, err
	}
	record.Success = true
	return record, nil
}
