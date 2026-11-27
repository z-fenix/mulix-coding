package cliutil

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/layout"
	"github.com/z-fenix/mulix-coding/internal/lockfile"
	"github.com/z-fenix/mulix-coding/internal/quality"
	"github.com/z-fenix/mulix-coding/internal/state"
	"github.com/z-fenix/mulix-coding/internal/tracker"
)

const fakeTrackerRepoURL = "https://tracker.example.test/api/v3/repos/team/project"
const fakeTrackerSpec = "# Delivery spec\n\nFR-001: Return two.\n\n```mulix-acceptance\ncriteria:\n  - id: AC-001\n    requirement: FR-001\n    expected: Value returns two.\n```\n"

type fakeHostedIssue struct {
	ID      int      `json:"id"`
	Number  int      `json:"number"`
	NodeID  string   `json:"node_id"`
	Repo    string   `json:"repository_url"`
	Title   string   `json:"title"`
	Body    string   `json:"body"`
	Status  string   `json:"state"`
	Labels  []string `json:"labels"`
	Replies int      `json:"comments"`
}

// The fake persists independent provider state, including when a POST accepts
// the issue but the CLI process exits unsuccessfully before returning its ID.
func init() {
	dir := os.Getenv("MULIX_TRACKER_WORKFLOW_FAKE")
	if dir == "" {
		return
	}
	args := os.Args[1:]
	if len(args) < 4 || args[0] != "api" || args[2] != "--hostname" || args[3] != "tracker.example.test" {
		os.Exit(2)
	}
	logFakeTrackerCall(dir, "calls", strings.Join(args, " "))
	if os.Getenv("MULIX_TRACKER_WORKFLOW_OFFLINE") != "" {
		os.Exit(1)
	}
	gitlab := strings.HasPrefix(filepath.Base(os.Args[0]), "glab")
	project, endpoint, query := "repos/team/project", "repos/team/project/issues", "?state=all&per_page=100"
	if gitlab {
		project, endpoint, query = "projects/team%2Fproject", "projects/42/issues", "?scope=all&state=all&per_page=100"
	}
	if args[1] == project && len(args) == 4 {
		fmt.Printf(`{"id":%s,"node_id":"R_fake_project","full_name":"team/project","path_with_namespace":"team/project"}`, os.Getenv("MULIX_TRACKER_WORKFLOW_PROJECT"))
		os.Exit(0)
	}
	file := filepath.Join(dir, "issues.json")
	data, err := os.ReadFile(file)
	var issues []fakeHostedIssue
	if err != nil || json.Unmarshal(data, &issues) != nil {
		os.Exit(2)
	}
	if args[1] == endpoint+query && strings.Join(args[4:], " ") == "--paginate" {
		rows := make([]any, 0, len(issues))
		for _, issue := range issues {
			rows = append(rows, fakeIssueResponse(issue, gitlab))
		}
		_ = json.NewEncoder(os.Stdout).Encode(rows)
		os.Exit(0)
	}
	callOptions := strings.Join(args[4:], " ")
	if gitlab && args[1] == "graphql" && len(args) == 8 && args[4] == "-f" && args[6] == "-f" && strings.Contains(args[5], "workItemUpdate(input:{id:$id,stateEvent:CLOSE})") {
		fakeCloseIssue(dir, file, issues, strings.TrimPrefix(args[7], "id="), gitlab)
	}
	if (args[1] == "graphql" && !gitlab || args[1] == endpoint && gitlab) && callOptions == "--method POST --input - --header Content-Type: application/json" {
		var request struct {
			Query       string            `json:"query"`
			Variables   map[string]string `json:"variables"`
			Title       string            `json:"title"`
			Description string            `json:"description"`
		}
		if json.NewDecoder(os.Stdin).Decode(&request) != nil {
			os.Exit(2)
		}
		if !gitlab && strings.Contains(request.Query, "closeIssue(input:{issueId:$issue})") {
			fakeCloseIssue(dir, file, issues, request.Variables["issue"], gitlab)
		}
		logFakeTrackerCall(dir, "posts", "POST")
		issue := fakeHostedIssue{Title: request.Title, Body: request.Description}
		if !gitlab {
			if !strings.Contains(request.Query, "createIssue(") || request.Variables["repository"] != "R_fake_project" {
				os.Exit(2)
			}
			issue.Title, issue.Body = request.Variables["title"], request.Variables["body"]
		}
		if issue.Title == "" || issue.Body == "" {
			os.Exit(2)
		}
		// Observe the durable intent inside the child before accepting anything.
		current, err := state.Load(os.Getenv("MULIX_TRACKER_WORKFLOW_ROOT"), os.Getenv("MULIX_TRACKER_WORKFLOW_CHANGE"))
		if err != nil || current.Tracker == nil || current.Tracker.Target.ProjectID != "42" {
			os.Exit(2)
		}
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ReplaceAll(issue.Body, "\r\n", "\n"))))
		intentSaved := false
		for n, m := range current.Tracker.Issues {
			if !m.Published() && m.PublishDigest == digest && strings.HasPrefix(issue.Body, current.Tracker.Marker(n)+"\n") {
				intentSaved = true
			}
		}
		if !intentSaved {
			os.Exit(2)
		}
		issue.ID, issue.Number, issue.Repo, issue.Status = 910+len(issues), 10+len(issues), fakeTrackerRepoURL, "open"
		issue.NodeID = "I_fake_" + strconv.Itoa(issue.Number)
		listed := issue
		if os.Getenv("MULIX_TRACKER_WORKFLOW_REPLACE_RECEIPT") == strconv.Itoa(issue.Number) {
			listed.ID, listed.Number, listed.NodeID = issue.ID+1, issue.Number+1, "I_substitute"
		}
		issues = append(issues, listed)
		data, err := json.Marshal(issues)
		if err != nil || os.WriteFile(file, data, 0600) != nil {
			os.Exit(2)
		}
		if os.Getenv("MULIX_TRACKER_WORKFLOW_LOST_NUMBER") == strconv.Itoa(issue.Number) {
			fmt.Fprintln(os.Stderr, "connection lost: fake-sensitive-token")
			os.Exit(1)
		}
		if gitlab {
			_ = json.NewEncoder(os.Stdout).Encode(fakeIssueResponse(issue, true))
		} else {
			fmt.Printf(`{"data":{"createIssue":{"issue":{"id":%q,"number":%d}}}}`, issue.NodeID, issue.Number)
		}
		os.Exit(0)
	}
	for _, issue := range issues {
		if args[1] == endpoint+"/"+strconv.Itoa(issue.Number) && len(args) == 4 {
			if p := os.Getenv("MULIX_TRACKER_WORKFLOW_EDIT_ON_GET"); p != "" {
				if err := os.WriteFile(p, []byte(fakeTrackerSpec+"\n## Clarifications\n\nEditor answer.\n"), 0644); err != nil {
					os.Exit(2)
				}
			}
			_ = json.NewEncoder(os.Stdout).Encode(fakeIssueResponse(issue, gitlab))
			os.Exit(0)
		}
	}
	os.Exit(1)
}

func logFakeTrackerCall(dir, name, line string) {
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		os.Exit(2)
	}
	_, _ = fmt.Fprintln(f, line)
	_ = f.Close()
}

func fakeIssueResponse(issue fakeHostedIssue, gitlab bool) any {
	if gitlab {
		return map[string]any{"id": issue.ID, "iid": issue.Number, "project_id": 42, "title": issue.Title, "description": issue.Body, "state": issue.Status}
	}
	return issue
}

func fakeCloseIssue(dir, file string, issues []fakeHostedIssue, id string, gitlab bool) {
	logFakeTrackerCall(dir, "closes", id)
	if os.Getenv("MULIX_TRACKER_WORKFLOW_CLOSE_FAIL") != "" {
		fmt.Fprintln(os.Stderr, "close failed: fake-sensitive-token")
		os.Exit(1)
	}
	for i, issue := range issues {
		if !gitlab && id != issue.NodeID || gitlab && id != "gid://gitlab/WorkItem/"+strconv.Itoa(issue.ID) {
			continue
		}
		issues[i].Status = "closed"
		data, err := json.Marshal(issues)
		if err != nil || os.WriteFile(file, data, 0600) != nil {
			os.Exit(2)
		}
		if gitlab {
			fmt.Printf(`{"data":{"workItemUpdate":{"workItem":{"id":%q,"iid":%q,"state":"CLOSED"},"errors":[]}}}`, id, strconv.Itoa(issue.Number))
		} else {
			fmt.Printf(`{"data":{"closeIssue":{"issue":{"id":%q,"number":%d,"state":"CLOSED"}}}}`, id, issue.Number)
		}
		os.Exit(0)
	}
	os.Exit(2)
}

type trackerFixture struct {
	t    *testing.T
	root string
	fake string
	s    flow.State
}

func newTrackerFixture(t *testing.T) trackerFixture {
	t.Helper()
	return newTrackerFixtureFor(t, "github")
}

func newTrackerFixtureFor(t *testing.T, kind string) trackerFixture {
	t.Helper()
	f := fakeTrackerProject(t)
	s := flow.New("delivery", "")
	t.Setenv("MULIX_TRACKER_WORKFLOW_CHANGE", s.Change)
	s.ArtifactLayout = flow.RuntimeIssuesLayout
	s.SpecPath = layout.ChangeRuntimePath(s.Change, layout.RuntimeSpecFile)
	s.TasksDir = layout.ChangeRuntimePath(s.Change, layout.RuntimeIssuesDir)
	s.ReportPath = layout.ChangeRuntimePath(s.Change, layout.RuntimeReportFile)
	var err error
	s.Tracker, err = tracker.NewChangeBinding(tracker.Target{Kind: kind, Host: "tracker.example.test", Project: "team/project"})
	if err != nil {
		t.Fatal(err)
	}
	saveStateForTest(t, f.root, s)
	if err := state.SetActive(f.root, s.Change); err != nil {
		t.Fatal(err)
	}
	qualityCLIFile(t, f.root, s.SpecPath, fakeTrackerSpec)
	f.s = s
	return f
}

func fakeTrackerProject(t *testing.T) trackerFixture {
	t.Helper()
	root := chdirTemp(t)
	initMulixRoot(t, root)
	originalPath := os.Getenv("PATH")
	setupFakeGH(t)
	dir := os.Getenv("PATH")
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	qualityCLIFile(t, dir, "glab"+suffix, trackerRead(t, filepath.Join(dir, "gh"+suffix)))
	if err := os.Chmod(filepath.Join(dir, "glab"+suffix), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+originalPath)
	t.Setenv("MULIX_SETUP_TEST_HELPER", "")
	t.Setenv("MULIX_SESSION_ID", "tracker-test")
	fake := t.TempDir()
	qualityCLIFile(t, fake, "issues.json", "[]")
	t.Setenv("MULIX_TRACKER_WORKFLOW_FAKE", fake)
	t.Setenv("MULIX_TRACKER_WORKFLOW_PROJECT", "42")
	t.Setenv("MULIX_TRACKER_WORKFLOW_ROOT", root)
	return trackerFixture{t: t, root: root, fake: fake}
}

func (f trackerFixture) run(args ...string) (string, error) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(append([]string{"tracker"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func (f trackerFixture) success(args ...string) string {
	f.t.Helper()
	out, err := f.run(args...)
	if err != nil {
		f.t.Fatalf("tracker %v: %v\n%s", args, err, out)
	}
	return out
}

var trackerTestToken = regexp.MustCompile(`(?m)^Token: ([a-f0-9]{64})$`)

func (f trackerFixture) token(args ...string) string {
	f.t.Helper()
	out := f.success(args...)
	m := trackerTestToken.FindStringSubmatch(out)
	if m == nil {
		f.t.Fatalf("tracker %v did not show an approval token:\n%s", args, out)
	}
	return m[1]
}

func (f trackerFixture) publishSpec() {
	f.t.Helper()
	f.success("publish", "spec", "--expect", f.token("publish", "spec", "--dry-run"))
}

func trackerRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (f trackerFixture) posts() int {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.fake, "posts"))
	if err != nil && !os.IsNotExist(err) {
		f.t.Fatal(err)
	}
	return bytes.Count(data, []byte("POST\n"))
}

func (f trackerFixture) remote() []fakeHostedIssue {
	f.t.Helper()
	var issues []fakeHostedIssue
	if err := json.Unmarshal([]byte(trackerRead(f.t, filepath.Join(f.fake, "issues.json"))), &issues); err != nil {
		f.t.Fatal(err)
	}
	return issues
}

func (f trackerFixture) setRemote(issues []fakeHostedIssue) {
	f.t.Helper()
	b, err := json.Marshal(issues)
	if err != nil {
		f.t.Fatal(err)
	}
	qualityCLIFile(f.t, f.fake, "issues.json", string(b))
}

func TestPublishRequiresExactApproval(t *testing.T) {
	f := newTrackerFixture(t)
	before := trackerRead(t, state.PathFor(f.root, f.s.Change))
	if out, err := f.run("publish", "spec"); err == nil || f.posts() != 0 {
		t.Fatalf("publication without approval: %v %q, POSTs: %d", err, out, f.posts())
	}
	preview := f.success("publish", "--dry-run")
	m := trackerTestToken.FindStringSubmatch(preview)
	if m == nil || !strings.Contains(preview, "tracker.example.test") || !strings.Contains(preview, "team/project") || !strings.Contains(preview, "expected: Value returns two.") {
		t.Fatalf("dry run omitted target, body or token:\n%s", preview)
	}
	if f.posts() != 0 || trackerRead(t, state.PathFor(f.root, f.s.Change)) != before {
		t.Fatal("dry run made a publication side effect")
	}
	// Changing even a trailing content byte must not authorize a POST.
	qualityCLIFile(t, f.root, f.s.SpecPath, fakeTrackerSpec+"\n")
	if _, err := f.run("publish", "spec", "--expect", m[1]); err == nil || f.posts() != 0 {
		t.Fatalf("changed draft authorized publication: %v, POSTs: %d", err, f.posts())
	}
	// Target ID from the live dry run also participates in approval, including
	// when the selector exists but its ID has not been pinned in state yet.
	qualityCLIFile(t, f.root, f.s.SpecPath, fakeTrackerSpec)
	t.Setenv("MULIX_TRACKER_WORKFLOW_PROJECT", "77")
	if _, err := f.run("publish", "spec", "--expect", m[1]); err == nil || f.posts() != 0 || trackerRead(t, state.PathFor(f.root, f.s.Change)) != before {
		t.Fatalf("changed pending target authorized publication: %v, POSTs: %d", err, f.posts())
	}
	t.Setenv("MULIX_TRACKER_WORKFLOW_PROJECT", "42")
	qualityCLIFile(t, f.root, f.s.SpecPath, strings.ReplaceAll(fakeTrackerSpec, "\n", "\r\n"))
	f.success("publish", "spec", "--expect", m[1])
	if got := f.remote(); len(got) != 1 || !strings.HasSuffix(strings.ReplaceAll(got[0].Body, "\r\n", "\n"), fakeTrackerSpec) {
		t.Fatalf("approved publication did not round trip the draft: %+v", got)
	}
}

func TestPublishRecoversCreatedIssueWithoutDuplicate(t *testing.T) {
	f := newTrackerFixture(t)
	token := f.token("publish", "spec", "--dry-run")
	t.Setenv("MULIX_TRACKER_WORKFLOW_LOST_NUMBER", "10")
	if out, err := f.run("publish", "spec", "--expect", token); err == nil || strings.Contains(fmt.Sprint(err)+out, "fake-sensitive-token") {
		t.Fatalf("expected a sanitized interrupted-create error: %v %q", err, out)
	}
	got := f.remote()
	if len(got) != 1 || f.posts() != 1 {
		t.Fatalf("fake server did not accept exactly one create: %+v, POSTs: %d", got, f.posts())
	}
	pinned, err := state.Load(f.root, f.s.Change)
	if err != nil || pinned.Tracker == nil || pinned.Tracker.Target.ProjectID != "42" {
		t.Fatalf("project ID was not persisted on interruption: %+v, %v", pinned.Tracker, err)
	}
	if _, err := f.run("preview", "spec"); err == nil {
		t.Fatal("an uncertain publication was exposed as bound")
	}
	before := trackerRead(t, state.PathFor(f.root, f.s.Change))
	duplicate := got[0]
	duplicate.ID, duplicate.Number = 999, 99
	wrongBody := got[0]
	wrongBody.Body += "\nAn unapproved change.\n"
	wrongRepo := got[0]
	wrongRepo.Repo = "https://tracker.example.test/api/v3/repos/team/other"
	for _, tc := range []struct {
		name string
		list []fakeHostedIssue
	}{
		{"zero matches", []fakeHostedIssue{}},
		{"multiple matches", []fakeHostedIssue{got[0], duplicate}},
		{"changed body", []fakeHostedIssue{wrongBody}},
		{"wrong project", []fakeHostedIssue{wrongRepo}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.setRemote(tc.list)
			if _, err := f.run("publish", "spec", "--dry-run"); err == nil {
				t.Fatal("preview offered unsafe recovery")
			}
			if _, err := f.run("publish", "spec", "--expect", token); err == nil || f.posts() != 1 || trackerRead(t, state.PathFor(f.root, f.s.Change)) != before {
				t.Fatalf("unsafe retry changed state or posted: %v, POSTs: %d", err, f.posts())
			}
		})
	}
	f.setRemote(got)
	edited := fakeTrackerSpec + "Local edit made after the first POST.\n"
	qualityCLIFile(t, f.root, f.s.SpecPath, edited)
	f.success("publish", "spec", "--expect", f.token("publish", "spec", "--dry-run"))
	if f.posts() != 1 || trackerRead(t, filepath.Join(f.root, f.s.SpecPath)) != edited {
		t.Fatal("recovery attempted a second POST or replaced the draft")
	}
	out := f.success("preview", "spec")
	if !strings.Contains(out, "#10") || !strings.Contains(out, "Local edit made after the first POST.") {
		t.Fatalf("recovered Issue or edited draft missing from preview:\n%s", out)
	}
}

func TestPublishCheckpointFailureStopsBeforeMutation(t *testing.T) {
	f := newTrackerFixture(t)
	ctx := context.Background()
	failure := errors.New("checkpoint rename is visible but durable commit failed")
	err := lockfile.With(ctx, f.root, lockfile.ChangeKey(f.s.Change), func() error {
		s, err := state.Load(f.root, f.s.Change)
		if err != nil {
			return err
		}
		p, err := preparePublication(ctx, f.root, s, "spec")
		if err != nil {
			return err
		}
		return executePublication(ctx, io.Discard, f.root, s, p, func(root string, s flow.State) error {
			if err := state.Save(root, s); err != nil {
				return err
			}
			return failure
		})
	})
	if !errors.Is(err, failure) || f.posts() != 0 || len(f.remote()) != 0 {
		t.Fatalf("durability failure did not stop at the provider mutation boundary: %v, POSTs: %d", err, f.posts())
	}
}

func TestPublishPositiveReceiptRejectsSubstituteOnRetry(t *testing.T) {
	f := newTrackerFixture(t)
	token := f.token("publish", "spec", "--dry-run")
	// POST positively returns #10, then both listing and GET expose #11 with
	// identical body and marker. Neither this invocation nor a new approval
	// may change the observed receipt's identity to #11.
	t.Setenv("MULIX_TRACKER_WORKFLOW_REPLACE_RECEIPT", "10")
	if _, err := f.run("publish", "spec", "--expect", token); err == nil || f.posts() != 1 {
		t.Fatalf("conflicting creation did not stop with a single provider mutation: %v", err)
	}
	if out, err := f.run("publish", "spec", "--dry-run"); err == nil || trackerTestToken.MatchString(out) {
		t.Fatalf("a fresh approval would permit adopting the substitute for the observed receipt: %v\n%s", err, out)
	}
	if f.posts() != 1 {
		t.Fatalf("recovery attempted another mutation: %d", f.posts())
	}
	pending, err := state.Load(f.root, f.s.Change)
	if err != nil {
		t.Fatal(err)
	}
	m := pending.Tracker.Issues[0]
	if m.Published() || m.Receipt == nil || m.Receipt.Number != "10" || m.Receipt.NodeID != "I_fake_10" {
		t.Fatalf("observed identity was not retained in pending state: %+v", m)
	}
	m.Receipt = nil
	pending.Tracker.Issues[0] = m
	if err := state.Save(f.root, pending); err == nil {
		t.Fatal("ordinary save cleared the observed receipt")
	}
	remote := f.remote()
	remote[0].ID, remote[0].Number, remote[0].NodeID = 910, 10, "I_fake_10"
	f.setRemote(remote)
	f.success("publish", "spec", "--expect", f.token("publish", "spec", "--dry-run"))
	if f.posts() != 1 {
		t.Fatal("recovering the original receipt issued another mutation")
	}
}

func TestApplyRejectsStaleApprovalAndLocalEdits(t *testing.T) {
	f := newTrackerFixture(t)
	f.publishSpec()
	remote := f.remote()
	remote[0].Body += "Remote edit.\n"
	f.setRemote(remote)
	before := trackerRead(t, state.PathFor(f.root, f.s.Change))
	file := filepath.Join(f.root, f.s.SpecPath)
	// Proof that even missing lock bookkeeping is not created by preview.
	if err := os.RemoveAll(filepath.Join(f.root, layout.LocksDir)); err != nil {
		t.Fatal(err)
	}
	old := f.token("preview", "spec")
	if _, err := os.Lstat(filepath.Join(f.root, layout.LocksDir)); !os.IsNotExist(err) {
		t.Fatalf("preview created local lock files: %v", err)
	}
	remote[0].Body += "A concurrent remote edit.\n"
	f.setRemote(remote)
	if _, err := f.run("apply", "spec", "--expect", old); err == nil || trackerRead(t, file) != fakeTrackerSpec {
		t.Fatalf("stale remote approval overwrote snapshot: %v", err)
	}
	old = f.token("preview", "spec")
	local := strings.ReplaceAll(fakeTrackerSpec, "\n", "\r\n") + "Keep this local work.\r\n"
	qualityCLIFile(t, f.root, f.s.SpecPath, local)
	if _, err := f.run("apply", "spec", "--expect", old); err == nil {
		t.Fatal("approval was reused after a local edit")
	}
	defaultToken := f.token("preview", "spec")
	if _, err := f.run("apply", "spec", "--expect", defaultToken); err == nil {
		t.Fatal("an approved default apply overwrote local work")
	}
	if _, err := f.run("apply", "spec", "--backup-local", "--expect", defaultToken); err == nil {
		t.Fatal("an approval without the backup choice was accepted for replacement")
	}
	if trackerRead(t, file) != local || trackerRead(t, state.PathFor(f.root, f.s.Change)) != before {
		t.Fatal("a rejected apply modified the snapshot or binding")
	}
	f.success("apply", "spec", "--backup-local", "--expect", f.token("preview", "spec", "--backup-local"))
	entries, err := filepath.Glob(filepath.Join(f.root, layout.ChangeRuntimeDir(f.s.Change), "tracker-backups", "*.md"))
	if err != nil || len(entries) != 1 || trackerRead(t, entries[0]) != local || trackerRead(t, file) != remote[0].Body || f.posts() != 1 {
		t.Fatalf("confirmed apply failed to back up exact local bytes and install only the approved remote body: %v (%v)", entries, err)
	}
}

func TestApplyKeepsApprovedBytesDespiteOpenEditor(t *testing.T) {
	f := newTrackerFixture(t)
	f.publishSpec()
	remote := f.remote()
	remote[0].Body += "Remote amendment.\n"
	f.setRemote(remote)
	local := strings.ReplaceAll(fakeTrackerSpec, "\n", "\r\n") + "Keep the exact approved bytes.\r\n"
	qualityCLIFile(t, f.root, f.s.SpecPath, local)
	path := filepath.Join(f.root, f.s.SpecPath)
	statePath := state.PathFor(f.root, f.s.Change)
	before := trackerRead(t, statePath)
	token := f.token("preview", "spec", "--backup-local")

	// Simulate an editor that opened the original file and saved through that
	// FD after apply displaced the path. No sleeps or competing goroutines.
	editor, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer editor.Close()
	out, err := f.run("apply", "spec", "--backup-local", "--expect", token)
	if err != nil {
		var renameErr *os.LinkError
		if runtime.GOOS == "windows" && errors.As(err, &renameErr) && renameErr.Op == "rename" && strings.Contains(err.Error(), "could not displace local file") {
			if trackerRead(t, path) != local || trackerRead(t, statePath) != before {
				t.Fatalf("sharing failure changed the active file or state: %v", err)
			}
			t.Logf("Windows refused to rename an open file and apply failed safely: %v", err)
			return
		}
		t.Fatalf("apply: %v\n%s", err, out)
	}
	const edited = "Editor replaced all bytes through the retained inode.\n"
	if err := editor.Truncate(0); err != nil {
		t.Fatal(err)
	}
	if _, err := editor.WriteAt([]byte(edited), 0); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(f.root, layout.ChangeRuntimeDir(f.s.Change), "tracker-backups")
	approved, err := filepath.Glob(filepath.Join(base, "spec-approved-*", "local.md"))
	if err != nil || len(approved) != 1 || trackerRead(t, approved[0]) != local {
		t.Fatalf("open editor save lost the independent exact approved backup: %v, %v\n%s", approved, err, out)
	}
	editing, err := filepath.Glob(filepath.Join(base, "spec-*.md"))
	if err != nil || len(editing) != 1 || trackerRead(t, editing[0]) != edited || trackerRead(t, path) != remote[0].Body {
		t.Fatalf("editor's own save or the installed snapshot was lost: %v, %v\n%s", editing, err, out)
	}
	if !strings.Contains(out, "exact approved backup") || !strings.Contains(out, "displaced editing file") {
		t.Fatalf("apply did not distinguish the two paths:\n%s", out)
	}
}

func TestApplyPreservesConcurrentEditorSave(t *testing.T) {
	for _, tc := range []struct {
		name       string
		beforeMove bool
	}{
		{"autosave before displacement", true},
		{"editor recreates the path after displacement", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTrackerFixture(t)
			f.publishSpec()
			remote := f.remote()
			remote[0].Body += "Remote amendment.\n"
			f.setRemote(remote)
			local := fakeTrackerSpec + "Previously approved for backup.\n"
			edited := local + "New bytes from the editor.\r\n"
			qualityCLIFile(t, f.root, f.s.SpecPath, local)
			path := filepath.Join(f.root, f.s.SpecPath)
			statePath := state.PathFor(f.root, f.s.Change)
			before := trackerRead(t, statePath)
			ctx := context.Background()
			var out bytes.Buffer
			err := lockfile.With(ctx, f.root, lockfile.ChangeKey(f.s.Change), func() error {
				s, err := state.Load(f.root, f.s.Change)
				if err != nil {
					return err
				}
				v, err := readSnapshotView(ctx, f.root, s, 0, true)
				if err != nil {
					return err
				}
				return v.apply(&out, f.root, s, func(from, to string) error {
					if tc.beforeMove {
						qualityCLIFile(t, f.root, f.s.SpecPath, edited)
					}
					if err := os.Rename(from, to); err != nil {
						return err
					}
					if !tc.beforeMove {
						qualityCLIFile(t, f.root, f.s.SpecPath, edited)
					}
					return nil
				})
			})
			if err == nil || trackerRead(t, path) != edited || trackerRead(t, statePath) != before {
				t.Fatalf("autosave overwritten or stale snapshot accepted: local=%q err=%v output=%s", trackerRead(t, path), err, &out)
			}
			backups, err := filepath.Glob(filepath.Join(f.root, layout.ChangeRuntimeDir(f.s.Change), "tracker-backups", "*.md"))
			want := local
			if tc.beforeMove {
				want = edited
			}
			if err != nil || len(backups) != 1 || trackerRead(t, backups[0]) != want {
				t.Fatalf("the displaced editor bytes were not preserved: %v %v", backups, err)
			}
		})
	}
}

func TestIssueBodyIgnoresTrackerMetadata(t *testing.T) {
	f := newTrackerFixture(t)
	f.publishSpec()
	token := f.token("preview", "spec")
	before := trackerRead(t, state.PathFor(f.root, f.s.Change))
	remote := f.remote()
	remote[0].Status, remote[0].Title, remote[0].Labels, remote[0].Replies = "closed", "Retitled issue", []string{"ready"}, 8
	remote[0].Body = strings.ReplaceAll(remote[0].Body, "\n", "\r\n")
	f.setRemote(remote)
	f.success("apply", "spec", "--expect", token)
	if trackerRead(t, state.PathFor(f.root, f.s.Change)) != before || trackerRead(t, filepath.Join(f.root, f.s.SpecPath)) != fakeTrackerSpec {
		t.Fatal("metadata-only change rewrote the binding or snapshot")
	}
	remote[0].ID++ // same number, marker and contents, but a different Issue
	f.setRemote(remote)
	if _, err := f.run("preview", "spec"); err == nil {
		t.Fatal("accepted a different Issue ID at the bound number")
	}
}

func TestPublishTasksResumesPartialBatch(t *testing.T) {
	f := newTrackerFixture(t)
	if _, err := f.run("publish", "tasks", "--dry-run"); err == nil {
		t.Fatal("task publication was allowed in specify")
	}
	f.publishSpec()
	s, err := state.Load(f.root, f.s.Change)
	if err != nil {
		t.Fatal(err)
	}
	s.Phase = flow.PhaseTasks
	saveStateForTest(t, f.root, s)
	qualityCLIFile(t, f.root, quality.PolicyPath, issueTaskPolicy)
	for n := 1; n <= 3; n++ {
		body := issueTaskText(n, fmt.Sprintf("Task %d definition", n), "code", fmt.Sprintf("TestTask%d", n))
		qualityCLIFile(t, f.root, fmt.Sprintf("%s/%02d-definition.md", s.TasksDir, n), body+"Dependencies: follow the previous task.\n")
	}
	if _, err := f.run("publish", "spec", "--dry-run"); err == nil {
		t.Fatal("spec draft publication was allowed outside specify")
	}
	token := f.token("publish", "tasks", "--dry-run")
	oldPath := filepath.Join(f.root, s.TasksDir, "03-definition.md")
	newPath := filepath.Join(f.root, s.TasksDir, "03-renamed.md")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run("publish", "tasks", "--expect", token); err == nil || f.posts() != 1 {
		t.Fatalf("renamed task plan authorized a POST: %v, POSTs: %d", err, f.posts())
	}
	if err := os.Rename(newPath, oldPath); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MULIX_TRACKER_WORKFLOW_LOST_NUMBER", "12")
	if _, err := f.run("publish", "tasks", "--expect", token); err == nil || f.posts() != 3 {
		t.Fatalf("did not stop after an interrupted second task: %v, POSTs: %d", err, f.posts())
	}
	f.success("preview", "task-1")
	f.success("publish", "tasks", "--expect", f.token("publish", "tasks", "--dry-run"))
	if f.posts() != 3 {
		t.Fatal("recovery attempted a new POST")
	}
	f.success("preview", "task-2")
	f.success("publish", "tasks", "--expect", f.token("publish", "tasks", "--dry-run"))
	remote := f.remote()
	if f.posts() != 4 || len(remote) != 4 || !strings.Contains(remote[3].Body, "Parent spec: #10\n") || !strings.Contains(remote[3].Body, "Dependencies: follow the previous task.") || !strings.Contains(remote[3].Body, "## Task 3:") {
		t.Fatalf("task did not preserve the full definition and verified parent spec link: %+v", remote)
	}

	// Applying a task snapshot changes exactly that task, not its spec.
	task := filepath.Join(f.root, s.TasksDir, "03-definition.md")
	remote[3].Body += "Approved task amendment.\n"
	f.setRemote(remote)
	f.success("apply", "task-3", "--expect", f.token("preview", "task-3"))
	if trackerRead(t, task) != remote[3].Body || trackerRead(t, filepath.Join(f.root, s.SpecPath)) != fakeTrackerSpec || f.posts() != 4 {
		t.Fatal("task apply wrote outside the bound snapshot")
	}

	// A changed slot path must not redirect apply, even with valid Markdown at
	// that path. Simulate an edited state file (not a supported setter).
	approval := f.token("preview", "task-3")
	other := ".mulix/.runtime/another/issues/03-definition.md"
	qualityCLIFile(t, f.root, other, "Do not touch this change.\n")
	p := state.PathFor(f.root, f.s.Change)
	metadata := trackerRead(t, p)
	body := trackerRead(t, task)
	if !strings.Contains(metadata, s.TasksDir+"/03-definition.md") {
		t.Fatal("task slot did not persist a root-relative path")
	}
	qualityCLIFile(t, f.root, filepath.ToSlash(layout.ChangeRuntimePath(f.s.Change, layout.StateFile)), strings.ReplaceAll(metadata, s.TasksDir+"/03-definition.md", other))
	if _, err := f.run("apply", "task-3", "--expect", approval); err == nil || trackerRead(t, filepath.Join(f.root, other)) != "Do not touch this change.\n" || trackerRead(t, task) != body {
		t.Fatalf("apply accepted a borrowed task path: %v", err)
	}
}

func TestApplyRecoversMetadataForIdenticalSnapshot(t *testing.T) {
	f := newTrackerFixture(t)
	f.publishSpec()
	remote := f.remote()
	remote[0].Body += "Remote amendment.\n"
	f.setRemote(remote)
	old := f.token("preview", "spec")
	// Simulate interruption after writing the snapshot, but before saving the
	// new baseline. The matching file is enough only with a fresh approval.
	qualityCLIFile(t, f.root, f.s.SpecPath, remote[0].Body)
	if _, err := f.run("apply", "spec", "--expect", old); err == nil {
		t.Fatal("pre-write approval was reused for metadata recovery")
	}
	before := trackerRead(t, state.PathFor(f.root, f.s.Change))
	f.success("apply", "spec", "--expect", f.token("preview", "spec"))
	if got := trackerRead(t, state.PathFor(f.root, f.s.Change)); got == before || f.posts() != 1 {
		t.Fatal("metadata was not recovered without another remote write")
	}
}
