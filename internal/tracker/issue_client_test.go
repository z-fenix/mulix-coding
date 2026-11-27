package tracker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/tracker"
)

type issueRoute struct {
	Response string
	Request  map[string]any
	Fail     bool
}

func setIssueRoutes(t *testing.T, routes map[string]issueRoute) {
	t.Helper()
	data, err := json.Marshal(routes)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "routes.json")
	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MULIX_TRACKER_TEST_ROUTES", filename)
}

const testGitLabCloseQuery = `mutation($id:WorkItemID!){workItemUpdate(input:{id:$id,stateEvent:CLOSE}){workItem{id iid state} errors}}`
const testGitLabWorkItemID = "gid://gitlab/WorkItem/512"
const testGitLabClosed = `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/512","iid":"7","state":"CLOSED"},"errors":[]}}}`

func testGitLabCloseCall(host string) string {
	return "glab api graphql --hostname " + host + " -f query=" + testGitLabCloseQuery + " -f id=" + testGitLabWorkItemID
}

func TestGitLabCloseUsesImmutableWorkItem(t *testing.T) {
	const get = "glab api projects/74/issues/7 --hostname gitlab.com"
	const unsafePut = get + " --method PUT --input - --header Content-Type: application/json"
	for _, tc := range []struct {
		name, result string
		ok           bool
	}{
		{"original WorkItem", testGitLabClosed, true},
		{"unsupported schema", `{"errors":[{"message":"no workItemUpdate, fake-auth-secret"}]}`, false},
		{"replacement receipt", strings.Replace(testGitLabClosed, "/512", "/513", 1), false},
		{"mutation error", strings.Replace(testGitLabClosed, `"errors":[]`, `"errors":["fake-auth-secret"]`, 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := fakeProviderCLIs(t)
			closeCall := testGitLabCloseCall("gitlab.com")
			setIssueRoutes(t, map[string]issueRoute{
				get: {Response: `{"id":512,"iid":7,"project_id":74,"description":"body","state":"opened"}`},
				// A simulated reuse occurs between GET and PUT. Detecting the
				// mismatched response after PUT would already be too late.
				unsafePut: {
					Request:  map[string]any{"state_event": "close"},
					Response: `{"id":513,"iid":7,"project_id":74,"description":"replacement","state":"closed"}`,
				},
				closeCall: {Response: tc.result},
			})
			target := tracker.Target{Kind: "gitlab", Host: "gitlab.com", Project: "team/repo", ProjectID: "74"}
			err := (tracker.Client{}).Close(context.Background(), target, tracker.Issue{ID: "512", Number: "7", Body: "body"})
			calls, logErr := os.ReadFile(log)
			if logErr != nil || string(calls) != get+"\n"+closeCall+"\n" {
				t.Fatalf("closure did not use only the verified immutable WorkItem: %v\n%s", err, calls)
			}
			if (err == nil) != tc.ok || (err != nil && strings.Contains(err.Error(), "fake-auth-secret")) {
				t.Fatalf("unexpected or unsanitized closure result: %v", err)
			}
			if tc.name == "unsupported schema" && !strings.Contains(err.Error(), "workItemUpdate") {
				t.Fatalf("unsupported server had no capability guidance: %v", err)
			}
		})
	}
}

func TestIssueCloseRejectsBodyChangeAtProviderRecheck(t *testing.T) {
	const get = "glab api projects/74/issues/7 --hostname gitlab.com"
	log := fakeProviderCLIs(t)
	setIssueRoutes(t, map[string]issueRoute{
		get:                               {Response: `{"id":512,"iid":7,"project_id":74,"description":"new body","state":"opened"}`},
		testGitLabCloseCall("gitlab.com"): {Response: testGitLabClosed},
	})
	target := tracker.Target{Kind: "gitlab", Host: "gitlab.com", Project: "team/repo", ProjectID: "74"}
	err := (tracker.Client{}).Close(t.Context(), target, tracker.Issue{ID: "512", Number: "7", Body: "approved body"})
	calls, logErr := os.ReadFile(log)
	if logErr != nil || err == nil || string(calls) != get+"\n" {
		t.Fatalf("changed body reached a mutation at the final read: %v\n%s", err, calls)
	}
}

func TestClientIssueAdapterPinnedProvider(t *testing.T) {
	ctx := context.Background()
	const marker = "<!-- mulix: tracked-slot -->"
	const title = "literal @file --hostname evil.example :id {owner}"
	const body = marker + "\n\nKeep @text, :repo, and {branch}.\n"
	const quoted = `"<!-- mulix: tracked-slot -->\n\nKeep @text, :repo, and {branch}.\n"`
	for _, tc := range []struct {
		name, cli, host, project, id, probe, endpoint, query, response string
	}{
		{"GitHub", "gh", "github.com", "team/repo", "42", "repos/team/repo", "repos/team/repo/issues", "?state=all&per_page=100",
			`{"id":512,"number":7,"node_id":"I_original","repository_url":"https://api.github.com/repos/team/repo","state":"closed","body":` + quoted + `}`},
		{"GitLab.com", "glab", "gitlab.com", "team/repo", "74", "projects/team%2Frepo", "projects/74/issues", "?scope=all&state=all&per_page=100",
			`{"id":512,"iid":7,"project_id":74,"state":"closed","description":` + quoted + `}`},
		{"Self-hosted GitLab subgroup", "glab", "git.example.test:8443", "team/sub/repo", "73", "projects/team%2Fsub%2Frepo", "projects/73/issues", "?scope=all&state=all&per_page=100",
			`{"id":512,"iid":7,"state":"closed","description":` + quoted + `}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := fakeProviderCLIs(t)
			// Ambient project/host settings must not select the destination.
			t.Setenv("GH_HOST", "ambient.example.test")
			t.Setenv("GH_REPO", "ambient/other")
			t.Setenv("GITLAB_HOST", "ambient.example.test")
			cli := tc.cli + " api "
			host := " --hostname " + tc.host
			input := " --input - --header Content-Type: application/json"
			kind, projectKey := "github", "full_name"
			pages := `[{"number":99,"pull_request":{},"body":` + quoted + `}]` + "\n[" + tc.response + "]\n"
			want := tracker.Issue{ID: "512", Number: "7", Body: body, NodeID: "I_original"}
			receipt := tracker.CreateReceipt{Number: "7", NodeID: "I_original"}
			createCall := cli + "graphql" + host + " --method POST" + input
			closeCall := createCall
			createRequest := issueRoute{
				Request: map[string]any{
					"query":     `mutation($repository:ID!,$title:String!,$body:String!){createIssue(input:{repositoryId:$repository,title:$title,body:$body}){issue{id number}}}`,
					"variables": map[string]any{"repository": "R_original", "title": title, "body": body},
				},
				Response: `{"data":{"createIssue":{"issue":{"id":"I_original","number":7}}}}`,
			}
			closeRequest := issueRoute{
				Request: map[string]any{
					"query":     `mutation($issue:ID!){closeIssue(input:{issueId:$issue}){issue{id number state}}}`,
					"variables": map[string]any{"issue": "I_original"},
				},
				Response: `{"data":{"closeIssue":{"issue":{"id":"I_original","number":7,"state":"CLOSED"}}}}`,
			}
			if tc.cli == "glab" {
				kind, projectKey = "gitlab", "path_with_namespace"
				pages = "[" + tc.response + "]\n"
				want.NodeID, receipt.ID, receipt.NodeID = "", "512", ""
				createCall = cli + tc.endpoint + host + " --method POST" + input
				closeCall = testGitLabCloseCall(tc.host)
				createRequest = issueRoute{Request: map[string]any{"title": title, "description": body}, Response: tc.response}
				closeRequest = issueRoute{Response: testGitLabClosed}
			}
			routes := map[string]issueRoute{
				cli + tc.probe + host:                               {Response: fmt.Sprintf(`{"id":%s,%q:%q,"node_id":"R_original"}`, tc.id, projectKey, tc.project)},
				cli + tc.endpoint + "/7" + host:                     {Response: tc.response},
				cli + tc.endpoint + tc.query + host + " --paginate": {Response: pages},
				createCall: createRequest,
			}
			setIssueRoutes(t, routes)
			target, err := (tracker.Client{}).Probe(ctx, tracker.Target{Kind: kind, Host: tc.host, Project: tc.project})
			if err != nil || target.ProjectID != tc.id {
				t.Fatalf("probe pinned target: %+v %v", target, err)
			}
			client := tracker.Client{}
			if got, err := client.Get(ctx, target, "7"); got != want || err != nil {
				t.Fatalf("Get: %+v %v", got, err)
			}
			if got, err := client.Find(ctx, target, marker); len(got) != 1 || got[0] != want || err != nil {
				t.Fatalf("Find must see closed issues on all pages and ignore PRs: %+v %v", got, err)
			}
			if got, err := client.Create(ctx, target, title, body); got != receipt || err != nil {
				t.Fatalf("Create: %+v %v", got, err)
			}
			routes[closeCall] = closeRequest
			setIssueRoutes(t, routes)
			before, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			wrong := want
			wrong.ID = "513"
			if err := client.Close(ctx, target, wrong); err == nil {
				t.Fatal("Close accepted a substitute for the verified Issue")
			}
			if err := client.Close(ctx, target, want); err != nil {
				t.Fatalf("Close: %v", err)
			}
			calls, err := os.ReadFile(log)
			if err != nil || strings.Count(strings.TrimPrefix(string(calls), string(before)), closeCall+"\n") != 1 {
				t.Fatalf("Close did not send exactly one pinned closure request: %v\n%s", err, calls)
			}
		})
	}
}

func TestClientIssueAdapterRejectsInvalidIdentity(t *testing.T) {
	ctx := context.Background()
	log := fakeProviderCLIs(t)
	c := tracker.Client{}
	target := tracker.Target{Kind: "github", Host: "github.example.test", Project: "team/repo", ProjectID: "42"}
	for _, n := range []string{"", "0", "01", "+7", "7/1", "7?x", "1e3", "-7"} {
		if _, err := c.Get(ctx, target, n); err == nil {
			t.Errorf("accepted invalid Issue number %q", n)
		}
	}
	pending := target
	pending.ProjectID = ""
	if _, err := c.Create(ctx, pending, "Title", "Body"); err == nil {
		t.Fatal("unverified target can create issues")
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("executed provider with invalid identity: %v", err)
	}
	base := `{"id":512,"number":7,"node_id":"I_original","body":"example",` +
		`"repository_url":"https://github.example.test/api/v3/repos/team/repo"}`
	probe := "gh api repos/team/repo --hostname github.example.test"
	for _, tc := range []struct{ name, response string }{
		{"number mismatch", strings.Replace(base, `"number":7`, `"number":8`, 1)},
		{"wrong project", strings.Replace(base, "team/repo", "other/repo", 1)},
		{"wrong host", strings.Replace(base, "github.example.test", "other.example.test", 1)},
		{"exponent ID", strings.Replace(base, `"id":512`, `"id":51e2`, 1)},
		{"quoted ID", strings.Replace(base, `"id":512`, `"id":"512"`, 1)},
		{"pull request", strings.TrimSuffix(base, "}") + `,"pull_request":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setIssueRoutes(t, map[string]issueRoute{
				probe: {Response: `{"id":42,"full_name":"team/repo"}`},
				"gh api repos/team/repo/issues/7 --hostname github.example.test": {Response: tc.response},
			})
			if _, err := c.Get(ctx, target, "7"); err == nil {
				t.Fatal("accepted invalid Issue response")
			}
		})
	}
	t.Run("wrong GitLab project_id", func(t *testing.T) {
		setIssueRoutes(t, map[string]issueRoute{
			"glab api projects/team%2Frepo --hostname gitlab.com": {Response: `{"id":74,"path_with_namespace":"team/repo"}`},
			"glab api projects/74/issues/7 --hostname gitlab.com": {Response: `{"id":512,"iid":7,"project_id":75,"description":"body"}`},
		})
		if _, err := c.Get(ctx, tracker.Target{Kind: "gitlab", Host: "gitlab.com", Project: "team/repo", ProjectID: "74"}, "7"); err == nil {
			t.Fatal("accepted GitLab response from another project")
		}
	})
	setIssueRoutes(t, map[string]issueRoute{
		probe: {Response: `{"id":42,"full_name":"team/repo"}`},
		"gh api repos/team/repo/issues/7 --hostname github.example.test": {Fail: true},
	})
	if _, err := c.Get(ctx, target, "7"); err == nil || strings.Contains(err.Error(), "fake-auth-secret") {
		t.Fatalf("authentication failure was not sanitized: %v", err)
	}
}

func TestClientIssueFindRefusesIncompletePages(t *testing.T) {
	fakeProviderCLIs(t)
	target := tracker.Target{Kind: "gitlab", Host: "gitlab.com", Project: "team/repo", ProjectID: "74"}
	for _, list := range []string{"", "null", "[]\nnull", "[]\n[null]"} {
		setIssueRoutes(t, map[string]issueRoute{
			"glab api projects/74/issues?scope=all&state=all&per_page=100 --hostname gitlab.com --paginate": {Response: list},
		})
		if _, err := (tracker.Client{}).Find(context.Background(), target, "<!-- tracked -->"); err == nil {
			t.Errorf("incomplete listing %q was treated as no matches", list)
		}
	}
}

func TestIssueCreateUsesPinnedNodeAfterSlugMoves(t *testing.T) {
	log := fakeProviderCLIs(t)
	// The verified GET still describes R_original. Suppose the slug is
	// reassigned immediately afterwards: a REST POST would send the draft to
	// the new owner, even though its Issue reports the same repository_url.
	setIssueRoutes(t, map[string]issueRoute{
		"gh api repos/team/repo --hostname github.com": {Response: `{"id":42,"node_id":"R_original","full_name":"team/repo"}`},
		"gh api repos/team/repo/issues --hostname github.com --method POST --input - --header Content-Type: application/json": {
			Request:  map[string]any{"title": "Draft", "body": "Only for the original repo"},
			Response: `{"id":900,"number":1,"node_id":"I_wrong","body":"Only for the original repo","repository_url":"https://api.github.com/repos/team/repo"}`,
		},
		"gh api graphql --hostname github.com --method POST --input - --header Content-Type: application/json": {
			Request: map[string]any{
				"query":     `mutation($repository:ID!,$title:String!,$body:String!){createIssue(input:{repositoryId:$repository,title:$title,body:$body}){issue{id number}}}`,
				"variables": map[string]any{"repository": "R_original", "title": "Draft", "body": "Only for the original repo"},
			},
			Response: `{"data":{"createIssue":{"issue":{"id":"I_original","number":10}}}}`,
		},
	})
	target := tracker.Target{Kind: "github", Host: "github.com", Project: "team/repo", ProjectID: "42"}
	result, err := (tracker.Client{}).Create(context.Background(), target, "Draft", "Only for the original repo")
	calls, readErr := os.ReadFile(log)
	if err != nil || readErr != nil || result.Number != "10" || strings.Contains(string(calls), "repos/team/repo/issues") || !strings.Contains(string(calls), "api graphql ") {
		t.Fatalf("draft did not go exclusively to the pinned repository node: %+v %v\n%s", result, err, calls)
	}
}

func TestIssueMissingBodyCannotHideMarker(t *testing.T) {
	fakeProviderCLIs(t)
	target := tracker.Target{Kind: "gitlab", Host: "gitlab.com", Project: "team/repo", ProjectID: "74"}
	list := "glab api projects/74/issues?scope=all&state=all&per_page=100 --hostname gitlab.com --paginate"
	get := "glab api projects/74/issues/7 --hostname gitlab.com"
	for _, tc := range []struct {
		name, bodyField string
		valid           bool
	}{
		{"omitted", "", false},
		{"nontext", `,"description":{}`, false},
		{"explicit null", `,"description":null`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := `{"id":512,"iid":7,"project_id":74` + tc.bodyField + `}`
			setIssueRoutes(t, map[string]issueRoute{list: {Response: "[" + row + "]"}, get: {Response: row}})
			c := tracker.Client{}
			found, err := c.Find(context.Background(), target, "<!-- tracked -->")
			if (err == nil) != tc.valid || len(found) != 0 {
				t.Errorf("Find treated %s body incorrectly: %+v %v", tc.name, found, err)
			}
			issue, err := c.Get(context.Background(), target, "7")
			if (err == nil) != tc.valid || issue.Body != "" {
				t.Errorf("Get treated %s body incorrectly: %+v %v", tc.name, issue, err)
			}
		})
	}
}

func TestIssueProbeRejectsQuotedProjectID(t *testing.T) {
	fakeProviderCLIs(t)
	setIssueRoutes(t, map[string]issueRoute{
		"gh api repos/team/repo --hostname github.example.test": {Response: `{"id":"42","full_name":"team/repo"}`},
	})
	if _, err := (tracker.Client{}).Probe(context.Background(), tracker.Target{Kind: "github", Host: "github.example.test", Project: "team/repo"}); err == nil {
		t.Fatal("accepted a non-numeric project identity")
	}
}
