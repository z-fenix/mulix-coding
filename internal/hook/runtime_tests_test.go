package hook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
)

func TestDecideRuntimeTestsRejectDanglingSourceLink(t *testing.T) {
	root := t.TempDir()
	rel := ".mulix/.runtime/own/tests/helpers/dangling.js"
	file := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "does-not-exist.js"), file); err != nil {
		t.Fatal(err)
	}
	s := flow.New("own", "")
	s.Phase = flow.PhaseBuild
	if got := Decide(root, s, Request{ToolName: "Edit", ToolInput: ToolInput{FilePath: rel}}); got.Allow {
		t.Fatal("dangling link was treated as a new authored source")
	}
}

func TestDecideRuntimeTests(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		phase flow.Phase
		path  string
		allow bool
	}{
		{flow.PhaseBuild, "own/tests/features/task-1/api.feature", true},
		{flow.PhaseBuild, "own/tests/features/final/suite.feature", true},
		{flow.PhaseBuild, "own/tests/helpers/fixture.js", true},
		{flow.PhaseBuild, "own/tests/features/task-1/result.json", false},
		{flow.PhaseBuild, "own/tests/features/task-1/launcher.karate.js", false},
		{flow.PhaseBuild, "other/tests/features/task-1/api.feature", false},
		{flow.PhaseTasks, "own/tests/features/task-1/api.feature", false},
		{flow.PhaseVerify, "own/tests/features/final/suite.feature", false},
		{flow.PhaseArchive, "own/tests/helpers/fixture.js", false},
	} {
		s := flow.New("own", "")
		s.Phase = tc.phase
		d := Decide(root, s, Request{ToolName: "Write", ToolInput: ToolInput{FilePath: ".mulix/.runtime/" + tc.path}})
		if d.Allow != tc.allow {
			t.Errorf("%s %s: allowed=%t want %t: %s", tc.phase, tc.path, d.Allow, tc.allow, d.Reason)
		}
	}
}
