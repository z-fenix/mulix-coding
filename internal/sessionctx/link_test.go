package sessionctx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestControlMetadata_RejectsBindingAndConfigSymlinks(t *testing.T) {
	for _, which := range []string{"binding", "config"} {
		t.Run(which, func(t *testing.T) {
			root, store := contextFixture(t)
			bindTest(t, store, "A", "X")
			target := bindingFile(root, "A")
			if which == "config" {
				target = filepath.Join(root, ".mulix", "context.json")
			}
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "data.json")
			if err := os.WriteFile(outside, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, target); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if which == "config" {
				if _, err := store.Mode(context.Background()); err == nil {
					t.Fatal("followed linked mode file")
				}
			} else if _, err := resolveTest(store, "A"); err == nil {
				t.Fatal("followed linked binding file")
			}
		})
	}
}
