package cliutil

import (
	"context"
	"strings"
	"testing"

	"github.com/z-fenix/mulix-coding/internal/flow"
	"github.com/z-fenix/mulix-coding/internal/sessionctx"
	"github.com/z-fenix/mulix-coding/internal/state"
)

func TestStateMutation_ExplicitChangeCannotBypassBindingOrFallback(t *testing.T) {
	root, s := sessionProject(t)
	selectForSession(t, s, "A", "X")
	for _, id := range []string{"A", "B"} {
		if out, err := runPresetCLI(t, "state", "set", "spec_path", "other", "--change", "Y", "--session-id", id); err == nil {
			t.Fatalf("%s bypass: %s", id, out)
		}
		out, err := runPresetCLI(t, "state", "show", "--change", "Y", "--session-id", id)
		if err != nil || !strings.Contains(out, "explicit-change") {
			t.Fatalf("query: %s %v", out, err)
		}
	}
	out, err := runPresetCLI(t, "state", "set", "spec_path", "owned", "--session-id", "A")
	if err != nil {
		t.Fatalf("%s %v", out, err)
	}
	x, _ := state.Load(root, "X")
	y, _ := state.Load(root, "Y")
	if x.SpecPath != "owned" || y.SpecPath != "" {
		t.Fatal("mutation crossed binding")
	}
}

func TestArchive_PreservesBindingAndConditionallyClearsGlobal(t *testing.T) {
	root, s := sessionProject(t)
	x, _ := state.Load(root, "X")
	x.Phase = flow.PhaseArchive
	x.ArchiveConfirmation = flow.ArchiveConfirmed
	if err := state.Save(root, x); err != nil {
		t.Fatal(err)
	}
	selectForSession(t, s, "A", "X")
	if err := state.SetActive(root, "Y"); err != nil {
		t.Fatal(err)
	}
	out, err := runPresetCLI(t, "state", "transition", "archived", "--session-id", "A")
	if err != nil {
		t.Fatalf("%s %v", out, err)
	}
	active, _ := state.Active(root)
	if active != "Y" {
		t.Fatal("cleared concurrent selection", active)
	}
	got, err := s.Resolve(context.Background(), sessionctx.ResolveRequest{Identity: sessionctx.Identity{Value: "A", Present: true}})
	if err == nil || got.Source != sessionctx.SessionBinding {
		t.Fatalf("archived binding silently fell back: %+v %v", got, err)
	}
}
