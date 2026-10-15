package scaffold

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/z-fenix/mulix-coding/internal/layout"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{"Add Login Flow": "add-login-flow", "  spaces  ": "spaces", "weird!!chars??": "weird-chars", "already-a-slug": "already-a-slug"}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q)=%q want %q", in, got, want)
		}
	}
}

func TestCreateChangeDirAt_DateIDAndCollision(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 14, 0, 15, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	for i, want := range []string{"2026-10-14-add-login-flow", "2026-10-14-add-login-flow-2", "2026-10-14-add-login-flow-3"} {
		nc, err := createChangeDirAt(root, "Add Login Flow", now)
		if err != nil {
			t.Fatal(err)
		}
		if nc.Change != want {
			t.Fatalf("allocation %d=%q want=%q", i+1, nc.Change, want)
		}
		if nc.SpecDir != "docs/specs/"+want || nc.ChangeDir != "docs/changes/"+want || nc.RuntimeDir != ".mulix/.runtime/"+want {
			t.Fatalf("inconsistent directories: %+v", nc)
		}
		for _, rel := range []string{nc.SpecDir, nc.ChangeDir, nc.RuntimeDir} {
			entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil || len(entries) != 0 {
				t.Fatalf("new directory not empty: %s, %v", rel, err)
			}
		}
	}
}

func TestCreateChangeDirAt_DifferentSlugsDaysAndLegacyDoNotShareCounter(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "docs/changes/001-old/KEEP.md", "LEGACY\n")
	now := time.Date(2026, 10, 14, 23, 55, 0, 0, time.Local)
	for _, c := range []struct {
		title string
		at    time.Time
		want  string
	}{
		{"Add Login", now, "2026-10-14-add-login"},
		{"Add Search", now, "2026-10-14-add-search"},
		{"Add Login", now.AddDate(0, 0, 1), "2026-10-15-add-login"},
	} {
		nc, err := createChangeDirAt(root, c.title, c.at)
		if err != nil || nc.Change != c.want {
			t.Fatalf("change=%q want=%q error=%v", nc.Change, c.want, err)
		}
	}
	if got := readProjectFile(t, root, "docs/changes/001-old/KEEP.md"); got != "LEGACY\n" {
		t.Fatal("legacy directory modified")
	}
}

func TestCreateChangeDirAt_ExistingAnyArtifactPathCausesSuffix(t *testing.T) {
	for _, parent := range []string{SpecsDir, ChangesDir, layout.RuntimeDir} {
		t.Run(parent, func(t *testing.T) {
			root := t.TempDir()
			now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.Local)
			collision := filepath.ToSlash(filepath.Join(parent, "2026-10-14-add-login"))
			writeProjectFile(t, root, collision+"/KEEP.md", "KEEP\n")
			nc, err := createChangeDirAt(root, "Add Login", now)
			if err != nil || nc.Change != "2026-10-14-add-login-2" {
				t.Fatalf("collision reused: %+v %v", nc, err)
			}
			if got := readProjectFile(t, root, collision+"/KEEP.md"); got != "KEEP\n" {
				t.Fatal("existing artifact overwritten")
			}
		})
	}
}

func TestCreateChangeDirAt_ExistingFileAlsoCausesSuffix(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "docs/specs/2026-10-14-login", "KEEP FILE\n")
	nc, err := createChangeDirAt(root, "Login", time.Date(2026, 10, 14, 12, 0, 0, 0, time.Local))
	if err != nil || nc.Change != "2026-10-14-login-2" {
		t.Fatalf("file collision: %+v %v", nc, err)
	}
	if got := readProjectFile(t, root, "docs/specs/2026-10-14-login"); got != "KEEP FILE\n" {
		t.Fatal("collision file changed")
	}
}

func TestCreateChangeDir_RejectsUnusableSlug(t *testing.T) {
	if _, err := CreateChangeDir(t.TempDir(), "!!!"); err == nil {
		t.Fatal("unusable slug accepted")
	}
}

func TestCreateChangeDirAt_ParentFileIsError(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, SpecsDir, "NOT A DIRECTORY\n")
	if _, err := createChangeDirAt(root, "Login", time.Now()); err == nil {
		t.Fatal("parent file ignored")
	}
	if got := readProjectFile(t, root, SpecsDir); got != "NOT A DIRECTORY\n" {
		t.Fatal("parent file changed")
	}
}
