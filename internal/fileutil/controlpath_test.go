package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestControlPath_RejectsTraversalSymlinkAndNonRegularTarget(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"../outside", "/absolute", "C:/outside", `.mulix\..\outside`, ".mulix/../outside"} {
		t.Run(rel, func(t *testing.T) {
			if _, err := ControlPath(root, rel, true); err == nil {
				t.Fatalf("accepted %q", rel)
			}
		})
	}
	p, err := ControlPath(root, ".mulix/sessions/id.json", true)
	if err != nil || p != filepath.Join(root, ".mulix", "sessions", "id.json") {
		t.Fatalf("normal target: %s %v", p, err)
	}
	if err := os.Mkdir(p, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ControlPath(root, ".mulix/sessions/id.json", false); err == nil {
		t.Fatal("accepted directory target")
	}
	t.Run("symlink", func(t *testing.T) {
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if _, err := ControlPath(root, "linked/file", true); err == nil {
			t.Fatal("followed linked parent")
		}
	})
}
