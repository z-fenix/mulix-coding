package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ErrUnreachable denotes a failed read-only remote probe, not invalid identity.
var ErrUnreachable = errors.New("tracker probe unavailable")

type Client struct{}

// Issue exposes stable provider identity and authored body, not tracker status.
// Number is repository-local on GitHub and the IID on GitLab; ID is not.
type Issue struct {
	ID     string
	Number string
	Body   string
	NodeID string // immutable GitHub GraphQL ID, returned by REST as node_id
}

// CreateReceipt records the identity known from a positive mutation response.
// GitHub returns its immutable NodeID rather than a potentially truncated
// databaseId. The numeric Issue.ID is obtained from the subsequent REST read.
type CreateReceipt struct {
	Number string `yaml:"number"`
	ID     string `yaml:"id,omitempty"`
	NodeID string `yaml:"node_id,omitempty"`
}

func (r CreateReceipt) validFor(target Target) bool {
	if !decimalID(r.Number) {
		return false
	}
	if target.Kind == "github" {
		return validNodeID(r.NodeID) && (r.ID == "" || decimalID(r.ID))
	}
	return r.NodeID == "" && decimalID(r.ID)
}

func (r CreateReceipt) matches(issue Issue) bool {
	return r.Number == issue.Number && r.NodeID == issue.NodeID && (r.ID == "" || r.ID == issue.ID)
}

// Probe reads the explicitly named project and validates its returned identity.
func (Client) Probe(ctx context.Context, target Target) (Target, error) {
	checked, _, err := probeProject(ctx, target)
	return checked, err
}

// A GitHub write uses only the node returned by this same checked GET, never
// a second slug dereference that could resolve to a different repository.
func probeProject(ctx context.Context, target Target) (Target, string, error) {
	if err := target.Validate(); err != nil {
		return Target{}, "", err
	}
	if target.Kind == "markdown" {
		return target, "", nil
	}
	endpoint := "repos/" + target.Project
	if target.Kind == "gitlab" {
		endpoint = "projects/" + url.PathEscape(target.Project)
	}
	out, err := runAPI(ctx, target, endpoint, "", false, nil)
	if err != nil {
		return Target{}, "", fmt.Errorf("%w: cannot read %s project on %s", ErrUnreachable, target.Kind, target.Host)
	}
	var response struct {
		ID       json.RawMessage `json:"id"`
		NodeID   string          `json:"node_id"`
		FullName string          `json:"full_name"`
		Path     string          `json:"path_with_namespace"`
	}
	if !utf8.Valid(out) || json.Unmarshal(out, &response) != nil {
		return Target{}, "", fmt.Errorf("invalid tracker project response")
	}
	actual := response.FullName
	if target.Kind == "gitlab" {
		actual = response.Path
	}
	if !strings.EqualFold(actual, target.Project) {
		return Target{}, "", fmt.Errorf("tracker returned a different or incomplete project identity")
	}
	id := string(response.ID)
	if !decimalID(id) {
		return Target{}, "", fmt.Errorf("tracker returned an invalid project ID")
	}
	if target.ProjectID != "" && target.ProjectID != id {
		return Target{}, "", fmt.Errorf("tracker project ID changed; verify the target before proceeding")
	}
	target.ProjectID = id
	return target, response.NodeID, nil
}

func decimalID(s string) bool {
	return s != "" && s[0] >= '1' && s[0] <= '9' && strings.Trim(s[1:], "0123456789") == ""
}

func validNodeID(s string) bool {
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

// runAPI sends a preconstructed JSON payload where a provider supports it.
func runAPI(ctx context.Context, target Target, endpoint, method string, paginate bool, payload any) ([]byte, error) {
	var options []string
	if paginate {
		options = append(options, "--paginate")
	}
	if method != "" {
		options = append(options, "--method", method, "--input", "-", "--header", "Content-Type: application/json")
	}
	return runAPIArgs(ctx, target, endpoint, options, payload)
}

// runAPIArgs uses the caller's trusted CLI and authentication. Do not route
// through a shell, ambient defaults, or worktree repository placeholders, and
// never include remote stdout or stderr in request diagnostics.
func runAPIArgs(ctx context.Context, target Target, endpoint string, options []string, payload any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	name := "gh"
	if target.Kind == "gitlab" {
		name = "glab"
	}
	args := append([]string{"api", endpoint, "--hostname", target.Host}, options...)
	cmd := exec.CommandContext(ctx, name, args...)
	for _, entry := range cmd.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "GH_HOST", "GH_REPO", "GH_FORCE_TTY", "GLAB_HOST", "GITLAB_HOST", "GITLAB_URI", "GITLAB_URL":
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "GH_PROMPT_DISABLED=1", "GLAB_NO_PROMPT=1", "GIT_TERMINAL_PROMPT=0")
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("cannot encode tracker request")
		}
		cmd.Stdin = bytes.NewReader(data)
	}
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s API request failed on %s (check CLI authentication and connectivity)", name, target.Host)
	}
	return out, nil
}

func requireVerifiedTarget(target Target) error {
	if err := target.Validate(); err != nil {
		return err
	}
	if target.ProjectID == "" {
		return fmt.Errorf("Issue operations require a verified remote tracker project")
	}
	return nil
}

func (c Client) issuesEndpoint(ctx context.Context, target Target) (string, error) {
	if err := requireVerifiedTarget(target); err != nil {
		return "", err
	}
	if target.Kind == "gitlab" {
		return gitlabIssuesEndpoint(target), nil
	}
	// GitHub's REST reads use a slug and recheck its numeric pin first.
	// No mutation ever uses this mutable path.
	if _, err := c.Probe(ctx, target); err != nil {
		return "", err
	}
	return "repos/" + target.Project + "/issues", nil
}

func gitlabIssuesEndpoint(target Target) string { return "projects/" + target.ProjectID + "/issues" }

type apiIssue struct {
	ID            json.RawMessage `json:"id"`
	Number        json.RawMessage `json:"number"`
	IID           json.RawMessage `json:"iid"`
	ProjectID     json.RawMessage `json:"project_id"`
	RepositoryURL string          `json:"repository_url"`
	PullRequest   json.RawMessage `json:"pull_request"`
	NodeID        string          `json:"node_id"`
	Body          json.RawMessage `json:"body"`
	Description   json.RawMessage `json:"description"`
}

func (r apiIssue) issue(target Target, number string) (Issue, error) {
	identity, err := r.identity(target, number)
	if err != nil {
		return Issue{}, err
	}
	body := r.Body
	if target.Kind == "gitlab" {
		body = r.Description
	}
	// An explicit null is an empty authored body. An omitted field (or a
	// non-string value) cannot establish that an issue has no marker.
	if len(body) == 0 || json.Unmarshal(body, &identity.Body) != nil {
		return Issue{}, fmt.Errorf("tracker Issue response is missing a valid body field")
	}
	return identity, nil
}

func (r apiIssue) identity(target Target, number string) (Issue, error) {
	n := string(r.Number)
	if target.Kind == "gitlab" {
		n = string(r.IID)
		if len(r.ProjectID) != 0 && string(r.ProjectID) != target.ProjectID {
			return Issue{}, fmt.Errorf("Issue response has a different GitLab project identity")
		}
	} else {
		if len(r.PullRequest) != 0 && string(r.PullRequest) != "null" {
			return Issue{}, fmt.Errorf("tracker returned a pull request or invalid pull_request field, not an Issue")
		}
		if !githubRepositoryURL(target, r.RepositoryURL) {
			return Issue{}, fmt.Errorf("Issue response has a different GitHub repository identity")
		}
		if !validNodeID(r.NodeID) {
			return Issue{}, fmt.Errorf("GitHub Issue response is missing its immutable node ID")
		}
	}
	if !decimalID(string(r.ID)) || !decimalID(n) || (number != "" && n != number) {
		return Issue{}, fmt.Errorf("tracker returned an invalid or unexpected Issue identity")
	}
	issue := Issue{ID: string(r.ID), Number: n}
	if target.Kind == "github" {
		issue.NodeID = r.NodeID
	}
	return issue, nil
}

func githubRepositoryURL(target Target, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	host, prefix := target.Host, "/api/v3/repos/"
	if strings.EqualFold(host, "github.com") || strings.HasSuffix(strings.ToLower(host), ".ghe.com") {
		host, prefix = "api."+host, "/repos/"
	}
	return strings.EqualFold(u.Host, host) && strings.EqualFold(u.Path, prefix+target.Project)
}

func requestIssue(ctx context.Context, target Target, endpoint, method string, payload any) (apiIssue, error) {
	out, err := runAPI(ctx, target, endpoint, method, false, payload)
	if err != nil {
		return apiIssue{}, err
	}
	var response apiIssue
	if !utf8.Valid(out) || json.Unmarshal(out, &response) != nil {
		return apiIssue{}, fmt.Errorf("invalid tracker Issue response")
	}
	return response, nil
}

func (c Client) Get(ctx context.Context, target Target, number string) (Issue, error) {
	if !decimalID(number) {
		return Issue{}, fmt.Errorf("Issue number must be a positive decimal integer")
	}
	endpoint, err := c.issuesEndpoint(ctx, target)
	if err != nil {
		return Issue{}, err
	}
	response, err := requestIssue(ctx, target, endpoint+"/"+number, "", nil)
	if err != nil {
		return Issue{}, err
	}
	return response.issue(target, number)
}

// Find scans all project issues, including closed ones. Page output may be a
// single array or a sequence of arrays; malformed output never means zero hits.
func (c Client) Find(ctx context.Context, target Target, marker string) ([]Issue, error) {
	if marker == "" || strings.ContainsAny(marker, "\r\n") {
		return nil, fmt.Errorf("expected one nonempty Issue marker line")
	}
	endpoint, err := c.issuesEndpoint(ctx, target)
	if err != nil {
		return nil, err
	}
	query := "?state=all&per_page=100"
	if target.Kind == "gitlab" {
		query = "?scope=all&state=all&per_page=100"
	}
	out, err := runAPI(ctx, target, endpoint+query, "", true, nil)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(out) {
		return nil, fmt.Errorf("invalid tracker Issue listing encoding")
	}
	dec := json.NewDecoder(bytes.NewReader(out))
	var matches []Issue
	for seen := false; ; seen = true {
		var page []apiIssue
		if err := dec.Decode(&page); err == io.EOF && seen {
			return matches, nil
		} else if err != nil || page == nil {
			return nil, fmt.Errorf("invalid tracker Issue listing; expected JSON array pages")
		}
		for _, r := range page {
			if target.Kind == "github" && bytes.HasPrefix(r.PullRequest, []byte("{")) {
				continue
			}
			issue, err := r.issue(target, "")
			if err != nil {
				return nil, err
			}
			if markerCount(issue.Body, marker) != 0 {
				matches = append(matches, issue)
			}
		}
	}
}

const githubCreateIssue = `mutation($repository:ID!,$title:String!,$body:String!){createIssue(input:{repositoryId:$repository,title:$title,body:$body}){issue{id number}}}`
const githubCloseIssue = `mutation($issue:ID!){closeIssue(input:{issueId:$issue}){issue{id number state}}}`

func githubMutation(ctx context.Context, target Target, query, operation string, variables map[string]string) (CreateReceipt, string, error) {
	request := struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}{query, variables}
	out, err := runAPI(ctx, target, "graphql", "POST", false, request)
	if err != nil {
		return CreateReceipt{}, "", err
	}
	var response struct {
		Data map[string]struct {
			Issue struct {
				ID     string          `json:"id"`
				Number json.RawMessage `json:"number"`
				State  string          `json:"state"`
			} `json:"issue"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if !utf8.Valid(out) || json.Unmarshal(out, &response) != nil || len(response.Errors) != 0 {
		return CreateReceipt{}, "", fmt.Errorf("GitHub did not confirm %s", operation)
	}
	issue := response.Data[operation].Issue
	receipt := CreateReceipt{Number: string(issue.Number), NodeID: issue.ID}
	if !receipt.validFor(target) {
		return CreateReceipt{}, "", fmt.Errorf("GitHub %s did not return a valid Issue identity", operation)
	}
	return receipt, issue.State, nil
}

func (Client) Create(ctx context.Context, target Target, title, body string) (CreateReceipt, error) {
	if strings.TrimSpace(title) == "" || !utf8.ValidString(title) || !utf8.ValidString(body) {
		return CreateReceipt{}, fmt.Errorf("Issue creation requires a title and valid UTF-8 text")
	}
	if err := requireVerifiedTarget(target); err != nil {
		return CreateReceipt{}, err
	}
	if target.Kind == "github" {
		_, node, err := probeProject(ctx, target)
		if err != nil {
			return CreateReceipt{}, err
		}
		if !validNodeID(node) {
			return CreateReceipt{}, fmt.Errorf("verified GitHub repository response is missing its immutable node ID")
		}
		receipt, _, err := githubMutation(ctx, target, githubCreateIssue, "createIssue", map[string]string{"repository": node, "title": title, "body": body})
		return receipt, err
	}
	response, err := requestIssue(ctx, target, gitlabIssuesEndpoint(target), "POST", map[string]string{"title": title, "description": body})
	if err != nil {
		return CreateReceipt{}, err
	}
	issue, err := response.identity(target, "")
	return CreateReceipt{Number: issue.Number, ID: issue.ID}, err
}

// Close is a provider primitive, not workflow permission to close an Issue.
// The caller supplies the verified Issue rather than just a reusable number.
func (c Client) Close(ctx context.Context, target Target, expected Issue) error {
	if !decimalID(expected.ID) {
		return fmt.Errorf("closure requires the verified Issue ID")
	}
	before, err := c.Get(ctx, target, expected.Number)
	if err != nil {
		return err
	}
	if before.ID != expected.ID || before.NodeID != expected.NodeID {
		return fmt.Errorf("bound Issue identity changed before closure")
	}
	if BodyDigest(before.Body) != BodyDigest(expected.Body) {
		return fmt.Errorf("bound Issue body changed before closure; review a fresh dry run")
	}
	if target.Kind == "github" {
		closed, status, err := githubMutation(ctx, target, githubCloseIssue, "closeIssue", map[string]string{"issue": expected.NodeID})
		if err != nil {
			return err
		}
		if !closed.matches(expected) || status != "CLOSED" {
			return fmt.Errorf("tracker did not confirm closure of the same Issue")
		}
		return nil
	}
	return closeGitLabWorkItem(ctx, target, expected)
}

const gitlabCloseWorkItem = `mutation($id:WorkItemID!){workItemUpdate(input:{id:$id,stateEvent:CLOSE}){workItem{id iid state} errors}}`

func closeGitLabWorkItem(ctx context.Context, target Target, expected Issue) error {
	// GitLab's REST Issue ID is the WorkItem record ID. Its IID is scoped
	// and reusable, so never use it to address a write (even after a GET).
	gid := "gid://gitlab/WorkItem/" + expected.ID
	out, err := runAPIArgs(ctx, target, "graphql", []string{"-f", "query=" + gitlabCloseWorkItem, "-f", "id=" + gid}, nil)
	const hint = "GitLab WorkItem closure not confirmed; check update permission and server support for workItemUpdate/CLOSE (no IID-based fallback)"
	if err != nil {
		return fmt.Errorf("%s: %w", hint, err)
	}
	var response struct {
		Data struct {
			Update struct {
				Item struct {
					ID    string `json:"id"`
					IID   string `json:"iid"`
					State string `json:"state"`
				} `json:"workItem"`
				Errors []json.RawMessage `json:"errors"`
			} `json:"workItemUpdate"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if !utf8.Valid(out) || json.Unmarshal(out, &response) != nil || len(response.Errors) != 0 || len(response.Data.Update.Errors) != 0 {
		return errors.New(hint)
	}
	item := response.Data.Update.Item
	if item.ID != gid || item.IID != expected.Number || item.State != "CLOSED" {
		return fmt.Errorf("%s; response did not confirm the same closed WorkItem", hint)
	}
	return nil
}

func markerCount(body, marker string) int {
	count := 0
	for line := range strings.SplitSeq(NormalizeBody(body), "\n") {
		if line == marker {
			count++
		}
	}
	return count
}
