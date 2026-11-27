package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewCreatesChangeOwnedRuntimeTests(t *testing.T) {
	root := t.TempDir()
	change, err := CreateChangeDir(root, "Add API checks")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(change.RuntimeDir), "tests"))
	if err != nil || !info.IsDir() {
		t.Fatalf("new change does not have its test source directory: %v", err)
	}
}
