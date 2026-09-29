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

func TestStateSet_DesignAndBuildFields(t *testing.T) {
	dir := chdirTemp(t)
	initMulixRoot(t, dir)

	s := flow.New("add-login", "")
	s.Phase = flow.PhaseDesign
	saveStateForTest(t, dir, s)
	if err := state.SetActive(dir, "add-login"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	for _, kv := range [][2]string{
		{"design_track", "architectural"},
		{"design_path", ".mulix/.runtime/add-login/specs/2026-01-01-login-design.md"},
		{"design_approved", "true"},
		{"execution_method", "inline"},
	} {
		if out, err := runPresetCLI(t, "state", "set", kv[0], kv[1]); err != nil {
			t.Fatalf("state set %s: %v (output: %s)", kv[0], err, out)
		}
	}

	loaded, err := state.Load(dir, "add-login")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.DesignTrack != flow.TrackArchitectural || !loaded.DesignApproved ||
		loaded.DesignPath == "" || loaded.ExecutionMethod != flow.ExecInline {
		t.Fatalf("fields not recorded: %+v", loaded)
	}
}

func TestStateSet_RejectsUnknownTrackAndMethod(t *testing.T) {
	dir := chdirTemp(t)
	initMulixRoot(t, dir)
	saveStateForTest(t, dir, flow.New("add-login", ""))
	if err := state.SetActive(dir, "add-login"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}

	// spike is a brainstorming path, but not one a change with a spec,
	// plan, and tasks can take.
	if _, err := runPresetCLI(t, "state", "set", "design_track", "spike"); err == nil {
		t.Fatal("expected design_track=spike to be rejected")
	}
	if _, err := runPresetCLI(t, "state", "set", "execution_method", "yolo"); err == nil {
		t.Fatal("expected an unknown execution_method to be rejected")
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
