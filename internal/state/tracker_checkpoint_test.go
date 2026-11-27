package state

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/layout"
	"github.com/z-fenix/mulix-coding/internal/tracker"
)

func nativeCheckpointFixture(t *testing.T) flow.State {
	t.Helper()
	s := flow.New("checkpoint", "")
	s.ArtifactLayout = flow.RuntimeIssuesLayout
	s.SpecPath = layout.ChangeRuntimePath(s.Change, layout.RuntimeSpecFile)
	s.TasksDir = layout.ChangeRuntimePath(s.Change, layout.RuntimeIssuesDir)
	s.ReportPath = layout.ChangeRuntimePath(s.Change, layout.RuntimeReportFile)
	b, err := tracker.NewChangeBinding(tracker.Target{Kind: "gitlab", Host: "gitlab.com", Project: "team/repo"})
	if err != nil {
		t.Fatal(err)
	}
	s.Tracker = b
	return s
}

func TestTrackerCheckpointPinsAndRoundTrips(t *testing.T) {
	root := t.TempDir()
	s := nativeCheckpointFixture(t)
	if err := Save(root, s); err != nil {
		t.Fatal(err)
	}
	if err := SaveTrackerCheckpoint(root, s); err == nil {
		t.Fatal("checkpoint without issue accepted")
	}
	s.Tracker.Target.ProjectID = "74"
	if err := Save(root, s); err != nil {
		t.Fatalf("first project pin: %v", err)
	}
	draft, body := tracker.BodyDigest("draft"), tracker.BodyDigest("published")
	pending := tracker.IssueBinding{Path: s.SpecPath, DraftDigest: draft, PublishDigest: body}
	s.Tracker.Issues = map[int]tracker.IssueBinding{0: pending}
	if err := SaveTrackerCheckpoint(root, s); err != nil {
		t.Fatalf("pending: %v", err)
	}
	loaded, err := Load(root, s.Change)
	if err != nil || loaded.Tracker.Target.ProjectID != "74" || loaded.Tracker.Issues[0] != pending {
		t.Fatalf("pending round trip: %+v %v", loaded.Tracker, err)
	}
	pending.Receipt = &tracker.CreateReceipt{Number: "7", ID: "512"}
	s.Tracker.Issues[0] = pending
	if err := SaveTrackerCheckpoint(root, s); err != nil {
		t.Fatalf("receipt: %v", err)
	}
	pending.ID, pending.Number, pending.BodyDigest, pending.Baseline = "512", "7", body, "local"
	s.Tracker.Issues[0] = pending
	if err := SaveTrackerCheckpoint(root, s); err != nil {
		t.Fatalf("mapping: %v", err)
	}
	loaded, err = Load(root, s.Change)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.Tracker.Issues[0]
	if got.ID != "512" || got.Number != "7" || got.Path != s.SpecPath || got.DraftDigest != draft || got.PublishDigest != body || got.Receipt == nil || *got.Receipt != *pending.Receipt || got.Baseline != "local" {
		t.Fatalf("lost mapping: %+v", got)
	}
	path := PathFor(root, s.Change)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*flow.State)
	}{
		{"selector", func(x *flow.State) { x.Tracker.Target.Project = "other/repo" }},
		{"project pin", func(x *flow.State) { x.Tracker.Target.ProjectID = "75" }},
		{"key", func(x *flow.State) { x.Tracker.RemoteKey = strings.Repeat("a", 32) }},
		{"layout downgrade", func(x *flow.State) { x.ArtifactLayout = ""; x.Tracker = nil; x.TasksDir = "" }},
		{"path", func(x *flow.State) { v := x.Tracker.Issues[0]; v.Path = "elsewhere"; x.Tracker.Issues[0] = v }},
		{"draft", func(x *flow.State) {
			v := x.Tracker.Issues[0]
			v.DraftDigest = tracker.BodyDigest("different")
			x.Tracker.Issues[0] = v
		}},
		{"publish", func(x *flow.State) {
			v := x.Tracker.Issues[0]
			v.PublishDigest = tracker.BodyDigest("different")
			x.Tracker.Issues[0] = v
		}},
		{"remove receipt", func(x *flow.State) { v := x.Tracker.Issues[0]; v.Receipt = nil; x.Tracker.Issues[0] = v }},
		{"replace identity", func(x *flow.State) {
			v := x.Tracker.Issues[0]
			v.ID = "513"
			v.Receipt = &tracker.CreateReceipt{Number: "7", ID: "513"}
			x.Tracker.Issues[0] = v
		}},
		{"remove slot", func(x *flow.State) { delete(x.Tracker.Issues, 0) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x, err := Load(root, s.Change)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(&x)
			if err := Save(root, x); err == nil {
				t.Fatal("accepted pinned change")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("saved state changed: %v", err)
			}
		})
	}
}

func TestNativeArtifactValidationAndInvalidYAML(t *testing.T) {
	root := t.TempDir()
	s := nativeCheckpointFixture(t)
	d := tracker.BodyDigest("draft")
	s.Tracker.Target.ProjectID = "74"
	s.Tracker.Issues = map[int]tracker.IssueBinding{0: {Path: s.SpecPath, DraftDigest: d, PublishDigest: d, ID: "512", Number: "7", BodyDigest: d}, 1: {Path: s.TasksDir + "/01-first.md", DraftDigest: d, PublishDigest: d}}
	if err := SaveTrackerCheckpoint(root, s); err != nil {
		t.Fatal(err)
	}
	path := PathFor(root, s.Change)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, badPath := range []string{"elsewhere/01-first.md", s.TasksDir + "/1-first.md", s.TasksDir + "/02-first.md", s.TasksDir + "/01-first.txt", s.TasksDir + "/nested/01-first.md"} {
		x, err := Load(root, s.Change)
		if err != nil {
			t.Fatal(err)
		}
		v := x.Tracker.Issues[1]
		v.Path = badPath
		x.Tracker.Issues[1] = v
		if err := Save(root, x); err == nil {
			t.Errorf("accepted task path %q", badPath)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, before) {
			t.Fatalf("invalid save altered state: %v", err)
		}
	}
	bad := []byte("artifact_layout: runtime-issues-v1\ntracker: [\n")
	if err := os.WriteFile(path, bad, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, s.Change); !errors.Is(err, ErrInvalidArtifacts) {
		t.Fatalf("invalid YAML not identified: %v", err)
	}
	if err := Save(root, s); err == nil {
		t.Fatal("overwrote corrupt pinned state")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, bad) {
		t.Fatalf("corrupt state overwritten: %v", err)
	}
}

func TestNativeStateRejectsInvalidMetadata(t *testing.T) {
	root := t.TempDir()
	base := nativeCheckpointFixture(t)
	cases := []struct {
		name   string
		mutate func(*flow.State)
	}{
		{"unknown layout", func(s *flow.State) { s.ArtifactLayout = "future" }},
		{"invalid change", func(s *flow.State) { s.Change = "../elsewhere" }},
		{"missing tracker", func(s *flow.State) { s.Tracker = nil }},
		{"invalid tracker", func(s *flow.State) { s.Tracker.RemoteKey = "not-hex" }},
		{"wrong spec", func(s *flow.State) { s.SpecPath = "other/spec.md" }},
		{"wrong report", func(s *flow.State) { s.ReportPath = "other/report.md" }},
		{"legacy tasks", func(s *flow.State) { s.TasksPath = "tasks.md" }},
		{"tracker without layout", func(s *flow.State) { s.ArtifactLayout = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			binding := *base.Tracker
			s.Tracker = &binding
			tc.mutate(&s)
			if err := Save(root, s); err == nil {
				t.Fatal("accepted invalid native state")
			}
			if Exists(root, base.Change) {
				t.Fatal("invalid state was saved")
			}
		})
	}
	if err := Save(root, base); err != nil {
		t.Fatal(err)
	}
	path := PathFor(root, base.Change)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, from, to string }{
		{"schema", "schema: " + flow.SchemaVersion, "schema: future"},
		{"phase", "phase: specify", "phase: imaginary"},
		{"change", "change: checkpoint", "change: different"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := strings.Replace(string(original), tc.from, tc.to, 1)
			if bad == string(original) {
				t.Fatalf("fixture lacks %q", tc.from)
			}
			if err := os.WriteFile(path, []byte(bad), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(root, base.Change); !errors.Is(err, ErrInvalidArtifacts) {
				t.Fatalf("expected invalid native artifacts: %v", err)
			}
		})
	}
}

func TestNativeFootprintAndLegacyMigrationFailClosed(t *testing.T) {
	root := t.TempDir()
	s := nativeCheckpointFixture(t)
	issues := filepath.Join(root, filepath.FromSlash(s.TasksDir))
	if err := os.MkdirAll(issues, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, s.Change); !errors.Is(err, ErrInvalidArtifacts) {
		t.Fatalf("missing state with native footprint: %v", err)
	}
	path := PathFor(root, s.Change)
	malformed := []byte("schema: [\n")
	if err := os.WriteFile(path, malformed, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root, s.Change); !errors.Is(err, ErrInvalidArtifacts) {
		t.Fatalf("malformed state with native footprint: %v", err)
	}
	root = t.TempDir()
	legacy := flow.New(s.Change, "")
	if err := Save(root, legacy); err != nil {
		t.Fatal(err)
	}
	path = PathFor(root, s.Change)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(root, s); err == nil {
		t.Fatal("migrated legacy state through ordinary save")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("legacy state changed: %v", err)
	}
}

func TestTrackerCheckpointRejectsLegacyAndUnsafePaths(t *testing.T) {
	root := t.TempDir()
	legacy := flow.New("legacy", "")
	if err := Save(root, legacy); err != nil {
		t.Fatal(err)
	}
	if err := SaveTrackerCheckpoint(root, legacy); err == nil {
		t.Fatal("legacy checkpoint accepted")
	}
	loaded, err := Load(root, "legacy")
	if err != nil || loaded.ArtifactLayout != "" || loaded.Change != "legacy" {
		t.Fatalf("legacy changed: %+v %v", loaded, err)
	}
	root = t.TempDir()
	s := nativeCheckpointFixture(t)
	s.Tracker.Target.ProjectID = "74"
	d := tracker.BodyDigest("x")
	s.Tracker.Issues = map[int]tracker.IssueBinding{0: {Path: s.SpecPath, DraftDigest: d, PublishDigest: d}}
	parent := filepath.Join(root, ".mulix")
	if err := os.MkdirAll(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".runtime"), []byte("unchanged"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := SaveTrackerCheckpoint(root, s); err == nil {
		t.Fatal("accepted non-directory runtime")
	}
	data, err := os.ReadFile(filepath.Join(parent, ".runtime"))
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("unsafe path changed: %q %v", data, err)
	}
}
