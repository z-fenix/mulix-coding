package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mulix-dev/mulix-coding/internal/flow"
)

func stateForTest(change string) flow.State {
	return flow.New(change, "")
}

func TestFindRoot_WalksUpToMulixDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".mulix"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	found, err := FindRoot(nested)
	if err != nil {
		t.Fatalf("FindRoot: %v", err)
	}
	wantAbs, _ := filepath.Abs(root)
	if found != wantAbs {
		t.Fatalf("expected %s, got %s", wantAbs, found)
	}
}

func TestFindRoot_NoMulixDirErrors(t *testing.T) {
	root := t.TempDir()
	if _, err := FindRoot(root); err != ErrNoRoot {
		t.Fatalf("expected ErrNoRoot, got %v", err)
	}
}

func TestSetActiveThenActive(t *testing.T) {
	root := t.TempDir()
	if err := SetActive(root, "add-login"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	got, err := Active(root)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if got != "add-login" {
		t.Fatalf("expected add-login, got %q", got)
	}
}

func TestClearActive_RemovesMarker(t *testing.T) {
	root := t.TempDir()
	if err := SetActive(root, "add-login"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if err := ClearActive(root); err != nil {
		t.Fatalf("ClearActive: %v", err)
	}
	if _, err := Active(root); err == nil {
		t.Fatal("expected error after ClearActive, active change should be unset")
	}
}

func TestClearActive_NoneSetIsNotAnError(t *testing.T) {
	root := t.TempDir()
	if err := ClearActive(root); err != nil {
		t.Fatalf("ClearActive with no active change set: %v", err)
	}
}

func TestActiveIs(t *testing.T) {
	root := t.TempDir()
	if ActiveIs(root, "add-login") {
		t.Fatal("expected false when no active change is set")
	}
	if err := SetActive(root, "add-login"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if !ActiveIs(root, "add-login") {
		t.Fatal("expected true for the active change")
	}
	if ActiveIs(root, "other-change") {
		t.Fatal("expected false for a different change id")
	}
}

func TestActive_NoneSetErrors(t *testing.T) {
	root := t.TempDir()
	if _, err := Active(root); err == nil {
		t.Fatal("expected error when no active change is set")
	}
}

func TestList_ReturnsSortedChangeIDs(t *testing.T) {
	root := t.TempDir()
	// No .mulix/state dir yet.
	changes, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no changes, got %v", changes)
	}

	for _, name := range []string{"zeta", "alpha", "mid"} {
		s := stateForTest(name)
		if err := Save(root, s); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	changes, err = List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha", "mid", "zeta"}
	if len(changes) != len(want) {
		t.Fatalf("expected %v, got %v", want, changes)
	}
	for i := range want {
		if changes[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, changes)
		}
	}
}
