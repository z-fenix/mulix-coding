package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Add Login Flow": "add-login-flow",
		"  spaces  ":     "spaces",
		"weird!!chars??": "weird-chars",
		"already-a-slug": "already-a-slug",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNextNumber_EmptyChangesDirStartsAtOne(t *testing.T) {
	root := t.TempDir()
	n, err := NextNumber(root)
	if err != nil {
		t.Fatalf("NextNumber: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1, got %d", n)
	}
}

func TestNextNumber_SkipsAboveExistingMax(t *testing.T) {
	root := t.TempDir()
	changes := filepath.Join(root, ChangesDir)
	for _, name := range []string{"001-first", "002-second", "not-numbered"} {
		if err := os.MkdirAll(filepath.Join(changes, name), 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	n, err := NextNumber(root)
	if err != nil {
		t.Fatalf("NextNumber: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3, got %d", n)
	}
}

func TestCreateChangeDir_CreatesNumberedDirectory(t *testing.T) {
	root := t.TempDir()
	nc, err := CreateChangeDir(root, "Add Login Flow")
	if err != nil {
		t.Fatalf("CreateChangeDir: %v", err)
	}
	if nc.Change != "001-add-login-flow" {
		t.Fatalf("expected change 001-add-login-flow, got %q", nc.Change)
	}
	info, err := os.Stat(nc.SpecAbsDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected spec directory at %s to exist: %v", nc.SpecAbsDir, err)
	}
	info, err = os.Stat(nc.ChangeAbsDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected change directory at %s to exist: %v", nc.ChangeAbsDir, err)
	}

	nc2, err := CreateChangeDir(root, "Second Change")
	if err != nil {
		t.Fatalf("CreateChangeDir: %v", err)
	}
	if nc2.Change != "002-second-change" {
		t.Fatalf("expected change 002-second-change, got %q", nc2.Change)
	}
}

func TestCreateChangeDir_RejectsUnusableSlug(t *testing.T) {
	root := t.TempDir()
	if _, err := CreateChangeDir(root, "!!!"); err == nil {
		t.Fatal("expected error for a title with no usable slug characters")
	}
}
