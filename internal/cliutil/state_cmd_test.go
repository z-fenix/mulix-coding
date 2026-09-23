package cliutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mulix-dev/mulix-coding/internal/flow"
	"github.com/mulix-dev/mulix-coding/internal/state"
)

// saveStateForTest writes s directly via the state package, bypassing the
// CLI's own guard-gated transitions, so tests can jump straight to the
// phase they want to exercise.
func saveStateForTest(t *testing.T, root string, s flow.State) {
	t.Helper()
	if err := state.Save(root, s); err != nil {
		t.Fatalf("state.Save: %v", err)
	}
}

func TestStateSet_DelegatedToSubagentsField(t *testing.T) {
	dir := chdirTemp(t)
	initMulixRoot(t, dir)

	s := flow.New("add-login", "")
	s.Phase = flow.PhaseBuild
	saveStateForTest(t, dir, s)
	if err := state.SetActive(dir, "add-login"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	out, err := runPresetCLI(t, "state", "set", "delegated_to_subagents", "true")
	if err != nil {
		t.Fatalf("state set delegated_to_subagents: %v (output: %s)", err, out)
	}

	loaded, err := state.Load(dir, "add-login")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !loaded.DelegatedToSubagents {
		t.Fatal("expected delegated_to_subagents to be set to true")
	}
}

func TestTransition_ArchivingActiveChangeClearsActiveMarker(t *testing.T) {
	dir := chdirTemp(t)
	initMulixRoot(t, dir)

	s := flow.New("add-login", "")
	s.Phase = flow.PhaseArchive
	s.ArchiveConfirmation = flow.ArchiveConfirmed
	saveStateForTest(t, dir, s)
	if err := state.SetActive(dir, "add-login"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	out, err := runPresetCLI(t, "state", "transition", "archived")
	if err != nil {
		t.Fatalf("state transition archived: %v (output: %s)", err, out)
	}

	if _, err := os.Stat(filepath.Join(dir, ".mulix", "active")); !os.IsNotExist(err) {
		t.Fatalf("expected .mulix/active to be removed after archiving the active change, stat err = %v", err)
	}
	if _, err := state.Active(dir); err == nil {
		t.Fatal("expected no active change after archiving it")
	}
}

func TestTransition_ArchivingNonActiveChangeLeavesActiveMarkerAlone(t *testing.T) {
	dir := chdirTemp(t)
	initMulixRoot(t, dir)

	s := flow.New("add-login", "")
	s.Phase = flow.PhaseArchive
	s.ArchiveConfirmation = flow.ArchiveConfirmed
	saveStateForTest(t, dir, s)

	other := flow.New("other-change", "")
	saveStateForTest(t, dir, other)
	if err := state.SetActive(dir, "other-change"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	out, err := runPresetCLI(t, "state", "transition", "archived", "--change", "add-login")
	if err != nil {
		t.Fatalf("state transition archived: %v (output: %s)", err, out)
	}

	active, err := state.Active(dir)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if active != "other-change" {
		t.Fatalf("expected active change to remain %q, got %q", "other-change", active)
	}
}
