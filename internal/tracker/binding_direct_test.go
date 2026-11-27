package tracker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/tracker"
)

func TestChangeBindingValidation(t *testing.T) {
	markdown, err := tracker.NewChangeBinding(tracker.Target{Kind: "markdown"})
	if err != nil || markdown.RemoteKey != "" || markdown.Validate() != nil {
		t.Fatalf("local binding: %+v, %v", markdown, err)
	}
	if _, err := tracker.NewChangeBinding(tracker.Target{Kind: "github"}); err == nil {
		t.Fatal("accepted invalid target")
	}
	for _, kind := range []string{"github", "gitlab"} {
		t.Run(kind, func(t *testing.T) {
			target := tracker.Target{Kind: kind, Host: "example.test", Project: "team/repo"}
			b, err := tracker.NewChangeBinding(target)
			if err != nil || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(b.RemoteKey) || b.Validate() != nil {
				t.Fatalf("pending binding: %+v %v", b, err)
			}
			other, err := tracker.NewChangeBinding(target)
			if err != nil || other.RemoteKey == b.RemoteKey {
				t.Fatal("keys should be independently generated")
			}
			d := tracker.BodyDigest("draft")
			spec := tracker.IssueBinding{Path: "spec.md", DraftDigest: d, PublishDigest: d, ID: "512", Number: "7", BodyDigest: d}
			if kind == "github" {
				spec.NodeID = "I_original"
				spec.Receipt = &tracker.CreateReceipt{Number: "7", NodeID: "I_original"}
			} else {
				spec.Receipt = &tracker.CreateReceipt{Number: "7", ID: "512"}
			}
			b.Issues = map[int]tracker.IssueBinding{0: spec}
			if b.Validate() == nil {
				t.Fatal("unverified project accepted slots")
			}
			b.Target.ProjectID = "42"
			if err := b.Validate(); err != nil {
				t.Fatalf("valid spec: %v", err)
			}
			task := spec
			task.Path, task.ID, task.Number, task.Receipt = "01-task.md", "513", "8", nil
			if kind == "github" {
				task.NodeID = "I_task"
			}
			b.Issues[1] = task
			if err := b.Validate(); err != nil {
				t.Fatalf("valid task: %v", err)
			}
			cases := []struct {
				name   string
				mutate func(*tracker.ChangeBinding)
			}{
				{"bad key", func(x *tracker.ChangeBinding) { x.RemoteKey = strings.ToUpper(x.RemoteKey) + "X" }},
				{"negative slot", func(x *tracker.ChangeBinding) { x.Issues[-1] = task }},
				{"missing path", func(x *tracker.ChangeBinding) { v := x.Issues[1]; v.Path = ""; x.Issues[1] = v }},
				{"bad digest", func(x *tracker.ChangeBinding) { v := x.Issues[1]; v.DraftDigest = "bad"; x.Issues[1] = v }},
				{"missing parent", func(x *tracker.ChangeBinding) { delete(x.Issues, 0) }},
				{"duplicate id", func(x *tracker.ChangeBinding) { v := x.Issues[1]; v.ID = spec.ID; x.Issues[1] = v }},
				{"duplicate number", func(x *tracker.ChangeBinding) { v := x.Issues[1]; v.Number = spec.Number; x.Issues[1] = v }},
				{"wrong node", func(x *tracker.ChangeBinding) {
					v := x.Issues[1]
					if kind == "github" {
						v.NodeID = spec.NodeID
					} else {
						v.NodeID = "I_wrong"
					}
					x.Issues[1] = v
				}},
				{"bad receipt", func(x *tracker.ChangeBinding) {
					v := x.Issues[0]
					v.Receipt = &tracker.CreateReceipt{Number: "0"}
					x.Issues[0] = v
				}},
				{"different receipt", func(x *tracker.ChangeBinding) {
					v := x.Issues[0]
					r := *v.Receipt
					r.Number = "9"
					v.Receipt = &r
					x.Issues[0] = v
				}},
				{"unbound snapshot", func(x *tracker.ChangeBinding) {
					v := x.Issues[0]
					v.ID, v.Number, v.NodeID = "", "", ""
					x.Issues[0] = v
				}},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					x := *b
					x.Issues = map[int]tracker.IssueBinding{0: spec, 1: task}
					tc.mutate(&x)
					if err := x.Validate(); err == nil {
						t.Fatal("accepted invalid binding")
					}
				})
			}
		})
	}
	markdown.RemoteKey = strings.Repeat("a", 32)
	if markdown.Validate() == nil {
		t.Fatal("local binding accepted remote key")
	}
}

func TestBoundIssueSnapshotAndRecovery(t *testing.T) {
	log := fakeProviderCLIs(t)
	target := tracker.Target{Kind: "gitlab", Host: "gitlab.com", Project: "team/repo", ProjectID: "74"}
	b := tracker.ChangeBinding{Target: target, RemoteKey: strings.Repeat("a", 32)}
	marker := b.Marker(0)
	body := "Approved\n\n" + marker + "\n"
	entry := tracker.IssueBinding{Path: "spec.md", DraftDigest: tracker.BodyDigest("draft"), PublishDigest: tracker.BodyDigest(body), ID: "512", Number: "7", Baseline: "Local\ntext\n", BodyDigest: tracker.BodyDigest(body)}
	b.Issues = map[int]tracker.IssueBinding{0: entry}
	const get = "glab api projects/74/issues/7 --hostname gitlab.com"
	const list = "glab api projects/74/issues?scope=all&state=all&per_page=100 --hostname gitlab.com --paginate"
	row := func(id, number int, content string) string {
		data, err := json.Marshal(map[string]any{"id": id, "iid": number, "project_id": 74, "description": content})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	good := row(512, 7, body)
	ctx := context.Background()
	c := tracker.Client{}
	setIssueRoutes(t, map[string]issueRoute{get: {Response: good}})
	issue, err := b.VerifySnapshot(ctx, c, 0, "spec.md", []byte("Local\r\ntext\r\n"))
	if err != nil || issue.ID != "512" || issue.Number != "7" || issue.Body != body {
		t.Fatalf("verified snapshot: %+v %v", issue, err)
	}
	if _, err := b.VerifySnapshot(ctx, c, 0, "other.md", []byte("Local\ntext\n")); err == nil {
		t.Error("accepted wrong path")
	}
	if _, err := b.VerifySnapshot(ctx, c, 0, "spec.md", []byte("changed")); err == nil {
		t.Error("accepted local drift")
	}
	for _, tc := range []struct{ name, response string }{
		{"wrong identity", row(513, 7, body)}, {"missing marker", row(512, 7, "other")}, {"duplicate marker", row(512, 7, body+marker)}, {"remote drift", row(512, 7, body+"changed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setIssueRoutes(t, map[string]issueRoute{get: {Response: tc.response}})
			if _, err := b.VerifySnapshot(ctx, c, 0, "spec.md", []byte("Local\ntext\n")); err == nil {
				t.Fatal("accepted changed readback")
			}
		})
	}
	if _, err := b.ReadBoundIssue(ctx, c, 1); err == nil {
		t.Error("accepted missing binding")
	}
	pending := entry
	pending.ID, pending.Number, pending.Baseline, pending.BodyDigest = "", "", "", ""
	b.Issues[0] = pending
	for _, tc := range []struct {
		name, listing string
		read          issueRoute
		receipt       *tracker.CreateReceipt
		ok            bool
	}{
		{"unique readback", "[" + good + "]", issueRoute{Response: good}, nil, true},
		{"positive receipt", "[" + good + "]", issueRoute{Response: good}, &tracker.CreateReceipt{Number: "7", ID: "512"}, true},
		{"different receipt", "[" + good + "]", issueRoute{Response: good}, &tracker.CreateReceipt{Number: "7", ID: "513"}, false},
		{"missing", "[]", issueRoute{Response: good}, nil, false},
		{"ambiguous", "[" + good + "," + row(513, 8, body) + "]", issueRoute{Response: good}, nil, false},
		{"read failure", "[" + good + "]", issueRoute{Fail: true}, nil, false},
		{"changed readback", "[" + good + "]", issueRoute{Response: row(512, 7, body+"x")}, nil, false},
		{"changed listing", "[" + row(512, 7, body+"x") + "]", issueRoute{Response: good}, nil, false},
		{"different identity", "[" + good + "]", issueRoute{Response: row(513, 7, body)}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := pending
			p.Receipt = tc.receipt
			b.Issues[0] = p
			setIssueRoutes(t, map[string]issueRoute{list: {Response: tc.listing}, get: tc.read})
			got, err := b.RecoverCreate(ctx, c, 0)
			if (err == nil) != tc.ok || (tc.ok && (got.ID != "512" || got.Body != body)) {
				t.Fatalf("recovery: %+v %v", got, err)
			}
		})
	}
	setIssueRoutes(t, map[string]issueRoute{list: {Fail: true}})
	if _, err := b.RecoverCreate(ctx, c, 0); err == nil {
		t.Error("listing failure treated as successful recovery")
	}
	if _, err := b.RecoverCreate(ctx, c, 2); err == nil {
		t.Error("recovered missing slot")
	}
	setIssueRoutes(t, map[string]issueRoute{get: {Fail: true}})
	b.Issues[0] = entry
	if _, err := b.ReadBoundIssue(ctx, c, 0); err == nil {
		t.Error("bound read failure was ignored")
	}
	if _, err := b.RecoverCreate(ctx, c, 0); err == nil {
		t.Error("recovered already published slot")
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		if line != get && line != list {
			t.Fatal(fmt.Sprintf("unexpected mutation or request: %s", line))
		}
	}
}
