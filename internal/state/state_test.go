package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mulix-dev/mulix-coding/internal/flow"
)

func TestSaveThenLoad_RoundTrips(t *testing.T) {
	root := t.TempDir()
	s := flow.New("add-login", "feat/add-login")
	s.SpecPath = "docs/specs/001-add-login/spec.md"

	if err := Save(root, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(root, "add-login")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Change != s.Change || loaded.Phase != s.Phase || loaded.SpecPath != s.SpecPath {
		t.Fatalf("round trip mismatch: got %+v, want fields from %+v", loaded, s)
	}
	if loaded.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt to be stamped on save")
	}
}

func TestLoad_UnknownFieldFailsClosed(t *testing.T) {
	root := t.TempDir()
	path := PathFor(root, "x")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	bad := []byte("schema: mulix.state.v1\nchange: x\nphase: specify\nverify_failures: 0\narchived: false\ntotally_unknown_field: true\n")
	if err := os.WriteFile(path, bad, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Load(root, "x"); err == nil {
		t.Fatal("expected Load to fail on unknown field, got nil error")
	}
}

func TestLoad_WrongSchemaVersionFailsClosed(t *testing.T) {
	root := t.TempDir()
	path := PathFor(root, "x")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	bad := []byte("schema: mulix.state.v0\nchange: x\nphase: specify\nverify_failures: 0\narchived: false\n")
	if err := os.WriteFile(path, bad, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := Load(root, "x"); err == nil {
		t.Fatal("expected Load to fail on schema mismatch, got nil error")
	}
}

func TestExists(t *testing.T) {
	root := t.TempDir()
	if Exists(root, "nope") {
		t.Fatal("expected Exists to be false before Save")
	}
	if err := Save(root, flow.New("nope", "")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !Exists(root, "nope") {
		t.Fatal("expected Exists to be true after Save")
	}
}

func TestPathFor_LivesUnderMulixRuntime(t *testing.T) {
	got := filepath.ToSlash(PathFor("/repo", "001-x"))
	if got != "/repo/.mulix/.runtime/001-x/state.yaml" {
		t.Fatalf("PathFor = %s", got)
	}
}

func TestLoad_PointsAtLegacyStateLocation(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "docs", "changes", "001-x", ".runtime", "state.yaml")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("schema: mulix.state.v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(root, "001-x")
	if err == nil || !strings.Contains(err.Error(), "mulix.state.v1") {
		t.Fatalf("expected an error naming the legacy state file, got %v", err)
	}
}

func TestSave_RejectsReservedSharedID(t *testing.T) {
	if err := Save(t.TempDir(), flow.New("_shared", "")); err == nil {
		t.Fatal("expected _shared to be rejected as a change id")
	}
}
